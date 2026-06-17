// Regression pin for the finalize id-collision the reserve-first refactor
// introduced.
//
// The OLD CreateTerminalSession was an UPSERT keyed on session_id: if a row
// with that id already existed (active OR soft-deleted), it RESTORED/updated
// that row instead of inserting a duplicate. The reserve-first refactor split
// the flow into CreateReservation (fresh placeholder row, session_id =
// "reserving:"+uuid) + FinalizeReservation, where finalize is a blind
//
//	UPDATE terminals SET session_id = <real tt-backend id> WHERE id = <pk>
//
// (src/terminalTrainer/repositories/terminalRepository.go FinalizeReservation).
// session_id carries a PLAIN unique index (src/terminalTrainer/models/
// terminal.go). So when the real tt-backend session id matches an EXISTING
// row — e.g. a prior soft-deleted session whose id tt-backend later reuses —
// the finalize UPDATE violates the unique index. The start errors, the
// placeholder reservation is left stuck (leaking budget until its TTL lapses),
// and the container is leaked on tt-backend.
//
// This test pins the DESIRED (old) behavior: a start whose finalize collides
// with an existing (soft-deleted) session_id must still SUCCEED, leaving
// exactly one non-deleted running row for that id and no leftover placeholder,
// charging the budget exactly once. It runs on SQLite, which enforces the
// unique index, so the collision reproduces here.
package terminalTrainer_tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/terminalTrainer/dto"
	terminalModels "soli/formations/src/terminalTrainer/models"
	"soli/formations/src/terminalTrainer/services"
)

// fixedIDComposeTTBackend mirrors failingComposeTTBackend's read catalog but
// makes POST /sessions always succeed (status 0) returning a CHOSEN, fixed id
// in the "id" field (TerminalTrainerSessionResponse.SessionID reads json:"id").
// This forces the finalize UPDATE to set session_id to exactly that value, so
// a pre-existing row holding the same session_id triggers the unique-index
// collision.
func fixedIDComposeTTBackend(t *testing.T, fixedID string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/distributions"):
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"name":               "ubuntu-24.04",
					"prefix":             "ubuntu",
					"description":        "Ubuntu 24.04 LTS",
					"os_type":            "deb",
					"min_size_key":       "",
					"supported_features": []string{},
					"default_size_key":   "S",
				},
			})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/sizes"):
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"key": "XS", "name": "Extra Small", "sort_order": 10, "cpu": 1, "memory": "256MB"},
				{"key": "S", "name": "Small", "sort_order": 20, "cpu": 1, "memory": "512MB"},
				{"key": "M", "name": "Medium", "sort_order": 30, "cpu": 2, "memory": "1GB"},
				{"key": "L", "name": "Large", "sort_order": 40, "cpu": 4, "memory": "2GB"},
				{"key": "XL", "name": "Extra Large", "sort_order": 50, "cpu": 4, "memory": "4GB"},
			})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/features"):
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/sessions"):
			// Hand back the EXACT id of the pre-existing soft-deleted row, as
			// tt-backend would when it reuses a recycled session id.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":         fixedID,
				"status":     0,
				"expires_at": time.Now().Add(time.Hour).Unix(),
				"backend":    "incus",
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

// insertSoftDeletedTerminalWithSessionID seeds a SOFT-DELETED Terminal row with
// a caller-chosen session_id — the existing insertExistingTerminal* helpers
// always derive session_id from the row's own uuid and never set deleted_at, so
// neither can stand in for "a prior ended session whose id tt-backend later
// hands back". Raw SQL is used (mirroring insertExistingTerminal) so the
// BeforeCreate hook is bypassed and deleted_at can be stamped directly.
func insertSoftDeletedTerminalWithSessionID(t *testing.T, db *gorm.DB, userID, sessionID string, cpu, memMB int) {
	t.Helper()
	id := uuid.New().String()
	now := time.Now()
	err := db.Exec(`INSERT INTO terminals
		(id, created_at, updated_at, deleted_at, user_id, organization_id, session_id, state, persistence_mode, size_cpu, size_memory_mb, expires_at, machine_size, user_terminal_key_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, now, now, now, userID, nil,
		sessionID, string(terminalModels.StateDeleted), "ephemeral",
		cpu, memMB, now.Add(time.Hour), "L", uuid.New().String()).Error
	require.NoError(t, err)
}

// TestComposedSession_FinalizeOnExistingSessionID_RestoresWithoutDuplicating
// pins the desired behavior: a composed start whose tt-backend session id
// collides with an EXISTING soft-deleted row's session_id must still succeed,
// restoring/reusing the row rather than failing on the unique index — exactly
// what the old UPSERT-by-session_id CreateTerminalSession did. The current
// reserve-first finalize (a blind UPDATE) fails the unique constraint instead,
// so this test is RED until the finalize is taught to absorb the collision.
func TestComposedSession_FinalizeOnExistingSessionID_RestoresWithoutDuplicating(t *testing.T) {
	// The id tt-backend will hand back — fixed so it matches the pre-seeded
	// soft-deleted row exactly. Not the reservation placeholder prefix.
	reusedSessionID := "reuse-sess-fixed-" + uuid.New().String()

	srv := fixedIDComposeTTBackend(t, reusedSessionID)
	defer srv.Close()
	t.Setenv("TERMINAL_TRAINER_URL", srv.URL)
	t.Setenv("TERMINAL_TRAINER_ADMIN_KEY", "test-admin-key")
	t.Setenv("TERMINAL_TRAINER_API_VERSION", "1.0")

	freshTestDB(t)
	userID := "finalize-collision-user-" + uuid.New().String()[:8]

	// Budget fits EXACTLY one L. If the start both leaked the placeholder AND
	// somehow created a second row, the budget sum would read two L.
	planID := seedBudgetPlanForUser(t, userID, lSizeCPU, lSizeMemMB)
	_, err := createTestUserKey(sharedTestDB, userID)
	require.NoError(t, err)

	// A prior ended session whose id tt-backend later reuses: soft-deleted, so
	// it does NOT occupy the budget, but its session_id still owns the unique
	// index slot the finalize UPDATE will try to claim.
	insertSoftDeletedTerminalWithSessionID(t, sharedTestDB, userID, reusedSessionID, lSizeCPU, lSizeMemMB)

	plan := &paymentModels.SubscriptionPlan{}
	require.NoError(t, sharedTestDB.First(plan, "id = ?", planID).Error)

	composedInput := dto.CreateComposedSessionInput{
		Distribution: "ubuntu-24.04",
		Size:         "L",
		Terms:        "accepted",
	}

	svc := services.NewTerminalTrainerService(sharedTestDB)

	// (a) The start must SUCCEED despite the id collision — the old UPSERT
	//     restored the existing row; the desired behavior is the same. The
	//     current finalize blind-UPDATE violates the unique index here.
	resp, err := svc.StartComposedSession(userID, composedInput, plan)
	require.NoError(t, err,
		"a finalize whose tt-backend id matches an existing soft-deleted row must restore that row, not fail on the unique index")
	require.NotNil(t, resp)
	assert.Equal(t, reusedSessionID, resp.SessionID)

	// (b) Exactly ONE non-deleted Terminal row owns that session_id, running,
	//     with the correct size snapshot — no duplicate, and the restored row
	//     was un-deleted rather than left soft-deleted beside a new one.
	var liveForSession int64
	require.NoError(t, sharedTestDB.Unscoped().Model(&terminalModels.Terminal{}).
		Where("session_id = ? AND deleted_at IS NULL", reusedSessionID).
		Count(&liveForSession).Error)
	assert.EqualValues(t, 1, liveForSession,
		"exactly one non-deleted row must own the reused session_id after finalize")

	var live terminalModels.Terminal
	require.NoError(t, sharedTestDB.
		Where("session_id = ? AND deleted_at IS NULL", reusedSessionID).
		First(&live).Error)
	assert.Equal(t, terminalModels.StateRunning, live.State,
		"the restored session must be running after a successful start")
	assert.Equal(t, lSizeCPU, live.SizeCPU)
	assert.Equal(t, lSizeMemMB, live.SizeMemoryMB)

	// (c) No leftover "reserving:" placeholder lingers for the user — the
	//     reservation must have been finalized (or folded into the restore),
	//     not abandoned to leak budget until its TTL lapses.
	var placeholders int64
	require.NoError(t, sharedTestDB.Unscoped().Model(&terminalModels.Terminal{}).
		Where("user_id = ? AND session_id LIKE ? AND deleted_at IS NULL",
			userID, terminalModels.TerminalReservationSessionIDPrefix+"%").
		Count(&placeholders).Error)
	assert.EqualValues(t, 0, placeholders,
		"no leftover reservation placeholder may remain after the start completes")

	// (d) The budget reflects exactly ONE occupying L — not two (the restored
	//     row), not zero (a leaked placeholder past its TTL would still count
	//     until expiry, but a duplicate would double-charge).
	cpu, mem := productionBudgetUsage(t, userID)
	assert.Equal(t, lSizeCPU, cpu,
		"a successful start with an id collision must charge exactly one L of CPU budget")
	assert.Equal(t, lSizeMemMB, mem,
		"a successful start with an id collision must charge exactly one L of RAM budget")
}
