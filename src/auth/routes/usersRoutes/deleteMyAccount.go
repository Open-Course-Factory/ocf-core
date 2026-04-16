package userController

import (
	"errors"
	"net/http"

	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/services"
	sqldb "soli/formations/src/db"
	"soli/formations/src/utils"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/gin-gonic/gin"
)

// deleteMyAccountRequest is the expected request body for account deletion
type deleteMyAccountRequest struct {
	Confirmation string `json:"confirmation" binding:"required"`
}

// DeleteMyAccount permanently deletes the authenticated user's account and all
// associated data (RGPD right to erasure).
//
//	@Summary		Delete own account (RGPD right to erasure)
//	@Description	Permanently deletes the authenticated user's account and all associated data.
//	@Description	Requires confirmation body {"confirmation": "DELETE_MY_ACCOUNT"}.
//	@Description	Blocks if the user owns non-personal organizations or groups (must transfer ownership first).
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Param			body	body	deleteMyAccountRequest	true	"Confirmation body"
//	@Security		Bearer
//	@Success		200	{object}	map[string]string	"Account deleted"
//	@Failure		400	{object}	map[string]string	"Missing or invalid confirmation"
//	@Failure		401	{object}	map[string]string	"Not authenticated"
//	@Failure		409	{object}	map[string]string	"User owns organizations or groups"
//	@Failure		500	{object}	map[string]string	"Deletion failed"
//	@Router			/users/me/account [delete]
func (uc *userController) DeleteMyAccount(ctx *gin.Context) {
	userID := ctx.GetString("userId")
	if userID == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	// Validate confirmation body
	var req deleteMyAccountRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Request body must contain {\"confirmation\": \"DELETE_MY_ACCOUNT\"}"})
		return
	}
	if req.Confirmation != "DELETE_MY_ACCOUNT" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Confirmation must be exactly \"DELETE_MY_ACCOUNT\""})
		return
	}

	// Run OCF database cascade deletion
	deletionService := services.NewUserDeletionService(sqldb.DB)
	err := deletionService.DeleteMyAccount(userID)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrOwnsOrganizations):
			ctx.JSON(http.StatusConflict, gin.H{"error": "You must transfer ownership of your organizations before deleting your account"})
			return
		case errors.Is(err, services.ErrOwnsGroups):
			ctx.JSON(http.StatusConflict, gin.H{"error": "You must transfer ownership of your groups before deleting your account"})
			return
		default:
			utils.Error("Account deletion failed for user %s: %v", userID, err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Account deletion failed"})
			return
		}
	}

	// Point of no return: delete Casdoor identity
	user, err := casdoorsdk.GetUserByUserId(userID)
	if err != nil {
		utils.Error("Failed to get Casdoor user %s after DB cleanup: %v", userID, err)
		// DB is already cleaned, return success to the user
	} else if user != nil {
		_, err = casdoorsdk.DeleteUser(user)
		if err != nil {
			utils.Error("Failed to delete Casdoor user %s: %v (DB already cleaned)", userID, err)
		}
	}

	// Remove RBAC policies
	opts := utils.DefaultPermissionOptions()
	opts.WarnOnError = true
	utils.RemoveGroupingPolicy(casdoor.Enforcer, userID, "", opts)

	ctx.JSON(http.StatusOK, gin.H{"message": "Account deleted"})
}
