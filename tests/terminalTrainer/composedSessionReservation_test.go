// Behavioral tests for the reserve-first composed-session path (Phase 3b).
//
// The reserve-first fix (terminalComposer.startComposedSession) inserts a
// StateStarting reservation row INSIDE a locked transaction before it POSTs to
// tt-backend, so the reservation counts toward the budget the moment it commits
// (StateStarting is in TerminalStatesOccupyingSlot). Two invariants make that
// safe without a background sweeper or a budget leak on failure:
//
//   1. Orphan reaping is via expires_at. A reservation carries a short TTL
//      (reservationTTL); OccupiesSlotScope's `expires_at > NOW()` clause stops
//      counting an abandoned reservation once its TTL lapses — so a crash
//      between commit and finalize cannot permanently eat budget. While the
//      reservation is still live (future expiry) it DOES count, which is what
//      blocks concurrent overshoot.
//
//   2. A failed tt-backend provision hard-deletes the reservation, returning
//      its budget to the scope. The failure surfaces as a plain error (NOT a
//      *BudgetRejection — the budget passed; provisioning failed), and the
//      freed budget must be fully reusable by a subsequent start.
//
// Both run on SQLite (the package default) — neither tests concurrency, only
// the single-threaded budget-accounting consequences of the reserve-first
// design.
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

	paymentModels "soli/formations/src/payment/models"
	paymentServices "soli/formations/src/payment/services"
	"soli/formations/src/terminalTrainer/dto"
	terminalModels "soli/formations/src/terminalTrainer/models"
	"soli/formations/src/terminalTrainer/services"
)

// L size footprint in budget terms — mirrors the catalog (L = 4 vCPU / 2 GiB =
// 4000 mCPU / 2048 MiB) used by the HTTP and race tests.
const (
	lSizeCPU   = 4000
	lSizeMemMB = 2048
)

// productionBudgetUsage reads the budget sum through the real QuotaService
// surface (GetBudgetUsage → sumActiveResources → OccupiesSlotScope), i.e. the
// exact predicate the budget gate enforces. Tests assert on this rather than
// re-deriving the scope so a drift in the production scope is caught here.
func productionBudgetUsage(t *testing.T, userID string) (int, int) {
	t.Helper()
	eps := paymentServices.NewEffectivePlanService(sharedTestDB)
	qs := paymentServices.NewQuotaService(sharedTestDB, eps)
	cpu, mem, err := qs.GetBudgetUsage(userID, nil)
	require.NoError(t, err)
	return cpu, mem
}

// TestComposedSession_ReservationOrphan_ExcludedAfterTTL pins invariant (1):
// the budget sum counts a live StateStarting reservation but EXCLUDES one whose
// TTL has lapsed — so an abandoned reservation auto-frees its budget via the
// expires_at clause, while a still-pending one keeps blocking overshoot.
func TestComposedSession_ReservationOrphan_ExcludedAfterTTL(t *testing.T) {
	freshTestDB(t)

	// Case A — an EXPIRED StateStarting reservation must NOT be counted.
	// This is the abandoned-reservation / process-crash scenario: the row was
	// committed but never finalized or deleted, and its short TTL has lapsed.
	orphanUser := "orphan-user-" + uuid.New().String()[:8]
	insertExistingTerminalWithExpiry(t, sharedTestDB, orphanUser, nil,
		string(terminalModels.StateStarting), "ephemeral",
		lSizeCPU, lSizeMemMB, time.Now().Add(-1*time.Minute))

	cpu, mem := productionBudgetUsage(t, orphanUser)
	assert.Equal(t, 0, cpu,
		"an expired (past-TTL) starting reservation must NOT keep eating CPU budget — expires_at reaps it")
	assert.Equal(t, 0, mem,
		"an expired (past-TTL) starting reservation must NOT keep eating RAM budget")

	// Case B — a LIVE StateStarting reservation (future expiry) MUST be counted.
	// This is the in-flight reservation that blocks a concurrent overshoot: it
	// holds its slice of the budget until finalize flips it to running or the
	// failure path deletes it.
	liveUser := "live-resv-user-" + uuid.New().String()[:8]
	insertExistingTerminalWithExpiry(t, sharedTestDB, liveUser, nil,
		string(terminalModels.StateStarting), "ephemeral",
		lSizeCPU, lSizeMemMB, time.Now().Add(reservationTTLForTest))

	cpu, mem = productionBudgetUsage(t, liveUser)
	assert.Equal(t, lSizeCPU, cpu,
		"a live (future-expiry) starting reservation MUST count toward the CPU budget so concurrent starts can't overshoot")
	assert.Equal(t, lSizeMemMB, mem,
		"a live (future-expiry) starting reservation MUST count toward the RAM budget")
}

// reservationTTLForTest is a future window long enough to outlive the test;
// the production TTL constant is unexported, so a fixed comfortable margin is
// used here. Only the sign of (expires_at - NOW) matters to the scope.
const reservationTTLForTest = 5 * time.Minute

// failingComposeTTBackend stands up a fake tt-backend whose POST /sessions
// fails the provision (status != 0) while still serving the read catalog
// (distributions/sizes/features) the composed path needs to validate the size.
// The `fail` pointer lets a later call switch to the success response so the
// same user can prove the budget freed by the failed start is reusable.
func failingComposeTTBackend(t *testing.T, fail *bool) *httptest.Server {
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
			if *fail {
				// status != 0 → provisioning failed. The composed path must
				// hard-delete the reservation it committed before this POST.
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":     "",
					"status": 1,
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":         "ok-sess-" + uuid.New().String(),
				"status":     0,
				"expires_at": time.Now().Add(time.Hour).Unix(),
				"backend":    "incus",
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

// TestComposedSession_TTBackendFailure_DeletesReservation pins invariant (2):
// when tt-backend fails the provision, the reservation row committed before the
// HTTP call is rolled back (hard-deleted), the call returns a plain error (NOT
// a budget rejection), and the freed budget is fully reusable — proven by a
// subsequent start for the SAME user succeeding.
func TestComposedSession_TTBackendFailure_DeletesReservation(t *testing.T) {
	fail := true
	srv := failingComposeTTBackend(t, &fail)
	defer srv.Close()
	t.Setenv("TERMINAL_TRAINER_URL", srv.URL)
	t.Setenv("TERMINAL_TRAINER_ADMIN_KEY", "test-admin-key")
	t.Setenv("TERMINAL_TRAINER_API_VERSION", "1.0")

	freshTestDB(t)
	userID := "tt-fail-user-" + uuid.New().String()[:8]

	// Plan budget fits EXACTLY one L. If the failed start leaked its
	// reservation, the second start below would be budget-rejected.
	planID := seedBudgetPlanForUser(t, userID, lSizeCPU, lSizeMemMB)
	_, err := createTestUserKey(sharedTestDB, userID)
	require.NoError(t, err)

	// StartComposedSession takes the resolved plan directly (the middleware
	// resolves it in production); load the one we just seeded.
	plan := &paymentModels.SubscriptionPlan{}
	require.NoError(t, sharedTestDB.First(plan, "id = ?", planID).Error)

	composedInput := dto.CreateComposedSessionInput{
		Distribution: "ubuntu-24.04",
		Size:         "L",
		Terms:        "accepted",
	}

	svc := services.NewTerminalTrainerService(sharedTestDB)

	// (a) The failed start returns a NON-NIL error that is NOT a budget
	//     rejection — the budget gate passed; tt-backend is what failed.
	resp, err := svc.StartComposedSession(userID, composedInput, plan)
	require.Error(t, err, "a tt-backend status!=0 must surface as an error")
	assert.Nil(t, resp)
	_, isRejection := err.(*services.BudgetRejection)
	assert.False(t, isRejection,
		"a provisioning failure must NOT be reported as a budget rejection — the budget was available; got %T", err)

	// (b) ZERO Terminal rows for the user count under OccupiesSlotScope — the
	//     reservation committed before the POST was hard-deleted on failure.
	var occupied int64
	require.NoError(t, sharedTestDB.Model(&terminalModels.Terminal{}).
		Scopes(terminalModels.OccupiesSlotScope).
		Where("terminals.user_id = ?", userID).
		Count(&occupied).Error)
	assert.EqualValues(t, 0, occupied,
		"the reservation must be rolled back on provisioning failure — no row may keep occupying a slot")

	cpu, mem := productionBudgetUsage(t, userID)
	assert.Equal(t, 0, cpu, "failed start must leave zero CPU budget consumed")
	assert.Equal(t, 0, mem, "failed start must leave zero RAM budget consumed")

	// (c) A subsequent SUCCESSFUL start for the same user must succeed — proves
	//     the failed start released its full budget, not a partial slice.
	fail = false
	resp2, err2 := svc.StartComposedSession(userID, composedInput, plan)
	require.NoError(t, err2,
		"after a failed start fully releases its reservation, the same one-L budget must permit a fresh L start")
	require.NotNil(t, resp2)

	// And exactly that one running session now occupies the budget.
	var occupiedAfter int64
	require.NoError(t, sharedTestDB.Model(&terminalModels.Terminal{}).
		Scopes(terminalModels.OccupiesSlotScope).
		Where("terminals.user_id = ?", userID).
		Count(&occupiedAfter).Error)
	assert.EqualValues(t, 1, occupiedAfter,
		"the successful retry must persist exactly one occupying Terminal row")
}
