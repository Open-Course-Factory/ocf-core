// tests/terminalTrainer/consoleGateStoppedContainer_test.go
//
// Bug under test: when a learner's container stops for any reason (crash, OOM,
// host action, in-container poweroff), the console gate let them straight back
// in. ValidateSessionAccess's forced API check read GET /info's legacy `status`
// field, which cannot express "the container is present but stopped" — it only
// distinguishes "instance record exists" from "instance is gone". So a dead
// environment answered status=0, the gate said active, the exec failed, the
// socket closed 1006, and the frontend told the learner their environment was
// still running and invited them to Reconnect. Forever.
//
// tt-backend now reports instance_running on /info (see its
// fix/detect-stopped-containers). This pins that the gate believes it, and —
// the part that matters more — that it stays backward compatible with a
// tt-backend that does not send the field yet, since ocf-core and tt-backend
// deploy independently.
package terminalTrainer_tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/terminalTrainer/services"
)

// newInfoOnlyTTBackend serves GET /1.0/info with the given body for any
// session, which is all ValidateSessionAccess's API check consults.
func newInfoOnlyTTBackend(t *testing.T, info map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(info); err != nil {
			t.Errorf("failed to encode /info response: %v", err)
		}
	}))
}

func TestValidateSessionAccess_RefusesTheConsoleWhenTheContainerIsStopped(t *testing.T) {
	db := setupTestDB(t)
	terminal, err := createTestTerminal(db, "console-gate-stopped", "running", time.Now().Add(time.Hour))
	require.NoError(t, err)

	// A stopped-but-present container: the instance record still exists, so
	// the legacy status stays 0. Only instance_running tells the truth.
	ttSrv := newInfoOnlyTTBackend(t, map[string]any{
		"id":               terminal.SessionID,
		"status":           0,
		"instance_exists":  true,
		"instance_running": false,
	})
	defer ttSrv.Close()
	configureTTServer(t, ttSrv.URL)

	svc := services.NewTerminalTrainerService(db)
	valid, reason, err := svc.ValidateSessionAccess(terminal.SessionID, true)

	require.NoError(t, err)
	assert.False(t, valid,
		"a stopped container must not pass the console gate — letting the learner "+
			"connect is what produced the infinite Reconnect loop")
	assert.NotEqual(t, "active", reason,
		"the refusal must carry a reason the frontend can render honestly, got %q", reason)
}

// TestValidateSessionAccess_StillAdmitsARunningContainer is the no-regression
// half: the ordinary case must be untouched.
func TestValidateSessionAccess_StillAdmitsARunningContainer(t *testing.T) {
	db := setupTestDB(t)
	terminal, err := createTestTerminal(db, "console-gate-running", "running", time.Now().Add(time.Hour))
	require.NoError(t, err)

	ttSrv := newInfoOnlyTTBackend(t, map[string]any{
		"id":               terminal.SessionID,
		"status":           0,
		"instance_exists":  true,
		"instance_running": true,
	})
	defer ttSrv.Close()
	configureTTServer(t, ttSrv.URL)

	svc := services.NewTerminalTrainerService(db)
	valid, reason, err := svc.ValidateSessionAccess(terminal.SessionID, true)

	require.NoError(t, err)
	assert.True(t, valid, "a running container must still pass the gate, got reason %q", reason)
	assert.Equal(t, "active", reason)
}

// TestValidateSessionAccess_TrustsTheLegacyStatusWhenTheFieldIsAbsent covers
// the deploy window. ocf-core and tt-backend ship independently, so ocf-core
// will run against a tt-backend that predates instance_running. An absent
// field must mean "no opinion" and fall back to the legacy status — never
// "not running", which would lock every learner out of a healthy terminal.
func TestValidateSessionAccess_TrustsTheLegacyStatusWhenTheFieldIsAbsent(t *testing.T) {
	db := setupTestDB(t)
	terminal, err := createTestTerminal(db, "console-gate-legacy", "running", time.Now().Add(time.Hour))
	require.NoError(t, err)

	ttSrv := newInfoOnlyTTBackend(t, map[string]any{
		"id":              terminal.SessionID,
		"status":          0,
		"instance_exists": true,
	})
	defer ttSrv.Close()
	configureTTServer(t, ttSrv.URL)

	svc := services.NewTerminalTrainerService(db)
	valid, reason, err := svc.ValidateSessionAccess(terminal.SessionID, true)

	require.NoError(t, err)
	assert.True(t, valid,
		"an older tt-backend sends no instance_running; treating its silence as "+
			"'not running' would lock every learner out of a healthy terminal")
	assert.Equal(t, "active", reason)
}

// TestValidateSessionAccess_StillReportsAVanishedInstanceAsExpired pins that
// the pre-existing absent-instance path is unchanged.
func TestValidateSessionAccess_StillReportsAVanishedInstanceAsExpired(t *testing.T) {
	db := setupTestDB(t)
	terminal, err := createTestTerminal(db, "console-gate-absent", "running", time.Now().Add(time.Hour))
	require.NoError(t, err)

	ttSrv := newInfoOnlyTTBackend(t, map[string]any{
		"id":               terminal.SessionID,
		"status":           6,
		"instance_exists":  false,
		"instance_running": false,
	})
	defer ttSrv.Close()
	configureTTServer(t, ttSrv.URL)

	svc := services.NewTerminalTrainerService(db)
	valid, reason, err := svc.ValidateSessionAccess(terminal.SessionID, true)

	require.NoError(t, err)
	assert.False(t, valid)
	assert.Equal(t, "expired", reason,
		"a vanished instance keeps its existing wire format, which the frontend "+
			"maps to Session ended")
}
