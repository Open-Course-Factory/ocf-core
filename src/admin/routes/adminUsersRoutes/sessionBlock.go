package adminUsersRoutes

import (
	"errors"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

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
		input, ok := httperrors.BindSessionBlock(ctx)
		if !ok {
			return
		}
		actor := httperrors.ControlActorFrom(ctx)
		targetID := ctx.Param("id")
		if targetID == actor.UserID {
			httperrors.WriteParticipantControlError(ctx, ttServices.ErrParticipantForbidden)
			return
		}

		results, err := participants.SetSessionBlock(
			actor, targetID, ttServices.GlobalControlScope(), *input.Blocked, input.Reason, input.StopRunning)
		if err == nil {
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
