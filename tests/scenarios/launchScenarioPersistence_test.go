// tests/scenarios/launchScenarioPersistence_test.go
//
// Pins the persistence_mode resolution in LaunchScenario:
//
//  1. PERSISTENT — when the user's plan has DataPersistenceEnabled=true and
//     the scenario does NOT carry CrashTraps, the launcher MUST request
//     persistent so learners can pause/resume across sessions. Before this
//     change the only branch that set PersistenceMode was the CrashTraps
//     override, so every other scenario inherited the empty-default
//     ("ephemeral") in resolvePersistenceMode — even on paid plans.
//
//  2. EPHEMERAL — when the plan does NOT permit persistence
//     (DataPersistenceEnabled=false), the empty PersistenceMode resolves to
//     "ephemeral" downstream. The wire body posted to tt-backend MUST carry
//     persistence_mode=ephemeral.
//
//  3. CRASH_TRAPS OVERRIDE — when scenario.CrashTraps=true, ephemeral is
//     forced regardless of plan capability. The trap mechanics depend on
//     container destruction; persisting state would defeat the design.
//
// Witness: assertions read persistence_mode from the JSON body the
// controller's StartComposedSession POSTs to the fake tt-backend's
// /1.0/sessions endpoint — i.e. on what production code actually wired to
// the next service, not on a "mock was called" boolean.
package scenarios_test

import (
	"bytes"
	"encoding/json"
	"io"
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
	terminalModels "soli/formations/src/terminalTrainer/models"
)

// persistenceRecorder captures the JSON body of the POST /1.0/sessions
// request so the test can assert on the persistence_mode field
// LaunchScenario forwarded downstream.
type persistenceRecorder struct {
	gotBody map[string]any
	calls   int
}

// newPersistenceTTBackend stands up a fake tt-backend wired for the full
// LaunchScenario flow: distributions/sizes/features for resolution,
// /terms for the terms call, /metrics for capacity, and /sessions to
// record the persistence_mode the controller posted.
func newPersistenceTTBackend(t *testing.T) (*httptest.Server, *persistenceRecorder) {
	t.Helper()
	rec := &persistenceRecorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/metrics":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ram_available_gb": 8.0,
				"ram_percent":      25.0,
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
				{"key": "S", "name": "Small", "cpu": 1, "memory": "512MiB", "disk": "2GiB", "sort_order": 1},
				{"key": "M", "name": "Medium", "cpu": 1, "memory": "1GiB", "disk": "5GiB", "sort_order": 2},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/features":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/terms":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"terms": "accepted",
				"hash":  "deadbeef",
			})
		case r.Method == http.MethodPost && r.URL.Path == "/1.0/sessions":
			rec.calls++
			body, _ := io.ReadAll(r.Body)
			parsed := map[string]any{}
			_ = json.Unmarshal(body, &parsed)
			rec.gotBody = parsed
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"session_id": "fake-sess-" + uuid.New().String(),
				"expires_at": time.Now().Add(time.Hour).Unix(),
				"backend":    "local",
				"status":     0,
			})
		default:
			http.Error(w, "unexpected: "+r.Method+" "+r.URL.Path, http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, rec
}

// configureTTServerForPersistence points the real terminalTrainerService at
// the fake tt-backend stood up by newPersistenceTTBackend.
func configureTTServerForPersistence(t *testing.T, url string) {
	t.Helper()
	t.Setenv("TERMINAL_TRAINER_URL", url)
	t.Setenv("TERMINAL_TRAINER_ADMIN_KEY", "test-admin-key")
	t.Setenv("TERMINAL_TRAINER_API_VERSION", "1.0")
}

// seedPersistencePlan creates a SubscriptionPlan with the given persistence
// flag plus an active UserSubscription so InjectEffectivePlan resolves a
// plan into context.
func seedPersistencePlan(t *testing.T, db *gorm.DB, userID string, dataPersistenceEnabled bool) {
	t.Helper()
	plan := &paymentModels.SubscriptionPlan{
		BaseModel:                 entityManagementModels.BaseModel{ID: uuid.New()},
		Name:                      "PersistencePlan",
		Priority:                  10,
		MaxSessionDurationMinutes: 60,
		MaxCPU:                    8000,
		MaxMemoryMB:               8192,
		DataPersistenceEnabled:    dataPersistenceEnabled,
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
}

// seedPersistenceUserKey pre-creates a UserTerminalKey so LaunchScenario's
// GetUserKey path short-circuits (no CreateUserKey HTTP call to the fake
// tt-backend needed).
func seedPersistenceUserKey(t *testing.T, db *gorm.DB, userID string) {
	t.Helper()
	require.NoError(t, db.Create(&terminalModels.UserTerminalKey{
		UserID:      userID,
		APIKey:      "test-key-" + userID,
		KeyName:     "test-" + userID,
		IsActive:    true,
		MaxSessions: 5,
	}).Error)
}

// seedPersistenceScenario creates a public scenario with InstanceType=M, a
// single step (so StartScenario / scenario session creation doesn't fail
// with "scenario has no steps"), and the given CrashTraps flag. Public so
// the assignment check inside LaunchScenario short-circuits without
// group/org setup.
func seedPersistenceScenario(t *testing.T, db *gorm.DB, userID string, crashTraps bool) *models.Scenario {
	t.Helper()
	scenario := &models.Scenario{
		Name:         "persistence-test-" + uuid.New().String(),
		Title:        "Persistence Test",
		InstanceType: "M",
		OsType:       "deb",
		IsPublic:     true,
		CreatedByID:  userID,
		CrashTraps:   crashTraps,
	}
	require.NoError(t, db.Create(scenario).Error)
	require.NoError(t, db.Create(&models.ScenarioStep{
		ScenarioID: scenario.ID,
		Order:      0,
		Title:      "Step 1",
		StepType:   "terminal",
	}).Error)
	return scenario
}

// setupPersistenceRouter wires POST /scenario-sessions/launch with the
// exact production middleware chain (no auth — Casdoor isn't stood up,
// "admin" role bypasses the access check so the test focuses on
// persistence resolution).
func setupPersistenceRouter(t *testing.T, db *gorm.DB, userID string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", userID)
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

// launchScenarioForTest performs the HTTP POST and returns the response.
func launchScenarioForTest(t *testing.T, router *gin.Engine, scenarioID uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"scenario_id": scenarioID.String()})
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/scenario-sessions/launch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// TestLaunchScenario_PersistentWhenPlanAllowsAndNoCrashTraps pins the
// behaviour added in this MR: when the plan permits persistence and the
// scenario doesn't force ephemeral, LaunchScenario MUST request persistent.
//
// Pre-change witness: composedInput.PersistenceMode was left empty for
// every non-CrashTraps scenario, so resolvePersistenceMode defaulted to
// "ephemeral" — paid users couldn't pause and resume.
// Post-change witness: the JSON body posted to tt-backend's /1.0/sessions
// carries persistence_mode="persistent".
func TestLaunchScenario_PersistentWhenPlanAllowsAndNoCrashTraps(t *testing.T) {
	db := freshTestDB(t)
	userID := "launch-pers-allow-" + uuid.New().String()

	seedPersistencePlan(t, db, userID, true /* DataPersistenceEnabled */)
	seedPersistenceUserKey(t, db, userID)
	scenario := seedPersistenceScenario(t, db, userID, false /* no crash_traps */)

	ttSrv, rec := newPersistenceTTBackend(t)
	configureTTServerForPersistence(t, ttSrv.URL)

	router := setupPersistenceRouter(t, db, userID)
	w := launchScenarioForTest(t, router, scenario.ID)

	require.Equal(t, http.StatusOK, w.Code,
		"launch must succeed on a healthy host with a paid plan; got %d. Body: %s",
		w.Code, w.Body.String())
	require.Equal(t, 1, rec.calls,
		"tt-backend /1.0/sessions must be reached exactly once")
	assert.Equal(t, "persistent", rec.gotBody["persistence_mode"],
		"plan.DataPersistenceEnabled=true + no crash_traps must request "+
			"persistent on the wire so learners can pause/resume; "+
			"body=%v", rec.gotBody)
}

// TestLaunchScenario_EphemeralWhenPlanForbidsPersistence locks the
// safety-net behaviour: a plan WITHOUT DataPersistenceEnabled must not
// upgrade the scenario to persistent. Empty PersistenceMode resolves to
// "ephemeral" downstream — that's the existing default and it must not
// regress.
func TestLaunchScenario_EphemeralWhenPlanForbidsPersistence(t *testing.T) {
	db := freshTestDB(t)
	userID := "launch-pers-forbid-" + uuid.New().String()

	seedPersistencePlan(t, db, userID, false /* DataPersistenceEnabled */)
	seedPersistenceUserKey(t, db, userID)
	scenario := seedPersistenceScenario(t, db, userID, false /* no crash_traps */)

	ttSrv, rec := newPersistenceTTBackend(t)
	configureTTServerForPersistence(t, ttSrv.URL)

	router := setupPersistenceRouter(t, db, userID)
	w := launchScenarioForTest(t, router, scenario.ID)

	require.Equal(t, http.StatusOK, w.Code,
		"launch on a free plan must succeed (just ephemeral); got %d. Body: %s",
		w.Code, w.Body.String())
	require.Equal(t, 1, rec.calls,
		"tt-backend /1.0/sessions must be reached exactly once")
	assert.Equal(t, "ephemeral", rec.gotBody["persistence_mode"],
		"plan.DataPersistenceEnabled=false must resolve to ephemeral on the "+
			"wire — the controller must not opt into persistent without "+
			"plan permission; body=%v", rec.gotBody)
}

// TestLaunchScenario_EphemeralForcedByCrashTrapsRegardlessOfPlan pins
// the override semantics: even on a paid plan, scenarios with
// CrashTraps=true must run ephemeral because the trap mechanics rely on
// container destruction. ScenarioForcesEphemeral wins over the plan's
// persistence capability.
func TestLaunchScenario_EphemeralForcedByCrashTrapsRegardlessOfPlan(t *testing.T) {
	db := freshTestDB(t)
	userID := "launch-pers-crash-" + uuid.New().String()

	// Paid plan that WOULD allow persistent if not for crash_traps.
	seedPersistencePlan(t, db, userID, true /* DataPersistenceEnabled */)
	seedPersistenceUserKey(t, db, userID)
	scenario := seedPersistenceScenario(t, db, userID, true /* crash_traps */)

	ttSrv, rec := newPersistenceTTBackend(t)
	configureTTServerForPersistence(t, ttSrv.URL)

	router := setupPersistenceRouter(t, db, userID)
	w := launchScenarioForTest(t, router, scenario.ID)

	require.Equal(t, http.StatusOK, w.Code,
		"launch on a crash_traps scenario must succeed (forced ephemeral); "+
			"got %d. Body: %s", w.Code, w.Body.String())
	require.Equal(t, 1, rec.calls,
		"tt-backend /1.0/sessions must be reached exactly once")
	assert.Equal(t, "ephemeral", rec.gotBody["persistence_mode"],
		"crash_traps=true must force persistence_mode=ephemeral on the wire "+
			"even when the plan permits persistent; trap mechanics rely on "+
			"container destruction; body=%v", rec.gotBody)
}
