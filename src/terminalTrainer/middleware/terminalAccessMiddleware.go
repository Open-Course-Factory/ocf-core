package terminalMiddleware

import (
	"fmt"
	"net/http"

	"soli/formations/src/auth/errors"
	"soli/formations/src/terminalTrainer/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// TerminalAccessMiddleware provides middleware for checking terminal access
type TerminalAccessMiddleware struct {
	service services.TerminalTrainerService
}

// NewTerminalAccessMiddleware creates a new terminal access middleware
func NewTerminalAccessMiddleware(db *gorm.DB) *TerminalAccessMiddleware {
	return &TerminalAccessMiddleware{
		service: services.NewTerminalTrainerService(db),
	}
}

// RequireTerminalAccess is a middleware that ensures the user has the required access level to a terminal.
// This implements the second layer of the two-layer security model:
// - Layer 1: Casbin checks generic route permissions (/api/v1/terminals/:id)
// - Layer 2: This middleware checks resource-specific access via terminal_shares table
//
// The terminal ID is expected to be in the route parameter "id".
// The user ID is expected to be in the context (set by AuthManagement middleware).
//
// Usage:
//
//	routes.GET("/:id", middleware.AuthManagement(), terminalAccessMiddleware.RequireTerminalAccess("read"), handler)
func (tam *TerminalAccessMiddleware) RequireTerminalAccess(requiredLevel string) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		// Get terminal ID from route parameter
		terminalID := ctx.Param("id")
		if terminalID == "" {
			ctx.AbortWithStatusJSON(http.StatusBadRequest, &errors.APIError{
				ErrorCode:    http.StatusBadRequest,
				ErrorMessage: "terminal ID is required",
			})
			return
		}

		// Get user ID from context (set by AuthManagement middleware)
		userID := ctx.GetString("userId")
		if userID == "" {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, &errors.APIError{
				ErrorCode:    http.StatusUnauthorized,
				ErrorMessage: "user not authenticated",
			})
			return
		}

		// Check if user is admin (admins have access to everything)
		userRoles := ctx.GetStringSlice("userRoles")
		for _, role := range userRoles {
			if role == "administrator" {
				// Admin has full access
				ctx.Next()
				return
			}
		}

		// Check terminal-specific access (owner or shared)
		hasAccess, err := tam.service.HasTerminalAccess(terminalID, userID, requiredLevel)
		if err != nil {
			ctx.AbortWithStatusJSON(http.StatusInternalServerError, &errors.APIError{
				ErrorCode:    http.StatusInternalServerError,
				ErrorMessage: fmt.Sprintf("failed to check terminal access: %v", err),
			})
			return
		}

		if !hasAccess {
			ctx.AbortWithStatusJSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: fmt.Sprintf("you do not have '%s' access to this terminal", requiredLevel),
			})
			return
		}

		// Access granted, continue to handler
		ctx.Next()
	}
}
