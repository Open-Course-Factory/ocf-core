package httperrors

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	apiErrors "soli/formations/src/auth/errors"
	services "soli/formations/src/terminalTrainer/services"
)

// WriteHostUnavailable answers 503 when no allowed host could take the
// session, and reports whether it did. The capacity wording is the one the
// frontend has always received.
func WriteHostUnavailable(ctx *gin.Context, err error) bool {
	switch {
	case errors.Is(err, services.ErrNoBackendCapacity):
		apiErrors.Respond(ctx, http.StatusServiceUnavailable, "Server at capacity. Please try again later.")
	case errors.Is(err, services.ErrBackendOffline):
		apiErrors.Respond(ctx, http.StatusServiceUnavailable, "No terminal server is available right now. Please try again later.")
	default:
		return false
	}
	return true
}
