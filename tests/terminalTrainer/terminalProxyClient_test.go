package terminalTrainer_tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/terminalTrainer/services"

	"github.com/stretchr/testify/assert"
)

// These tests exercise the extracted terminalProxyClient end-to-end through the
// public TerminalTrainerService facade. They prove the tt-backend HTTP layer
// still behaves identically after being carved out of terminalTrainerService:
// distributions are parsed from the stub, and the backend-list cache (whose
// state now lives on terminalProxyClient) still coalesces repeated reads into a
// single upstream call.

// newProxyTestService wires a TerminalTrainerService against an httptest stub,
// mirroring the env-var setup used by the other tt-backend HTTP tests.
func newProxyTestService(t *testing.T, serverURL string) services.TerminalTrainerService {
	t.Helper()
	db := freshTestDB(t)
	t.Setenv("TERMINAL_TRAINER_URL", serverURL)
	t.Setenv("TERMINAL_TRAINER_ADMIN_KEY", "test-admin-key")
	t.Setenv("TERMINAL_TRAINER_API_VERSION", "1.0")
	return services.NewTerminalTrainerService(db)
}

// TestProxy_GetDistributions_ParsedThroughFacade proves the moved
// GetDistributions method and its facade wrapper round-trip a tt-backend
// response into typed DTOs.
func TestProxy_GetDistributions_ParsedThroughFacade(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/1.0/distributions" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]dto.TTDistribution{
				{Name: "debian-12", Prefix: "deb12", MinSizeKey: "XS"},
				{Name: "ubuntu-24", Prefix: "ubu24", MinSizeKey: "S"},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc := newProxyTestService(t, srv.URL)

	distros, err := svc.GetDistributions("")
	assert.NoError(t, err)
	assert.Len(t, distros, 2)
	assert.Equal(t, "debian-12", distros[0].Name)
	assert.Equal(t, "XS", distros[0].MinSizeKey)
	assert.Equal(t, "ubuntu-24", distros[1].Name)
}

// TestProxy_BackendCache_CoalescesRepeatedReads proves the backend-list cache
// moved correctly onto terminalProxyClient: IsBackendOnline reads through the
// cached path, so a second call within the TTL must NOT issue a second upstream
// HTTP request.
func TestProxy_BackendCache_CoalescesRepeatedReads(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/1.0/backends" {
			hits++
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]dto.BackendInfo{
				{ID: "incus-1", Name: "Incus 1", Connected: true},
				{ID: "incus-2", Name: "Incus 2", Connected: false},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc := newProxyTestService(t, srv.URL)

	online, err := svc.IsBackendOnline("incus-1")
	assert.NoError(t, err)
	assert.True(t, online)

	offline, err := svc.IsBackendOnline("incus-2")
	assert.NoError(t, err)
	assert.False(t, offline)

	// Both reads were served from the single cached fetch.
	assert.Equal(t, 1, hits, "backend list should be fetched once and cached")
}
