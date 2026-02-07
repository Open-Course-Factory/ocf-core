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
			// Check if it's a "terminal not found" error
			if err.Error() == "terminal not found" {
				ctx.AbortWithStatusJSON(http.StatusNotFound, &errors.APIError{
					ErrorCode:    http.StatusNotFound,
					ErrorMessage: "terminal not found",
				})
				return
			}
			// Other errors are internal server errors
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

		// NEW: Validate session state (use local state only for performance)
		isValid, reason, err := tam.service.ValidateSessionAccess(terminalID, false)
		if err != nil {
			ctx.AbortWithStatusJSON(http.StatusInternalServerError, &errors.APIError{
				ErrorCode:    http.StatusInternalServerError,
				ErrorMessage: fmt.Sprintf("failed to validate session status: %v", err),
			})
			return
		}

		if !isValid {
			// Return appropriate status based on reason
			if reason == "backend_offline" {
				ctx.AbortWithStatusJSON(http.StatusServiceUnavailable, &errors.APIError{
					ErrorCode:    http.StatusServiceUnavailable,
					ErrorMessage: "Session's backend is currently unavailable",
				})
			} else if reason == "expired" {
				ctx.AbortWithStatusJSON(http.StatusGone, &errors.APIError{
					ErrorCode:    http.StatusGone,
					ErrorMessage: "Terminal session has expired and is no longer accessible",
				})
			} else if reason == "stopped" {
				ctx.AbortWithStatusJSON(http.StatusForbidden, &errors.APIError{
					ErrorCode:    http.StatusForbidden,
					ErrorMessage: "Terminal session has been stopped and is no longer accessible",
				})
			} else {
				ctx.AbortWithStatusJSON(http.StatusForbidden, &errors.APIError{
					ErrorCode:    http.StatusForbidden,
					ErrorMessage: fmt.Sprintf("Terminal session is not in an active state: %s", reason),
				})
			}
			return
		}

		// Access granted, continue to handler
		ctx.Next()
	}
}
