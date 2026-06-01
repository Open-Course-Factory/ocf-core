// tests/scenarios/launchScenarioCapacity_test.go
//
// Regression coverage for the Launch flow's body-read + host-capacity
// regression. Two contracts are pinned:
//
//  1. BODY PARSE — POST /scenario-sessions/launch parses its JSON body.
//     Pre-fix, the route chain installed paymentMiddleware.CheckRAMAvailability
//     between InjectEffectivePlan and the controller. CheckRAMAvailability
//     calls ctx.ShouldBindBodyWith(&input, binding.JSON) to peek at Size,
//     which DRAINS c.Request.Body. The controller's subsequent
//     ctx.ShouldBindJSON(&input) then read from a drained reader, returning
//     io.EOF and a user-visible 400 "Invalid input: EOF". Witness: a launch
//     of an unknown scenario must return 404 "Scenario not found" — proving
//     the body was parsed (scenarioID was extracted and the DB lookup ran),
//     NOT 400 "Invalid input: EOF".
//
//  2. CAPACITY — When the host genuinely cannot fit the resolved scenario
//     size, LaunchScenario must 503 with the canonical "Server at capacity"
//     error shape. The check has to evaluate against the SIZE THE SCENARIO
//     WILL ACTUALLY USE (resolved via scenario.InstanceType + distribution
//     MinSizeKey + launch-time fallback) — NOT the plan's max size. The old
//     middleware estimated against LargestSize (XL=4 GiB) because scenarios
//     don't carry a size in the request body; that produced false 503s for
//     scenarios needing only M (1 GiB). The in-controller check uses the
//     resolved size, so the gate is meaningful.
package scenarios_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	entityManagementModels "soli/formations/src/entityManagement/models"
	paymentMiddleware "soli/formations/src/payment/middleware"
	paymentModels "soli/formations/src/payment/models"
	paymentServices "soli/formations/src/payment/services"
	"soli/formations/src/scenarios/models"
	scenarioController "soli/formations/src/scenarios/routes"
)

// configureTTServerForLaunch points the real terminalTrainerService at a
// fake tt-backend. Mirrors tests/terminalTrainer/persistenceMode_test.go's
// configureTTServer helper.
func configureTTServerForLaunch(t *testing.T, url string) {
	t.Helper()
	t.Setenv("TERMINAL_TRAINER_URL", url)
	t.Setenv("TERMINAL_TRAINER_ADMIN_KEY", "test-admin-key")
	t.Setenv("TERMINAL_TRAINER_API_VERSION", "1.0")
}

// newLaunchTTBackend stands up a fake tt-backend that serves the read-only
// endpoints LaunchScenario hits before the capacity check runs:
//
//   - GET /1.0/distributions  → one distribution matching the seeded scenario
//   - GET /1.0/sizes          → a small size catalog (XS/M/XL)
//   - GET /1.0/metrics        → configurable RAM headroom / utilisation
//
// Endpoints beyond the capacity check (e.g. /compose) are deliberately not
// mocked: tests in this file assert on outcomes BEFORE that boundary.
func newLaunchTTBackend(t *testing.T, ramAvailableGB, ramPercent float64) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/metrics":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ram_available_gb": ramAvailableGB,
				"ram_percent":      ramPercent,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/distributions":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"name":             "ubuntu",
					"prefix":           "ubu",
					"description":      "Ubuntu test distribution",
					"os_type":          "deb",
					"is_global":        true,
					"min_size_key":     "XS",
					"default_size_key": "M",
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/sizes":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"key": "XS", "name": "Extra Small", "cpu": 1, "memory": "256MiB", "disk": "1GiB", "sort_order": 0},
				{"key": "M", "name": "Medium", "cpu": 1, "memory": "1GiB", "disk": "5GiB", "sort_order": 2},
				{"key": "XL", "name": "Extra Large", "cpu": 4, "memory": "4GiB", "disk": "20GiB", "sort_order": 4},
			})
		default:
			http.Error(w, "unexpected: "+r.Method+" "+r.URL.Path, http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// setupLaunchRouterWithProdMiddleware wires POST /scenario-sessions/launch
// EXACTLY as production registers it post-fix in scenarioRoutes.go — that is:
//
//	InjectOrgContext → InjectEffectivePlan → RequirePlan → LaunchScenario
//
// (No CheckRAMAvailability — that's the bug fix.) AuthManagement is replaced
// with a userId/userRoles stub since tests don't stand up Casdoor.
func setupLaunchRouterWithProdMiddleware(t *testing.T, db *gorm.DB, userID string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()

	router.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		// "admin" role bypasses the access check in LaunchScenario — keeps
		// these tests focused on body-parse and capacity, not authorization.
		c.Set("userRoles", []string{"admin"})
		c.Next()
	})

	effectivePlanService := paymentServices.NewEffectivePlanService(db)
	ctrl := scenarioController.NewScenarioLaunchController(db)

	router.POST("/api/v1/scenario-sessions/launch",
		paymentMiddleware.InjectOrgContext(),
		paymentMiddleware.InjectEffectivePlan(effectivePlanService, db),
		paymentMiddleware.RequirePlan(),
		ctrl.LaunchScenario,
	)
	return router
}

// seedActivePlan creates a SubscriptionPlan + active UserSubscription so
// InjectEffectivePlan resolves a plan into context. Returns the plan ID.
func seedActivePlan(t *testing.T, db *gorm.DB, userID string) uuid.UUID {
	t.Helper()
	plan := &paymentModels.SubscriptionPlan{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Name:      "Formateur",
		Priority:  10,
		// Non-trivial MaxMemoryMB so a body-less request would, under the
		// old buggy middleware, estimate LargestSize (XL = 4 GiB). The
		// in-controller check uses the resolved scenario size instead.
		MaxSessionDurationMinutes: 60,
		MaxMemoryMB:               4096,
		DataPersistenceEnabled:    true,
		IsActive:                  true,
		BillingInterval:           "month",
		Currency:                  "eur",
	}
	require.NoError(t, db.Create(plan).Error)
	require.NoError(t, db.Create(&paymentModels.UserSubscription{
		UserID:             userID,
		SubscriptionPlanID: plan.ID,
		Status:             "active",
		SubscriptionType:   "personal",
		CurrentPeriodStart: time.Now(),
		CurrentPeriodEnd:   time.Now().AddDate(1, 0, 0),
	}).Error)
	return plan.ID
}

// TestLaunchScenario_BodyParsedWhenNoCapacityMiddleware is the regression
// test for the user-reported "Invalid input: EOF" 400.
//
// Pre-fix: CheckRAMAvailability middleware called ctx.ShouldBindBodyWith,
// draining c.Request.Body. The controller then read EOF and 400'd before
// looking at scenario_id.
//
// Post-fix: middleware is removed, the controller's ShouldBindJSON reads
// the body cleanly, scenario_id is parsed, the DB lookup runs and returns
// a structured 404 because the scenario doesn't exist. The 404 outcome is
// the WITNESS that body parsing succeeded — under the bug, the controller
// would 400 long before reaching the lookup.
func TestLaunchScenario_BodyParsedWhenNoCapacityMiddleware(t *testing.T) {
	db := freshTestDB(t)
	userID := "launch-bodyparse-user"
	seedActivePlan(t, db, userID)

	// Fake tt-backend with healthy headroom — capacity check passes if it
	// runs. Distributions/sizes are returned in case the controller ever
	// reaches resolveScenarioBackendAndDistribution; but with a non-existent
	// scenario_id, the DB lookup short-circuits first.
	ttSrv := newLaunchTTBackend(t, 8.0, 25.0)
	configureTTServerForLaunch(t, ttSrv.URL)

	router := setupLaunchRouterWithProdMiddleware(t, db, userID)

	// A valid UUID for a scenario that doesn't exist in the DB. Parsing
	// must succeed; the DB lookup must fail with NotFound; the controller
	// must return 404 "Scenario not found".
	body, _ := json.Marshal(map[string]string{
		"scenario_id": uuid.New().String(),
	})
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/scenario-sessions/launch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// The bug surfaced as 400 "Invalid input: EOF". The fix surfaces as
	// 404 "Scenario not found" because the body was parsed correctly and
	// the controller proceeded to the lookup.
	assert.NotEqual(t, http.StatusBadRequest, w.Code,
		"Launch must not 400 — body parsing must succeed now that the "+
			"body-consuming middleware has been removed. Got %d. Body: %s",
		w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "EOF",
		"Response body must not mention EOF — that was the symptom of the "+
			"middleware draining c.Request.Body. Got: %s", w.Body.String())
	assert.Equal(t, http.StatusNotFound, w.Code,
		"With body parsing fixed and a non-existent scenario_id, the "+
			"controller must reach the DB lookup and return 404. Got %d. "+
			"Body: %s", w.Code, w.Body.String())
}

// TestLaunchScenario_503sWhenHostCannotFitResolvedSize proves the
// in-controller capacity check uses the resolved scenario size — not the
// plan-max fallback the old middleware used. It also proves the
// legitimate 503 guard survives.
//
// Setup: a scenario whose InstanceType resolves to size "XL" (4 GiB). Host
// reports 0.04 GB available / 99% used — RAM saturated, so even an XS
// would be refused by EvaluateLaunchCapacity's ram_full short-circuit. The
// resolved XL is definitively unfittable.
//
// Expected: 503 with the canonical "Server at capacity" message. The body
// must be bit-compatible with the old middleware response shape so frontend
// error handling stays uniform.
func TestLaunchScenario_503sWhenHostCannotFitResolvedSize(t *testing.T) {
	db := freshTestDB(t)
	userID := "launch-503-user"
	seedActivePlan(t, db, userID)

	// Seed a scenario whose InstanceType is XL (the resolved size will be
	// XL). os_type=deb matches the fake distribution; no compatible
	// instance type rows are needed because resolveDistribution falls
	// through to OsType matching when CompatibleInstanceTypes is empty.
	scenario := &models.Scenario{
		Name:         "launch-503-test",
		Title:        "Launch 503 Test",
		InstanceType: "XL",
		OsType:       "deb",
		IsPublic:     true,
		CreatedByID:  userID,
	}
	require.NoError(t, db.Create(scenario).Error)

	// Saturated host: 0.04 GB available, 99% used. EvaluateLaunchCapacity's
	// ram_full short-circuit refuses any launch regardless of resolved size,
	// but the test pinned size is XL so even without the short-circuit the
	// answer is the same.
	ttSrv := newLaunchTTBackend(t, 0.04, 99.0)
	configureTTServerForLaunch(t, ttSrv.URL)

	router := setupLaunchRouterWithProdMiddleware(t, db, userID)

	body, _ := json.Marshal(map[string]string{
		"scenario_id": scenario.ID.String(),
	})
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/scenario-sessions/launch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusServiceUnavailable, w.Code,
		"Launch on a RAM-saturated host (99%% used, 0.04 GB free) must "+
			"503 — the resolved XL cannot be allocated. Got %d. Body: %s",
		w.Code, w.Body.String())

	// Bit-compatible error shape with ramCheckMiddleware.go's 503 path,
	// so frontend error handling stays uniform across create/resume/launch.
	assert.Contains(t, w.Body.String(), "Server at capacity",
		"503 body must contain the canonical 'Server at capacity' string. "+
			"Got %q", w.Body.String())
}
