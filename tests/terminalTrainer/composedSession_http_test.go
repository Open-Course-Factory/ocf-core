// HTTP integration test for the composed-session budget gate.
//
// Cleanup 5 (post-MR-243 review): the existing budget-mode tests for
// StartComposedSession exercise EnforceBudget in isolation. None hit the
// full HTTP path through the controller → middleware → service → DB.
// This file fills that gap: real DB, real QuotaService, real
// TerminalTrainerService, real controller, real middleware chain.
//
// A fake tt-backend is stood up via httptest.NewServer because
// GetSessionOptions reaches out to it for distributions / sizes /
// features. The fake responds with a small fixture. The POST /sessions
// path is also stubbed so the happy-path test can complete the
// terminal-creation flow end-to-end.
//
// The contract pinned here is the HTTP body shape on rejection:
//
//	403 Forbidden, source="budget", reason="budget_exhausted"
//
// Cleanup 4 collapses the granular CPU/RAM reasons into the coarse
// "budget_exhausted" — both rejection axes must surface the same code
// to the customer. The granular value is preserved on the internal
// BudgetRejection struct so service logs can still distinguish.
package terminalTrainer_tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	entityManagementModels "soli/formations/src/entityManagement/models"
	paymentMiddleware "soli/formations/src/payment/middleware"
	paymentModels "soli/formations/src/payment/models"
	paymentServices "soli/formations/src/payment/services"
	terminalController "soli/formations/src/terminalTrainer/routes"
	terminalServices "soli/formations/src/terminalTrainer/services"
)

// startComposedTTBackendStub stands up a fake tt-backend exposing the
// minimal surface StartComposedSession needs: distributions, sizes,
// features (read paths) plus POST /sessions (write path). All endpoints
// return small, valid fixtures — enough for the controller to route
// to the budget gate or to complete the create flow.
func startComposedTTBackendStub(t *testing.T) *httptest.Server {
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
			// Mirror tt-backend's canonical seed catalog (dbSeedSizes).
			// CPU/Memory must match backfill.sizeCatalog so the budget
			// engine and the catalog endpoint agree on resource cost.
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
			// Happy-path: respond with a valid session payload so the
			// controller can persist a Terminal row.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"session_id":   "stub-sess-" + uuid.New().String(),
				"status":       0,
				"expires_at":   time.Now().Add(time.Hour).Unix(),
				"backend":      "incus",
				"instance_name": "stub-instance",
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

// setupBudgetHTTPRouter wires the production middleware chain for
// /start-composed-session minus the two unavoidable substitutions other
// HTTP tests in this package make:
//
//   - AuthManagement → userId/userRoles stub (no Casdoor in tests)
//   - CheckRAMAvailability omitted (needs live metrics; orthogonal)
//
// Everything else is exactly the route's production wiring.
func setupBudgetHTTPRouter(t *testing.T, userID string, svc terminalServices.TerminalTrainerService) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()

	router.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Set("userRoles", []string{"member"})
		c.Next()
	})

	eps := paymentServices.NewEffectivePlanService(sharedTestDB)
	ctrl := terminalController.NewTerminalControllerWithService(sharedTestDB, svc)

	router.POST("/api/v1/terminals/start-composed-session",
		paymentMiddleware.InjectOrgContext(),
		paymentMiddleware.InjectEffectivePlan(eps, sharedTestDB),
		paymentMiddleware.RequirePlan(),
		paymentMiddleware.CheckLimit(eps, sharedTestDB, "concurrent_terminals"),
		ctrl.StartComposedSession,
	)
	return router
}

// budgetPlan inserts a budget-mode SubscriptionPlan with the given caps
// and a UserSubscription tying it to userID. Returns the plan ID so
// callers can reference it if needed.
func seedBudgetPlanForUser(t *testing.T, userID string, maxCPU, maxMemMB int) uuid.UUID {
	t.Helper()
	plan := &paymentModels.SubscriptionPlan{
		BaseModel:              entityManagementModels.BaseModel{ID: uuid.New()},
		Name:                   "BudgetHTTP",
		Priority:               5,
		IsActive:               true,
		BillingInterval:        "month",
		Currency:               "eur",
		QuotaModel:             "budget",
		MaxCPU:                 maxCPU,
		MaxMemoryMB:            maxMemMB,
		MaxCourses:             10,
		MaxConcurrentTerminals: 100, // high enough to not collide with slot gate
		AllowedMachineSizes:    []string{"all"}, // size gating handled by budget, not size allowlist
	}
	require.NoError(t, sharedTestDB.Create(plan).Error)
	require.NoError(t, sharedTestDB.Create(&paymentModels.UserSubscription{
		UserID:             userID,
		SubscriptionPlanID: plan.ID,
		Status:             "active",
		SubscriptionType:   "personal",
		CurrentPeriodStart: time.Now(),
		CurrentPeriodEnd:   time.Now().AddDate(1, 0, 0),
	}).Error)
	return plan.ID
}

// TestStartComposedSession_HTTP_BudgetMode_RejectsOverBudget — the
// regression-pin for cleanups 4 + 5 combined.
//
// Setup: budget plan with MaxCPU=4 / MaxMemoryMB=2048. A pre-existing
// L terminal (4 CPU / 2048 MiB, persistent) consumes the entire budget.
// The user POSTs /start-composed-session asking for another L.
//
// Expected: 403 Forbidden with body
//
//	{ "source": "budget", "reason": "budget_exhausted", ... }
//
// The "reason" must be the coarse code (not budget_cpu_exceeded) per
// cleanup 4.
func TestStartComposedSession_HTTP_BudgetMode_RejectsOverBudget(t *testing.T) {
	// Feature flag must be on for the budget gate to fire.
	t.Setenv("OCF_FEATURE_BUDGET_QUOTAS", "1")

	ttServer := startComposedTTBackendStub(t)
	defer ttServer.Close()
	t.Setenv("TERMINAL_TRAINER_URL", ttServer.URL)
	t.Setenv("TERMINAL_TRAINER_ADMIN_KEY", "test-admin-key")
	t.Setenv("TERMINAL_TRAINER_API_VERSION", "1.0")

	freshTestDB(t)
	userID := "http-budget-overbudget-user"

	// Plan: 4 CPU / 2048 MiB.
	seedBudgetPlanForUser(t, userID, 4, 2048)

	// User key — startComposedSession looks this up before posting to
	// tt-backend. We won't reach that step in the reject path, but the
	// budget gate runs after GetSessionOptions so we don't need it here.
	// Still seed it so the test mirrors production state.
	_, err := createTestUserKey(sharedTestDB, userID)
	require.NoError(t, err)

	// Pre-state: one running L (4 CPU / 2048 MiB) → budget fully spent.
	insertExistingTerminal(t, sharedTestDB, userID, nil, "running", "ephemeral", 4, 2048)

	// NewTerminalTrainerService wires the real QuotaService internally.
	// No mocking: the budget check runs against the real DB state seeded above.
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

	assert.Equal(t, http.StatusForbidden, w.Code,
		"over-budget request must be rejected with 403 — got %d. Body: %s", w.Code, w.Body.String())

	var payload map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	assert.Equal(t, "budget", payload["source"],
		"403 payload must carry source='budget' so the frontend can pick the right toast")
	assert.Equal(t, "budget_exhausted", payload["reason"],
		"reason must be the coarse code; granular CPU/RAM axes must NOT leak to the customer (cleanup 4)")
}

// TestStartComposedSession_HTTP_BudgetMode_AllowsWithinBudget — happy-path
// counterpart. Same plan, no pre-existing usage; an XS request must
// succeed. Pins that the budget gate doesn't false-reject under capacity.
//
// The success path persists a Terminal row carrying SizeCPU /
// SizeMemoryMB sourced from backfill (cleanup 1 SSOT). We assert both
// the HTTP status AND the persisted footprint so a regression that
// silently zeroes the denormalised columns is caught here.
func TestStartComposedSession_HTTP_BudgetMode_AllowsWithinBudget(t *testing.T) {
	t.Setenv("OCF_FEATURE_BUDGET_QUOTAS", "1")

	ttServer := startComposedTTBackendStub(t)
	defer ttServer.Close()
	t.Setenv("TERMINAL_TRAINER_URL", ttServer.URL)
	t.Setenv("TERMINAL_TRAINER_ADMIN_KEY", "test-admin-key")
	t.Setenv("TERMINAL_TRAINER_API_VERSION", "1.0")

	freshTestDB(t)
	userID := "http-budget-happy-user"

	seedBudgetPlanForUser(t, userID, 8, 4096)
	_, err := createTestUserKey(sharedTestDB, userID)
	require.NoError(t, err)

	// NewTerminalTrainerService wires the real QuotaService internally.
	// No mocking: the budget check runs against the real DB state seeded above.
	svc := terminalServices.NewTerminalTrainerService(sharedTestDB)
	router := setupBudgetHTTPRouter(t, userID, svc)

	body, _ := json.Marshal(map[string]any{
		"distribution": "ubuntu-24.04",
		"size":         "XS",
		"terms":        "accepted",
	})
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/terminals/start-composed-session", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code,
		"within-budget request must succeed — got %d. Body: %s", w.Code, w.Body.String())

	// Verify the persisted Terminal row carries the denormalised footprint
	// sourced from the backfill catalog (cleanup 1's SSOT). A drift here
	// breaks the budget-sum query (which reads from these columns).
	var cnt int64
	require.NoError(t, sharedTestDB.Raw(
		`SELECT COUNT(*) FROM terminals WHERE user_id = ? AND size_cpu = ? AND size_memory_mb = ?`,
		userID, 1, 256,
	).Scan(&cnt).Error)
	assert.EqualValues(t, 1, cnt,
		"persisted Terminal must carry SizeCPU=1, SizeMemoryMB=256 from the backfill catalog (XS)")
}
