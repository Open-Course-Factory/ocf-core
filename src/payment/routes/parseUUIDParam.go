package paymentController

import (
	"net/http"

	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// parseUUIDParam responds with 400 and the given message when raw is not a UUID.
func parseUUIDParam(ctx *gin.Context, raw, invalidMessage string) (uuid.UUID, bool) {
	id, err := uuid.Parse(raw)
	if err != nil {
		errors.Respond(ctx, http.StatusBadRequest, invalidMessage)
		return uuid.Nil, false
	}
	return id, true
}
