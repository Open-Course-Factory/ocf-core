package userController

import (
	"net/http"
	"soli/formations/src/auth/access"
	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/errors"
	"soli/formations/src/auth/services"
	"soli/formations/src/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Delete user godoc
//
//	@Summary		Delete a user (administrator)
//	@Description	Permanently erases a user: Stripe cancellation, Casdoor identity, RBAC policies and all OCF-side data (memberships, personal organization, sessions, settings).
//	@Description	Same erasure flow and same ownership rule as the self-service deletion: a user who still owns a non-personal organization or a group must transfer ownership first.
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Param			id	path	string	true	"ID user"
//
//	@Security		Bearer
//
//	@Success		204	{object}	string
//
//	@Failure		400	{object}	errors.APIError	"Invalid user id"
//	@Failure		403	{object}	errors.APIError	"Admin access required"
//	@Failure		404	{object}	errors.APIError	"User not found"
//	@Failure		409	{object}	errors.APIError	"User still owns organizations or groups"
//	@Failure		500	{object}	errors.APIError	"Erasure failed (retryable)"
//
//	@Router			/users/{id} [delete]
func (u userController) DeleteUser(ctx *gin.Context) {
	userRoles := ctx.GetStringSlice("userRoles")
	isAdmin := access.IsAdmin(userRoles)
	if !isAdmin {
		errors.Respond(ctx, http.StatusForbidden, "Admin access required")
		ctx.Abort()
		return
	}

	idParam := ctx.Param("id")

	id, parseError := uuid.Parse(idParam)

	if parseError != nil {
		errors.Respond(ctx, http.StatusBadRequest, parseError.Error())
		ctx.Abort()
		return
	}

	// Same erasure flow as the self-service deletion (issue #490): the OCF
	// cascade must run whoever triggers the deletion.
	if err := u.deletionService.EraseUser(id.String()); err != nil {
		status, message := services.ErasureErrorResponse(err)
		if status == http.StatusInternalServerError {
			utils.Error("Admin erasure failed for user %s: %v", id, err)
		}
		errors.Respond(ctx, status, message)
		ctx.Abort()
		return
	}

	// Remove all policies for this user
	opts := utils.DefaultPermissionOptions()
	opts.WarnOnError = true
	utils.RemovePolicy(casdoor.Enforcer, id.String(), "", "", opts)

	ctx.JSON(http.StatusNoContent, "Done")
}
