// Decisive concurrency test for the composed-session budget gate.
//
// The composed-session flow (POST /terminals/start-composed-session) enforces
// the CPU/RAM budget with an UNLOCKED read, then writes the accounting row
// only AFTER an external tt-backend HTTP call provisions the container
// (terminalComposer.go: enforceBudget → POST /sessions → CreateTerminalSession).
// N concurrent starts therefore all read the same pre-request sum, all pass,
// all provision, and all insert — overshooting the budget.
//
// This is the RED test for that defect. It drives the REAL production path
// (controller → effectivePlanMiddleware → TerminalTrainerService →
// terminalComposer → QuotaService → DB) under TRUE parallelism against a
// PostgreSQL backend, because SQLite serialises writers and cannot reproduce
// the race. The existing terminalTrainer suite is SQLite-only, so until this
// harness landed the race protection had never actually been exercised.
//
// All assertions are user-observable:
//   - persisted Terminal rows counted under the production OccupiesSlotScope,
//   - the number of HTTP requests that returned 200 vs the 403 budget code,
//   - the number of POSTs the tt-backend stub actually received.
//
// Against the CURRENT racy code these FAIL (overshoot). Phase 3's reserve-first
// fix turns them green.
package terminalTrainer_tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	entityManagementModels "soli/formations/src/entityManagement/models"
	orgModels "soli/formations/src/organizations/models"
	paymentModels "soli/formations/src/payment/models"
	terminalModels "soli/formations/src/terminalTrainer/models"
	terminalServices "soli/formations/src/terminalTrainer/services"
)

// raceStub stands up a fake tt-backend that mirrors the read paths
// startComposedTTBackendStub serves (distributions/sizes/features) and, on the
// write path (POST /sessions), (a) counts how many provisioning calls actually
// reached it via an atomic counter and (b) sleeps briefly to WIDEN the
// reserve→provision window so concurrent unlocked budget reads overlap. A
// unique session_id per call is required because Terminal.SessionID has a
// unique index — two losers reusing one id would fail on the index rather than
// on the budget gate, masking the defect under test.
func raceStub(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var posts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			posts.Add(1)
			// Widen the window between the unlocked budget read and the row
			// insert so the racing goroutines overlap deterministically.
			time.Sleep(40 * time.Millisecond)
			// tt-backend names the session id field "id" (see
			// dto.TerminalTrainerSessionResponse). A UNIQUE id per call is
			// mandatory: Terminal.SessionID has a unique index AND
			// CreateTerminalSession reinits an existing row when it finds a
			// matching session_id — so reusing an id (or an empty string)
			// would collapse the racing inserts into a single reinit and HIDE
			// the overshoot from the row-count assertion.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":         "race-sess-" + uuid.New().String(),
				"status":     0,
				"expires_at": time.Now().Add(time.Hour).Unix(),
				"backend":    "incus",
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	return srv, &posts
}

// composedRaceResult tallies the user-observable outcomes of one concurrent
// burst of /start-composed-session calls.
type composedRaceResult struct {
	successes    atomic.Int32 // HTTP 200
	budget403    atomic.Int32 // HTTP 403 with reason=budget_exhausted
	otherFailure atomic.Int32 // anything else (decode error, 500, etc.)
}

// fireConcurrentComposedStarts releases `n` goroutines simultaneously via a
// barrier, each POSTing /start-composed-session with the given JSON body
// against the supplied production router. Results are tallied by HTTP outcome.
func fireConcurrentComposedStarts(t *testing.T, router http.Handler, body []byte, n int) *composedRaceResult {
	t.Helper()
	res := &composedRaceResult{}
	var wg sync.WaitGroup
	barrier := make(chan struct{})

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-barrier
			req := httptest.NewRequest(http.MethodPost,
				"/api/v1/terminals/start-composed-session", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			switch w.Code {
			case http.StatusOK:
				res.successes.Add(1)
			case http.StatusForbidden:
				var payload map[string]any
				if err := json.Unmarshal(w.Body.Bytes(), &payload); err == nil &&
					payload["reason"] == "budget_exhausted" {
					res.budget403.Add(1)
				} else {
					res.otherFailure.Add(1)
				}
			default:
				res.otherFailure.Add(1)
			}
		}()
	}

	close(barrier)
	wg.Wait()
	return res
}

// TestComposedSession_ConcurrentStarts_NeverExceedsBudget — the decisive RED
// test. A plan whose budget fits EXACTLY ONE size-L container (MaxCPU = L's
// 4000 mCPU) receives 10 simultaneous start requests for an L. Production must
// allow exactly one and reject the rest BEFORE provisioning.
//
// Against the current racy composed path, multiple unlocked budget reads all
// see zero existing usage, all pass, all POST to tt-backend, and all insert —
// so multiple rows persist (overshoot), multiple POSTs land, and multiple 200s
// return. Every assertion below fails in that direction.
func TestComposedSession_ConcurrentStarts_NeverExceedsBudget(t *testing.T) {
	pg := sharedTestPGDB(t)

	// All package helpers (seedBudgetPlanForUser, setupBudgetHTTPRouter,
	// createTestUserKey) operate on the package global sharedTestDB. Point it
	// at PostgreSQL for the duration of this test so the production wiring runs
	// against a driver that exhibits real concurrency. Restored on exit.
	// Safe: Go tests in a package run sequentially (no t.Parallel here).
	prev := sharedTestDB
	sharedTestDB = pg
	defer func() { sharedTestDB = prev }()

	ttServer, posts := raceStub(t)
	defer ttServer.Close()
	t.Setenv("TERMINAL_TRAINER_URL", ttServer.URL)
	t.Setenv("TERMINAL_TRAINER_ADMIN_KEY", "test-admin-key")
	t.Setenv("TERMINAL_TRAINER_API_VERSION", "1.0")

	userID := "race-user-" + uuid.New().String()[:8]

	// Plan budget fits EXACTLY one L: catalog L = 4000 mCPU / 2048 MiB.
	seedBudgetPlanForUser(t, userID, 4000, 2048)
	_, err := createTestUserKey(pg, userID)
	require.NoError(t, err)

	svc := terminalServices.NewTerminalTrainerService(pg)
	router := setupBudgetHTTPRouter(t, userID, svc)

	body, _ := json.Marshal(map[string]any{
		"distribution": "ubuntu-24.04",
		"size":         "L",
		"terms":        "accepted",
	})

	const n = 10
	res := fireConcurrentComposedStarts(t, router, body, n)

	// Headline: rows surviving under the production budget scope. Budget fits
	// one L, so at most one row may persist.
	var occupied int64
	require.NoError(t, pg.Model(&terminalModels.Terminal{}).
		Scopes(terminalModels.OccupiesSlotScope).
		Where("terminals.user_id = ?", userID).
		Count(&occupied).Error)

	t.Logf("concurrent L starts: successes=%d budget403=%d other=%d | persisted(occupied)=%d | tt-backend POSTs=%d",
		res.successes.Load(), res.budget403.Load(), res.otherFailure.Load(), occupied, posts.Load())

	assert.LessOrEqual(t, occupied, int64(1),
		"budget fits exactly one L → at most one Terminal row may occupy a slot; more means the budget was overshot")
	assert.EqualValues(t, 1, res.successes.Load(),
		"exactly one concurrent start must return 200; the rest must be budget-rejected")
	assert.EqualValues(t, n-1, res.budget403.Load(),
		"the N-1 losers must return 403 budget_exhausted")
	assert.LessOrEqual(t, posts.Load(), int32(1),
		"losers must be rejected BEFORE provisioning → tt-backend must receive at most one POST")
}

// TestComposedSession_ConcurrentStarts_OrgScoped_NeverExceedsBudget — org
// variant. N DIFFERENT members of one organization start concurrently; the
// org-wide budget fits one L. The org budget sums across ALL members
// (QuotaService.sumActiveResourcesForOrg's member-join), so exactly one
// org-wide success is permitted regardless of which member wins.
//
// Each member POSTs with organization_id in the body so InjectOrgContext +
// InjectEffectivePlan resolve the ORG plan (via OrganizationSubscription) and
// the budget sum runs the member-join. Against the racy code every member's
// unlocked read sees zero org usage, so all succeed — overshooting the org
// budget.
func TestComposedSession_ConcurrentStarts_OrgScoped_NeverExceedsBudget(t *testing.T) {
	pg := sharedTestPGDB(t)

	prev := sharedTestDB
	sharedTestDB = pg
	defer func() { sharedTestDB = prev }()

	ttServer, posts := raceStub(t)
	defer ttServer.Close()
	t.Setenv("TERMINAL_TRAINER_URL", ttServer.URL)
	t.Setenv("TERMINAL_TRAINER_ADMIN_KEY", "test-admin-key")
	t.Setenv("TERMINAL_TRAINER_API_VERSION", "1.0")

	// Org plan whose budget fits exactly one L (4000 mCPU / 2048 MiB).
	plan := &paymentModels.SubscriptionPlan{
		BaseModel:   entityManagementModels.BaseModel{ID: uuid.New()},
		Name:        "OrgRaceBudget",
		Priority:    5,
		IsActive:    true,
		IsCatalog:   true,
		MaxCPU:      4000,
		MaxMemoryMB: 2048,
	}
	require.NoError(t, pg.Create(plan).Error)

	org := createTestOrgForHistory(t, pg, "race-org-owner")
	orgSub := &paymentModels.OrganizationSubscription{
		OrganizationID:     org.ID,
		SubscriptionPlanID: plan.ID,
		StripeCustomerID:   "cus_race_" + uuid.New().String()[:8],
		Status:             "active",
		CurrentPeriodStart: time.Now(),
		CurrentPeriodEnd:   time.Now().AddDate(1, 0, 0),
	}
	require.NoError(t, pg.Create(orgSub).Error)

	// N distinct members, each with a terminal key.
	const n = 8
	members := make([]string, n)
	for i := 0; i < n; i++ {
		uid := "race-org-member-" + uuid.New().String()[:8]
		members[i] = uid
		createTestOrgMember(t, pg, org.ID, uid, orgModels.OrgRoleMember)
		_, err := createTestUserKey(pg, uid)
		require.NoError(t, err)
	}

	svc := terminalServices.NewTerminalTrainerService(pg)

	// Same composed request for every member — organization_id in the body is
	// what makes InjectOrgContext/InjectEffectivePlan resolve the org plan and
	// run the member-join budget sum.
	reqBody, _ := json.Marshal(map[string]any{
		"distribution":    "ubuntu-24.04",
		"size":            "L",
		"terms":           "accepted",
		"organization_id": org.ID.String(),
	})

	// Fire one goroutine per member, each through its own production router
	// (the router pins userId), released together by a shared barrier.
	res := &composedRaceResult{}
	var wg sync.WaitGroup
	barrier := make(chan struct{})
	for _, uid := range members {
		router := setupBudgetHTTPRouter(t, uid, svc)
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-barrier
			req := httptest.NewRequest(http.MethodPost,
				"/api/v1/terminals/start-composed-session", bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			switch w.Code {
			case http.StatusOK:
				res.successes.Add(1)
			case http.StatusForbidden:
				var payload map[string]any
				if err := json.Unmarshal(w.Body.Bytes(), &payload); err == nil &&
					payload["reason"] == "budget_exhausted" {
					res.budget403.Add(1)
				} else {
					res.otherFailure.Add(1)
				}
			default:
				res.otherFailure.Add(1)
			}
		}()
	}
	close(barrier)
	wg.Wait()

	// Org-wide occupied rows under the production scope + member-join.
	var occupied int64
	require.NoError(t, pg.Model(&terminalModels.Terminal{}).
		Scopes(terminalModels.OccupiesSlotScope).
		Joins("JOIN organization_members ON organization_members.user_id = terminals.user_id").
		Where("organization_members.organization_id = ? AND organization_members.deleted_at IS NULL", org.ID).
		Count(&occupied).Error)

	t.Logf("org concurrent L starts: successes=%d budget403=%d other=%d | org-wide occupied=%d | tt-backend POSTs=%d",
		res.successes.Load(), res.budget403.Load(), res.otherFailure.Load(), occupied, posts.Load())

	assert.LessOrEqual(t, occupied, int64(1),
		"org budget fits exactly one L → at most one Terminal across ALL members may occupy a slot")
	assert.EqualValues(t, 1, res.successes.Load(),
		"exactly one org-wide start must succeed; concurrent members must not each pass an unlocked read")
	assert.LessOrEqual(t, posts.Load(), int32(1),
		"losing members must be rejected BEFORE provisioning")
}
