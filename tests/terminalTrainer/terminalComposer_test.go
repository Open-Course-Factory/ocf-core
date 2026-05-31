// Behavioural tests for terminalComposer, exercised through the public
// TerminalTrainerService facade (StartComposedSession) over the production
// HTTP/middleware chain against an httptest stub of tt-backend.
//
// These complement composedSession_http_test.go (which pins the rejection
// body shape) by asserting the DB side-effects the composer owns: on the
// happy path it must persist a Terminal row carrying the chosen
// distribution/size; on a budget rejection it must persist nothing. We
// assert real DB rows, never mock calls.
package terminalTrainer_tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	terminalServices "soli/formations/src/terminalTrainer/services"
)

// TestComposer_StartComposedSession_PersistsChosenSizeAndDistribution —
// happy path. A within-budget composed request must persist exactly one
// Terminal row whose ComposedDistribution / ComposedSize / MachineSize
// match the request. This pins the composer's startComposedSession
// persistence step (the tt-backend POST → repository.CreateTerminalSession).
func TestComposer_StartComposedSession_PersistsChosenSizeAndDistribution(t *testing.T) {
	ttServer := startComposedTTBackendStub(t)
	defer ttServer.Close()
	t.Setenv("TERMINAL_TRAINER_URL", ttServer.URL)
	t.Setenv("TERMINAL_TRAINER_ADMIN_KEY", "test-admin-key")
	t.Setenv("TERMINAL_TRAINER_API_VERSION", "1.0")

	freshTestDB(t)
	userID := "composer-persist-user"

	seedBudgetPlanForUser(t, userID, 8000, 4096)
	_, err := createTestUserKey(sharedTestDB, userID)
	require.NoError(t, err)

	svc := terminalServices.NewTerminalTrainerService(sharedTestDB)
	router := setupBudgetHTTPRouter(t, userID, svc)

	body, _ := json.Marshal(map[string]any{
		"distribution": "ubuntu-24.04",
		"size":         "M",
		"terms":        "accepted",
	})
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/terminals/start-composed-session", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code,
		"within-budget composed request must succeed — got %d. Body: %s", w.Code, w.Body.String())

	// The composer persists the composed footprint; assert the row carries
	// the requested distribution and size (MachineSize is upper-cased).
	var cnt int64
	require.NoError(t, sharedTestDB.Raw(
		`SELECT COUNT(*) FROM terminals WHERE user_id = ? AND composed_distribution = ? AND composed_size = ? AND machine_size = ?`,
		userID, "ubuntu-24.04", "M", "M",
	).Scan(&cnt).Error)
	assert.EqualValues(t, 1, cnt,
		"composer must persist one Terminal carrying composed_distribution=ubuntu-24.04, composed_size=M, machine_size=M")
}

// TestComposer_StartComposedSession_OverBudget_PersistsNothing — the
// budget gate runs before the tt-backend POST, so an over-budget request
// must be rejected with 403 AND leave the terminals table untouched
// (beyond the pre-seeded consumer). This pins that the composer does not
// create a row when the budget rejects.
func TestComposer_StartComposedSession_OverBudget_PersistsNothing(t *testing.T) {
	ttServer := startComposedTTBackendStub(t)
	defer ttServer.Close()
	t.Setenv("TERMINAL_TRAINER_URL", ttServer.URL)
	t.Setenv("TERMINAL_TRAINER_ADMIN_KEY", "test-admin-key")
	t.Setenv("TERMINAL_TRAINER_API_VERSION", "1.0")

	freshTestDB(t)
	userID := "composer-overbudget-user"

	// Plan: 4 vCPU / 2048 MiB. A pre-existing L consumes the whole budget.
	seedBudgetPlanForUser(t, userID, 4000, 2048)
	_, err := createTestUserKey(sharedTestDB, userID)
	require.NoError(t, err)
	insertExistingTerminal(t, sharedTestDB, userID, nil, "running", "ephemeral", 4000, 2048)

	svc := terminalServices.NewTerminalTrainerService(sharedTestDB)
	router := setupBudgetHTTPRouter(t, userID, svc)

	body, _ := json.Marshal(map[string]any{
		"distribution": "ubuntu-24.04",
		"size":         "L",
		"terms":        "accepted",
	})
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/terminals/start-composed-session", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code,
		"over-budget composed request must be rejected with 403 — got %d. Body: %s", w.Code, w.Body.String())

	// Only the pre-seeded consumer must exist — the rejected request must
	// not have persisted a second row.
	var cnt int64
	require.NoError(t, sharedTestDB.Raw(
		`SELECT COUNT(*) FROM terminals WHERE user_id = ?`, userID,
	).Scan(&cnt).Error)
	assert.EqualValues(t, 1, cnt,
		"a budget-rejected composed request must not persist a Terminal row")
}
