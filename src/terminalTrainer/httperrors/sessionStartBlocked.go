package httperrors

import (
	stderrors "errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"soli/formations/src/terminalTrainer/models"
)

// WriteSessionStartBlocked answers 403 session_start_blocked, with the scope of
// the block and the reason the participant is shown, and reports whether err
// was that refusal.
func WriteSessionStartBlocked(ctx *gin.Context, err error) bool {
	var blocked *models.SessionStartBlockedError
	if !stderrors.As(err, &blocked) {
		return false
	}
	ctx.JSON(http.StatusForbidden, gin.H{
		"error_code":    http.StatusForbidden,
		"error_message": blocked.Error(),
		"scope":         blocked.Scope,
		"reason":        blocked.Reason,
	})
	return true
}
