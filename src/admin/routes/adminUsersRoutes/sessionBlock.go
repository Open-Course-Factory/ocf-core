package adminUsersRoutes

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	apiErrors "soli/formations/src/auth/errors"
	"soli/formations/src/terminalTrainer/httperrors"
	ttServices "soli/formations/src/terminalTrainer/services"
	"soli/formations/src/utils"
)

// NewSetSessionBlockHandler returns the handler for
// PUT /admin/users/:id/session-block. It blocks (or unblocks) the user from
// starting any session on the platform. Blocking also disables their terminal
// key, after the optional stop of their running sessions (the stop uses that
// key); unblocking gives a key back.
func NewSetSessionBlockHandler(db *gorm.DB) gin.HandlerFunc {
	participants := ttServices.NewParticipantControlsService(db)
	return func(ctx *gin.Context) {
		if !requireAdmin(ctx) {
			return
		}
		targetID := ctx.Param("id")
		// An unknown account gets nothing: no block, no audit row naming it.
		if user, err := GetCasdoorUserByID(targetID); err != nil || user == nil {
			apiErrors.Abort(ctx, http.StatusNotFound, "user_not_found")
			return
		}
		input, ok := httperrors.BindSessionBlock(ctx)
		if !ok {
			return
		}
		actor := httperrors.ControlActorFrom(ctx)
		if targetID == actor.UserID {
			httperrors.WriteParticipantControlError(ctx, ttServices.ErrParticipantForbidden)
			return
		}

		results, removed, err := participants.SetSessionBlock(
			actor, targetID, ttServices.GlobalControlScope(), *input.Blocked, input.Reason, input.StopRunning)
		// An unblock that lifted nothing leaves the terminal key as it is.
		if err == nil && (*input.Blocked || removed) {
			setTerminalKeyAfterBlock(db, targetID, *input.Blocked)
		}
		httperrors.WriteSessionBlockResult(ctx, *input.Blocked, results, err)
	}
}

// setTerminalKeyAfterBlock disables (blocked) or gives back the user's
// terminal key. Best-effort, as for an account block: the block itself
// already refuses every session start and key regeneration in ocf-core.
func setTerminalKeyAfterBlock(db *gorm.DB, userID string, blocked bool) {
	if blocked {
		if err := DisableUserTerminalKey(db, userID); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			utils.Warn("could not disable terminal key of session-blocked user %s: %v", userID, err)
		}
		return
	}
	if err := EnableUserTerminalKey(db, userID); err != nil {
		utils.Warn("could not give back the terminal key of unblocked user %s: %v", userID, err)
	}
}
