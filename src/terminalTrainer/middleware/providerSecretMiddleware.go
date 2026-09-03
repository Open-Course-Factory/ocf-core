package terminalMiddleware

import (
	"crypto/subtle"
	"net/http"
	"os"

	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

// RequireProviderSecret protects service-to-service endpoints meant to be
// polled by an external process (Traefik's HTTP provider) rather than
// called by an authenticated user — there is no JWT to check here, so this
// is the first inbound-secret check in ocf-core (every other X-API-Key
// usage in this module is outbound, toward tt-backend).
//
// The secret is read from TRAEFIK_PROVIDER_SECRET on every request rather
// than captured once at startup, so rotating the env var and restarting
// Traefik (but not ocf-core) still works. An empty configured secret always
// denies — this must never fail open just because the operator hasn't set
// EXPOSE_DOMAIN/TRAEFIK_PROVIDER_SECRET (see
// services.IsExposedPortsFeatureEnabled, which keeps the route unmounted in
// that case; this check is the belt to that suspenders).
func RequireProviderSecret() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		expected := os.Getenv("TRAEFIK_PROVIDER_SECRET")
		provided := ctx.GetHeader("X-Provider-Secret")

		if expected == "" || provided == "" ||
			subtle.ConstantTimeCompare([]byte(expected), []byte(provided)) != 1 {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, &errors.APIError{
				ErrorCode:    http.StatusUnauthorized,
				ErrorMessage: "invalid or missing provider secret",
			})
			return
		}

		ctx.Next()
	}
}
