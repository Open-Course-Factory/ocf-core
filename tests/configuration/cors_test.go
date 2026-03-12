package configuration_tests

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	config "soli/formations/src/configuration"
)

// cleanupCORSEnv unsets CORS-related environment variables after a test.
// Each test must call its own InitAllowedOrigins() after setting env vars.
func cleanupCORSEnv(t *testing.T) {
	t.Helper()
	os.Unsetenv("ENVIRONMENT")
	os.Unsetenv("FRONTEND_URL")
	os.Unsetenv("ADMIN_FRONTEND_URL")
}

// --- InitAllowedOrigins tests ---

func TestInitAllowedOrigins_DevMode_SetsAllowLocalhost(t *testing.T) {
	t.Cleanup(func() { cleanupCORSEnv(t) })
	cleanupCORSEnv(t)

	os.Setenv("ENVIRONMENT", "development")
	origins := config.InitAllowedOrigins()

	assert.Empty(t, origins, "No explicit origins should be returned when none are configured")
	assert.True(t, config.IsOriginAllowed("http://localhost:4000"),
		"localhost should be allowed in development mode")
}

func TestInitAllowedOrigins_EmptyEnvironment_SetsAllowLocalhost(t *testing.T) {
	t.Cleanup(func() { cleanupCORSEnv(t) })
	cleanupCORSEnv(t)

	os.Setenv("ENVIRONMENT", "")
	config.InitAllowedOrigins()

	assert.True(t, config.IsOriginAllowed("http://localhost:3000"),
		"localhost should be allowed when ENVIRONMENT is empty")
}

func TestInitAllowedOrigins_UnsetEnvironment_SetsAllowLocalhost(t *testing.T) {
	t.Cleanup(func() { cleanupCORSEnv(t) })
	cleanupCORSEnv(t)

	// ENVIRONMENT is unset by cleanupCORSEnv
	config.InitAllowedOrigins()

	assert.True(t, config.IsOriginAllowed("http://localhost:8080"),
		"localhost should be allowed when ENVIRONMENT is unset")
}

func TestInitAllowedOrigins_NoOrigins_SetsAllowLocalhost(t *testing.T) {
	t.Cleanup(func() { cleanupCORSEnv(t) })
	cleanupCORSEnv(t)

	// Even in production, if no origins are configured, allowLocalhost is set
	os.Setenv("ENVIRONMENT", "production")
	origins := config.InitAllowedOrigins()

	assert.Empty(t, origins, "No origins should be returned when none configured")
	assert.True(t, config.IsOriginAllowed("http://localhost:4000"),
		"localhost should be allowed when no origins configured (fallback to dev behavior)")
}

func TestInitAllowedOrigins_ProductionWithOrigins_DoesNotAllowLocalhost(t *testing.T) {
	t.Cleanup(func() { cleanupCORSEnv(t) })
	cleanupCORSEnv(t)

	os.Setenv("ENVIRONMENT", "production")
	os.Setenv("FRONTEND_URL", "https://ocf.example.com")
	origins := config.InitAllowedOrigins()

	assert.Equal(t, []string{"https://ocf.example.com"}, origins)
	assert.False(t, config.IsOriginAllowed("http://localhost:4000"),
		"localhost should NOT be allowed in production with explicit origins")
}

func TestInitAllowedOrigins_ReturnsBothFrontendAndAdminURLs(t *testing.T) {
	t.Cleanup(func() { cleanupCORSEnv(t) })
	cleanupCORSEnv(t)

	os.Setenv("ENVIRONMENT", "production")
	os.Setenv("FRONTEND_URL", "https://ocf.example.com")
	os.Setenv("ADMIN_FRONTEND_URL", "https://admin.example.com")
	origins := config.InitAllowedOrigins()

	assert.Len(t, origins, 2)
	assert.Contains(t, origins, "https://ocf.example.com")
	assert.Contains(t, origins, "https://admin.example.com")
}

func TestInitAllowedOrigins_DevWithOrigins_StillAllowsLocalhost(t *testing.T) {
	t.Cleanup(func() { cleanupCORSEnv(t) })
	cleanupCORSEnv(t)

	os.Setenv("ENVIRONMENT", "development")
	os.Setenv("FRONTEND_URL", "https://ocf.example.com")
	origins := config.InitAllowedOrigins()

	assert.Equal(t, []string{"https://ocf.example.com"}, origins)
	// In development mode, localhost is always allowed even with explicit origins
	assert.True(t, config.IsOriginAllowed("http://localhost:9999"),
		"localhost should still be allowed in development mode even with explicit origins")
	assert.True(t, config.IsOriginAllowed("https://ocf.example.com"),
		"Explicitly configured origin should be allowed")
}

// --- GetAllowedOrigins tests ---

func TestGetAllowedOrigins_ReturnsConfiguredOrigins(t *testing.T) {
	t.Cleanup(func() { cleanupCORSEnv(t) })
	cleanupCORSEnv(t)

	os.Setenv("ENVIRONMENT", "production")
	os.Setenv("FRONTEND_URL", "https://ocf.example.com")
	os.Setenv("ADMIN_FRONTEND_URL", "https://admin.example.com")
	config.InitAllowedOrigins()

	origins := config.GetAllowedOrigins()
	assert.Len(t, origins, 2)
	assert.Contains(t, origins, "https://ocf.example.com")
	assert.Contains(t, origins, "https://admin.example.com")
}

// --- IsOriginAllowed: localhost in dev mode ---

func TestIsOriginAllowed_LocalhostVariousPortsInDevMode(t *testing.T) {
	t.Cleanup(func() { cleanupCORSEnv(t) })
	cleanupCORSEnv(t)

	os.Setenv("ENVIRONMENT", "development")
	config.InitAllowedOrigins()

	tests := []struct {
		name   string
		origin string
	}{
		{"localhost_4000", "http://localhost:4000"},
		{"localhost_3000", "http://localhost:3000"},
		{"localhost_8080", "http://localhost:8080"},
		{"localhost_5173", "http://localhost:5173"},
		{"localhost_443_https", "https://localhost:443"},
		{"localhost_no_port", "http://localhost"},
		{"127.0.0.1_4000", "http://127.0.0.1:4000"},
		{"127.0.0.1_8080", "http://127.0.0.1:8080"},
		{"127.0.0.1_no_port", "http://127.0.0.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.True(t, config.IsOriginAllowed(tt.origin),
				"Origin %s should be allowed in dev mode", tt.origin)
		})
	}
}

func TestIsOriginAllowed_NonLocalhostRejectedInDevMode(t *testing.T) {
	t.Cleanup(func() { cleanupCORSEnv(t) })
	cleanupCORSEnv(t)

	os.Setenv("ENVIRONMENT", "development")
	config.InitAllowedOrigins()

	tests := []struct {
		name   string
		origin string
	}{
		{"external_domain", "https://evil.example.com"},
		{"external_ip", "http://192.168.1.1:4000"},
		{"localhost_in_path_only", "https://evil.com/localhost"},
		{"subdomain_of_localhost", "http://sub.localhost:4000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.False(t, config.IsOriginAllowed(tt.origin),
				"Non-localhost origin %s should be rejected in dev mode", tt.origin)
		})
	}
}

// --- IsOriginAllowed: configured origins matching ---

func TestIsOriginAllowed_ExactMatchForConfiguredOrigins(t *testing.T) {
	t.Cleanup(func() { cleanupCORSEnv(t) })
	cleanupCORSEnv(t)

	os.Setenv("ENVIRONMENT", "production")
	os.Setenv("FRONTEND_URL", "https://ocf.example.com")
	os.Setenv("ADMIN_FRONTEND_URL", "https://admin.example.com")
	config.InitAllowedOrigins()

	assert.True(t, config.IsOriginAllowed("https://ocf.example.com"),
		"Exact match for FRONTEND_URL should be allowed")
	assert.True(t, config.IsOriginAllowed("https://admin.example.com"),
		"Exact match for ADMIN_FRONTEND_URL should be allowed")
}

func TestIsOriginAllowed_CaseInsensitiveMatch(t *testing.T) {
	t.Cleanup(func() { cleanupCORSEnv(t) })
	cleanupCORSEnv(t)

	os.Setenv("ENVIRONMENT", "production")
	os.Setenv("FRONTEND_URL", "https://ocf.example.com")
	config.InitAllowedOrigins()

	assert.True(t, config.IsOriginAllowed("HTTPS://OCF.EXAMPLE.COM"),
		"All-uppercase should match case-insensitively")
	assert.True(t, config.IsOriginAllowed("https://OCF.Example.Com"),
		"Mixed case should match case-insensitively")
}

func TestIsOriginAllowed_WebSocketSchemeHostMatch(t *testing.T) {
	t.Cleanup(func() { cleanupCORSEnv(t) })
	cleanupCORSEnv(t)

	os.Setenv("ENVIRONMENT", "production")
	os.Setenv("FRONTEND_URL", "https://ocf.example.com:4000")
	config.InitAllowedOrigins()

	assert.True(t, config.IsOriginAllowed("ws://ocf.example.com:4000"),
		"ws:// with same host:port should be allowed via host comparison")
	assert.True(t, config.IsOriginAllowed("wss://ocf.example.com:4000"),
		"wss:// with same host:port should be allowed via host comparison")
}

func TestIsOriginAllowed_DifferentSchemeMatchesViaHost(t *testing.T) {
	t.Cleanup(func() { cleanupCORSEnv(t) })
	cleanupCORSEnv(t)

	os.Setenv("ENVIRONMENT", "production")
	os.Setenv("FRONTEND_URL", "https://ocf.example.com:4000")
	config.InitAllowedOrigins()

	assert.True(t, config.IsOriginAllowed("http://ocf.example.com:4000"),
		"Same host:port with different scheme should match via host comparison")
}

func TestIsOriginAllowed_UnknownOriginRejected(t *testing.T) {
	t.Cleanup(func() { cleanupCORSEnv(t) })
	cleanupCORSEnv(t)

	os.Setenv("ENVIRONMENT", "production")
	os.Setenv("FRONTEND_URL", "https://ocf.example.com")
	config.InitAllowedOrigins()

	tests := []struct {
		name   string
		origin string
	}{
		{"different_domain", "https://evil.example.com"},
		{"different_port", "https://ocf.example.com:9999"},
		{"different_host", "http://other.example.com"},
		{"ip_address", "http://192.168.1.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.False(t, config.IsOriginAllowed(tt.origin),
				"Unknown origin %s should be rejected", tt.origin)
		})
	}
}

// --- Edge cases ---

func TestIsOriginAllowed_EmptyOrigin(t *testing.T) {
	t.Cleanup(func() { cleanupCORSEnv(t) })
	cleanupCORSEnv(t)

	os.Setenv("ENVIRONMENT", "production")
	os.Setenv("FRONTEND_URL", "https://ocf.example.com")
	config.InitAllowedOrigins()

	assert.False(t, config.IsOriginAllowed(""),
		"Empty origin should be rejected")
}

func TestIsOriginAllowed_EmptyOriginInDevMode(t *testing.T) {
	t.Cleanup(func() { cleanupCORSEnv(t) })
	cleanupCORSEnv(t)

	os.Setenv("ENVIRONMENT", "development")
	config.InitAllowedOrigins()

	// Empty origin hostname is "", not "localhost" or "127.0.0.1"
	assert.False(t, config.IsOriginAllowed(""),
		"Empty origin should be rejected even in dev mode")
}

func TestIsOriginAllowed_OriginWithoutPort(t *testing.T) {
	t.Cleanup(func() { cleanupCORSEnv(t) })
	cleanupCORSEnv(t)

	os.Setenv("ENVIRONMENT", "production")
	os.Setenv("FRONTEND_URL", "https://ocf.example.com")
	config.InitAllowedOrigins()

	assert.True(t, config.IsOriginAllowed("https://ocf.example.com"),
		"Origin without explicit port should match configured origin without port")
}

func TestIsOriginAllowed_OriginWithPathMatchesViaHost(t *testing.T) {
	t.Cleanup(func() { cleanupCORSEnv(t) })
	cleanupCORSEnv(t)

	os.Setenv("ENVIRONMENT", "production")
	os.Setenv("FRONTEND_URL", "https://ocf.example.com")
	config.InitAllowedOrigins()

	// Origin with a path won't exact-match the configured origin,
	// but the host fallback comparison (originURL.Host == allowedURL.Host) matches
	assert.True(t, config.IsOriginAllowed("https://ocf.example.com/some/path"),
		"Origin with path should match via host comparison fallback")
}

func TestIsOriginAllowed_MalformedURL(t *testing.T) {
	t.Cleanup(func() { cleanupCORSEnv(t) })
	cleanupCORSEnv(t)

	os.Setenv("ENVIRONMENT", "development")
	config.InitAllowedOrigins()

	assert.False(t, config.IsOriginAllowed("://"),
		"Malformed URL '://' should be rejected")
}

func TestIsOriginAllowed_JustHostnameNoScheme(t *testing.T) {
	t.Cleanup(func() { cleanupCORSEnv(t) })
	cleanupCORSEnv(t)

	os.Setenv("ENVIRONMENT", "development")
	config.InitAllowedOrigins()

	// "localhost:4000" without scheme — url.Parse treats "localhost" as scheme,
	// opaque as "4000", Hostname() returns "", so this should be rejected
	assert.False(t, config.IsOriginAllowed("localhost:4000"),
		"localhost:4000 without scheme should be rejected (parsed as scheme:opaque)")
}

func TestIsOriginAllowed_DifferentPortRejected(t *testing.T) {
	t.Cleanup(func() { cleanupCORSEnv(t) })
	cleanupCORSEnv(t)

	os.Setenv("ENVIRONMENT", "production")
	os.Setenv("FRONTEND_URL", "https://ocf.example.com:4000")
	config.InitAllowedOrigins()

	assert.False(t, config.IsOriginAllowed("https://ocf.example.com:5000"),
		"Different port should not match")
}

func TestIsOriginAllowed_OnlyFrontendURLSet(t *testing.T) {
	t.Cleanup(func() { cleanupCORSEnv(t) })
	cleanupCORSEnv(t)

	os.Setenv("ENVIRONMENT", "production")
	os.Setenv("FRONTEND_URL", "https://ocf.example.com")
	origins := config.InitAllowedOrigins()

	assert.Len(t, origins, 1)
	assert.True(t, config.IsOriginAllowed("https://ocf.example.com"))
	assert.False(t, config.IsOriginAllowed("https://admin.example.com"))
}

func TestIsOriginAllowed_OnlyAdminURLSet(t *testing.T) {
	t.Cleanup(func() { cleanupCORSEnv(t) })
	cleanupCORSEnv(t)

	os.Setenv("ENVIRONMENT", "production")
	os.Setenv("ADMIN_FRONTEND_URL", "https://admin.example.com")
	origins := config.InitAllowedOrigins()

	assert.Len(t, origins, 1)
	assert.True(t, config.IsOriginAllowed("https://admin.example.com"))
}
