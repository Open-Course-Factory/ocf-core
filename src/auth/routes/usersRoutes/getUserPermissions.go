package userController

import (
	"net/http"

	"soli/formations/src/auth/errors"
	"soli/formations/src/auth/services"
	sqldb "soli/formations/src/db"

	"github.com/gin-gonic/gin"
)

// GetUserPermissions godoc
//
//	@Summary		Get user permissions
//	@Description	Retrieve comprehensive permissions for the authenticated user including Casbin permissions, roles, organization memberships, group memberships, and aggregated features
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Security		Bearer
//	@Success		200	{object}	dto.UserPermissionsOutput
//	@Failure		401	{object}	errors.APIError	"Unauthorized"
//	@Failure		500	{object}	errors.APIError	"Internal server error"
//	@Router			/auth/permissions [get]
func GetUserPermissions(ctx *gin.Context) {
	// Get authenticated user ID from JWT token
	userID := ctx.GetString("userId")
	if userID == "" {
		ctx.JSON(http.StatusUnauthorized, &errors.APIError{
			ErrorCode:    http.StatusUnauthorized,
			ErrorMessage: "User not authenticated",
		})
		return
	}

	// Create permission service
	permissionsService := services.NewUserPermissionsService(sqldb.DB)

	// Get user permissions
	permissions, err := permissionsService.GetUserPermissions(userID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to retrieve user permissions: " + err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, permissions)
}
