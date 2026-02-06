package userController

import (
	"net/http"
	"slices"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/gin-gonic/gin"

	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/errors"
)

// GetCurrentUser godoc
//
//	@Summary		Get current user
//	@Description	Retrieve basic information about the currently authenticated user including roles
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Security		Bearer
//	@Success		200	{object}	dto.CurrentUserOutput
//	@Failure		401	{object}	errors.APIError	"Unauthorized"
//	@Failure		500	{object}	errors.APIError	"Internal server error"
//	@Router			/auth/me [get]
func GetCurrentUser(ctx *gin.Context) {
	// Get authenticated user ID from JWT token
	userID := ctx.GetString("userId")
	if userID == "" {
		ctx.JSON(http.StatusUnauthorized, &errors.APIError{
			ErrorCode:    http.StatusUnauthorized,
			ErrorMessage: "User not authenticated",
		})
		return
	}

	// Get user roles from context (already fetched by auth middleware)
	userRoles, _ := ctx.Get("userRoles")
	roles := []string{}
	if userRoles != nil {
		roles = userRoles.([]string)
	}

	// Get user from Casdoor
	user, err := casdoorsdk.GetUserByUserId(userID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to retrieve user information: " + err.Error(),
		})
		return
	}

	if user == nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "User not found",
		})
		return
	}

	// Check if user is admin based on roles
	isAdmin := slices.Contains(roles, "administrator")

	// Extract email verification status from Casdoor properties
	emailVerified := false
	emailVerifiedAt := ""
	if user.Properties != nil {
		emailVerified = user.Properties["email_verified"] == "true"
		emailVerifiedAt = user.Properties["email_verified_at"]
	}

	// Build response
	response := &dto.CurrentUserOutput{
		UserID:          user.Id,
		UserName:        user.Name,
		DisplayName:     user.DisplayName,
		Email:           user.Email,
		FirstName:       user.FirstName,
		LastName:        user.LastName,
		Avatar:          user.Avatar,
		Roles:           roles,
		IsAdmin:         isAdmin,
		EmailVerified:   emailVerified,
		EmailVerifiedAt: emailVerifiedAt,
	}

	ctx.JSON(http.StatusOK, response)
}
