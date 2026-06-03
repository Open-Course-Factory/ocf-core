package userController

import (
	"errors"
	"net/http"

	"soli/formations/src/auth/services"
	sqldb "soli/formations/src/db"
	paymentServices "soli/formations/src/payment/services"
	"soli/formations/src/utils"

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

	// Self-service erasure is irreversible — never let it run under an
	// impersonated session (an admin "acting as" a user must not be able to
	// delete that user's account).
	if ctx.GetString("impersonatorId") != "" {
		ctx.JSON(http.StatusForbidden, gin.H{"error": "Account deletion is not allowed while impersonating another user"})
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

	// Run the full erasure flow. The service composes the canonical
	// userService.DeleteUser (Stripe cancel → pseudonymize → Casdoor delete →
	// RBAC removal) before the OCF-side cascade, so the handler no longer does
	// any Casdoor / RBAC work itself.
	deletionService := services.NewUserDeletionService(
		sqldb.DB,
		services.NewUserService(
			services.NewCasdoorUserClient(),
			paymentServices.NewPaymentDeletionHelper(sqldb.DB),
		),
	)
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
			// Includes the Stripe-cancel abort: returning 5xx lets the user
			// retry rather than silently leaving a billed-but-deleted account.
			utils.Error("Account deletion failed for user %s: %v", userID, err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Account deletion failed"})
			return
		}
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "Account deleted"})
}
