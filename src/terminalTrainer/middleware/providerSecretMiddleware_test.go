package terminalMiddleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func callWithSecret(t *testing.T, configured, provided string) int {
	t.Setenv("TRAEFIK_PROVIDER_SECRET", configured)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/x", RequireProviderSecret(), func(c *gin.Context) { c.Status(http.StatusOK) })
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	if provided != "" {
		req.Header.Set("X-Provider-Secret", provided)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code
}

func TestRequireProviderSecret(t *testing.T) {
	assert.Equal(t, http.StatusOK, callWithSecret(t, "s3cret", "s3cret"))
	assert.Equal(t, http.StatusUnauthorized, callWithSecret(t, "s3cret", "wrong"))
	assert.Equal(t, http.StatusUnauthorized, callWithSecret(t, "s3cret", ""))
	// An unconfigured secret must never fail open.
	assert.Equal(t, http.StatusUnauthorized, callWithSecret(t, "", ""))
}
