package terminalController

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubTraefikService struct{ rows []exposedPortRow }

func (s stubTraefikService) GetActiveExposedPortsForTraefik() ([]exposedPortRow, error) {
	return s.rows, nil
}

func dynamicConfigFor(t *testing.T, scheme string, rows []exposedPortRow) traefikDynamicConfig {
	t.Setenv("EXPOSE_DOMAIN", "expose.example")
	t.Setenv("EXPOSE_SCHEME", scheme)
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/internal/traefik/dynamic-config", nil)
	getDynamicConfig(ctx, stubTraefikService{rows: rows})
	require.Equal(t, http.StatusOK, w.Code)
	var cfg traefikDynamicConfig
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &cfg))
	return cfg
}

func TestDynamicConfig_OneRouterAndServicePerExposure(t *testing.T) {
	cfg := dynamicConfigFor(t, "", []exposedPortRow{{Slug: "abc", ContainerIP: "10.1.2.3", ContainerPort: 8000}})

	router, ok := cfg.HTTP.Routers["abc"]
	require.True(t, ok)
	assert.Equal(t, "Host(`abc.expose.example`)", router.Rule)
	assert.Equal(t, "abc", router.Service)
	assert.Nil(t, router.TLS, "plain http: no tls block")
	assert.Equal(t, "http://10.1.2.3:8000", cfg.HTTP.Services["abc"].LoadBalancer.Servers[0].URL)
}

func TestDynamicConfig_HTTPSMarksRoutersTLS(t *testing.T) {
	cfg := dynamicConfigFor(t, "https", []exposedPortRow{{Slug: "abc", ContainerIP: "10.1.2.3", ContainerPort: 8000}})
	assert.NotNil(t, cfg.HTTP.Routers["abc"].TLS)
}

func TestDynamicConfig_BareObjectWhenNothingExposed(t *testing.T) {
	// Traefik v3 rejects every empty nested object; {} is the one idle payload
	// it accepts, and it clears previously published routes.
	t.Setenv("EXPOSE_DOMAIN", "expose.example")
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/internal/traefik/dynamic-config", nil)
	getDynamicConfig(ctx, stubTraefikService{})
	assert.Equal(t, "{}", w.Body.String())
}
