package httperrors

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"soli/formations/src/auth/access"
	apiErrors "soli/formations/src/auth/errors"
	"soli/formations/src/terminalTrainer/dto"
	services "soli/formations/src/terminalTrainer/services"
)

// ControlActorFrom reads who acts on a participant from the request context.
func ControlActorFrom(ctx *gin.Context) services.ControlActor {
	return services.ControlActor{
		UserID:         ctx.GetString("userId"),
		ImpersonatorID: ctx.GetString("impersonatorId"),
		IsAdmin:        access.IsAdmin(ctx.GetStringSlice("userRoles")),
	}
}

// WriteParticipantControlError answers a participant-control service error:
// 403 for a refusal, 400 for a reason too long, 500 otherwise.
func WriteParticipantControlError(ctx *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrParticipantForbidden):
		apiErrors.Respond(ctx, http.StatusForbidden, "You may not act on this participant")
	case errors.Is(err, services.ErrSessionBlockReasonTooLong):
		apiErrors.Respond(ctx, http.StatusBadRequest, "The reason is limited to 500 characters")
	default:
		slog.Error("participant control failed", "err", err)
		apiErrors.Respond(ctx, http.StatusInternalServerError, "The action could not be carried out, nothing was changed")
	}
}

// WriteSessionActionResults answers an action on several sessions with
// {"results": [...]}: 200 when at least one session succeeded or none was
// attempted, otherwise the status and code of the first failure, the code
// also at the top level.
func WriteSessionActionResults(ctx *gin.Context, results []services.SessionActionResult) {
	if results == nil {
		results = []services.SessionActionResult{}
	}
	var firstFailure *services.SessionActionResult
	succeeded := false
	for i := range results {
		switch {
		case results[i].Skipped:
		case results[i].OK:
			succeeded = true
		case firstFailure == nil:
			firstFailure = &results[i]
		}
	}
	if firstFailure != nil && !succeeded {
		ctx.JSON(firstFailure.Status, gin.H{"results": results, "code": firstFailure.Code})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"results": results})
}

// BindSessionBlock reads a session-block body, answering 400 when it is
// malformed or lacks blocked.
func BindSessionBlock(ctx *gin.Context) (dto.SetSessionBlockInput, bool) {
	var input dto.SetSessionBlockInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		apiErrors.Respond(ctx, http.StatusBadRequest, "blocked is required")
		return input, false
	}
	return input, true
}

// WriteSessionBlockResult answers a session-block call: the block state and
// the outcome of the sessions it stopped.
func WriteSessionBlockResult(ctx *gin.Context, blocked bool, results []services.SessionActionResult, err error) {
	if err != nil {
		WriteParticipantControlError(ctx, err)
		return
	}
	if results == nil {
		results = []services.SessionActionResult{}
	}
	ctx.JSON(http.StatusOK, gin.H{"blocked": blocked, "results": results})
}
