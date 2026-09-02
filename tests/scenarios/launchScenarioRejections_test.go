// tests/scenarios/launchScenarioRejections_test.go
//
// Pins the launch flow's structured rejections — the paths that used to
// degrade to a generic 500 (scenarios-e2e-test-plan.md §7.2) — plus the
// abandon-during-provisioning contract (§7.3):
//
//  1. DUNNING — a past_due subscription beyond its grace window must get the
//     same 402 subscription_past_due the terminal creation routes emit. The
//     gate lived only on the terminal controller; the scenario launch path
//     called the service directly and skipped it.
//
//  2. BUDGET — when the CPU/RAM budget cannot fit the resolved scenario
//     size, launch must answer the structured 403 (source=budget,
//     reason=budget_exhausted) the terminal route emits, so the launcher can
//     render honest copy instead of a raw 500.
//
//  3. ABANDON — cancelling during provisioning must work. AbandonSession
//     only matched status='active', so the provisioning overlay's cancel
//     button 500'd; the setup goroutine already guards its own writes with
//     WHERE status='provisioning', so an abandoned session stays abandoned.
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
	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/scenarios/models"
	scenarioController "soli/formations/src/scenarios/routes"
	"soli/formations/src/scenarios/services"
	terminalModels "soli/formations/src/terminalTrainer/models"
)

// seedPastDuePlan mirrors seedActivePlan but with a subscription that entered
// past_due long before any conceivable grace window.
func seedPastDuePlan(t *testing.T, db *gorm.DB, userID string) {
	t.Helper()
	plan := &paymentModels.SubscriptionPlan{
		BaseModel:                 entityManagementModels.BaseModel{ID: uuid.New()},
		Name:                      "Formateur",
		Priority:                  10,
		MaxSessionDurationMinutes: 60,
		MaxMemoryMB:               4096,
		IsActive:                  true,
		BillingInterval:           "month",
		Currency:                  "eur",
	}
	require.NoError(t, db.Create(plan).Error)
	pastDueSince := time.Now().AddDate(-1, 0, 0)
	require.NoError(t, db.Create(&paymentModels.UserSubscription{
		UserID:             userID,
		SubscriptionPlanID: plan.ID,
		Status:             "past_due",
		PastDueSince:       &pastDueSince,
		SubscriptionType:   "personal",
		CurrentPeriodStart: time.Now().AddDate(-1, 0, 0),
		CurrentPeriodEnd:   time.Now().AddDate(1, 0, 0),
	}).Error)
}

func TestLaunchScenario_402sWhenSubscriptionPastDueBeyondGrace(t *testing.T) {
	db := freshTestDB(t)
	userID := "launch-dunning-user"
	seedPastDuePlan(t, db, userID)

	scenario := &models.Scenario{
		Name:         "launch-dunning-test",
		Title:        "Launch Dunning Test",
		InstanceType: "M",
		OsType:       "deb",
		IsPublic:     true,
		CreatedByID:  userID,
	}
	require.NoError(t, db.Create(scenario).Error)

	// Healthy host — the request must be rejected by the dunning gate, not
	// capacity.
	ttSrv := newLaunchTTBackend(t, 8.0, 25.0)
	configureTTServerForLaunch(t, ttSrv.URL)

	router := setupLaunchRouterWithProdMiddleware(t, db, userID)

	body, _ := json.Marshal(map[string]string{"scenario_id": scenario.ID.String()})
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/scenario-sessions/launch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusPaymentRequired, w.Code,
		"a past_due sub beyond grace must be blocked by the dunning gate on "+
			"scenario launch, exactly like terminal creation. Got %d. Body: %s",
		w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "subscription_past_due",
		"402 body must carry the stable subscription_past_due code. Got %q",
		w.Body.String())
}

// seedBudgetExhaustedUser prepares a user whose plan's RAM budget (256 MiB)
// cannot fit an M scenario (1 GiB) — reserveBudget must reject before any
// tt-backend call. The tt user key is pre-seeded so the flow reaches the
// budget check instead of failing on key auto-provisioning (the fake
// tt-backend serves no admin endpoints). Returns the seeded M scenario.
func seedBudgetExhaustedUser(t *testing.T, db *gorm.DB, userID, scenarioName string) *models.Scenario {
	t.Helper()
	plan := &paymentModels.SubscriptionPlan{
		BaseModel:                 entityManagementModels.BaseModel{ID: uuid.New()},
		Name:                      "Tiny",
		Priority:                  10,
		MaxSessionDurationMinutes: 60,
		MaxMemoryMB:               256,
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

	scenario := &models.Scenario{
		Name:         scenarioName,
		Title:        scenarioName,
		InstanceType: "M",
		OsType:       "deb",
		IsPublic:     true,
		CreatedByID:  userID,
	}
	require.NoError(t, db.Create(scenario).Error)

	seedUserTTKey(t, db, userID)
	return scenario
}

// seedUserTTKey pre-seeds the tt user key so flows reach their gate under
// test instead of failing on key auto-provisioning (the fake tt-backend
// serves no admin endpoints).
func seedUserTTKey(t *testing.T, db *gorm.DB, userID string) {
	t.Helper()
	require.NoError(t, db.Create(&terminalModels.UserTerminalKey{
		UserID:   userID,
		APIKey:   "test-user-key",
		KeyName:  "test",
		IsActive: true,
	}).Error)
}

// assertStructuredBudget403 pins the shared response contract of
// httperrors.WriteBudgetRejection for any session-creating route.
func assertStructuredBudget403(t *testing.T, code int, body string) {
	t.Helper()
	require.Equal(t, http.StatusForbidden, code,
		"budget exhaustion must answer the structured 403 the terminal route "+
			"emits, not a generic 500. Got %d. Body: %s", code, body)
	assert.Contains(t, body, `"source":"budget"`,
		"403 body must carry source=budget. Got %q", body)
	assert.Contains(t, body, "budget_exhausted",
		"403 body must carry the coarse budget_exhausted reason (never the "+
			"per-axis internal reason). Got %q", body)
}

// setupPreviewRouterWithAdminStub wires POST /scenarios/:id/preview as
// production registers it — bare AuthManagement, no plan-chain middleware
// (PreviewScenario resolves its plan manually). The admin role stub passes
// the preview authorization options.
func setupPreviewRouterWithAdminStub(t *testing.T, db *gorm.DB, userID string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Set("userRoles", []string{"admin"})
		c.Next()
	})
	ctrl := scenarioController.NewScenarioLaunchController(db)
	router.POST("/api/v1/scenarios/:id/preview", ctrl.PreviewScenario)
	return router
}

func TestLaunchScenario_403sStructuredWhenBudgetExhausted(t *testing.T) {
	db := freshTestDB(t)
	userID := "launch-budget-user"
	scenario := seedBudgetExhaustedUser(t, db, userID, "launch-budget-test")

	ttSrv := newLaunchTTBackend(t, 8.0, 25.0)
	configureTTServerForLaunch(t, ttSrv.URL)

	router := setupLaunchRouterWithProdMiddleware(t, db, userID)

	body, _ := json.Marshal(map[string]string{"scenario_id": scenario.ID.String()})
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/scenario-sessions/launch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertStructuredBudget403(t, w.Code, w.Body.String())
}

func TestPreviewScenario_402sWhenSubscriptionPastDueBeyondGrace(t *testing.T) {
	db := freshTestDB(t)
	userID := "preview-dunning-user"
	seedPastDuePlan(t, db, userID)
	// Preview provisions the tt user key before resolving the plan, so the
	// key must exist for the request to reach the dunning gate.
	seedUserTTKey(t, db, userID)

	scenario := &models.Scenario{
		Name:         "preview-dunning-test",
		Title:        "Preview Dunning Test",
		InstanceType: "M",
		OsType:       "deb",
		IsPublic:     true,
		CreatedByID:  userID,
	}
	require.NoError(t, db.Create(scenario).Error)

	ttSrv := newLaunchTTBackend(t, 8.0, 25.0)
	configureTTServerForLaunch(t, ttSrv.URL)

	router := setupPreviewRouterWithAdminStub(t, db, userID)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/scenarios/"+scenario.ID.String()+"/preview", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusPaymentRequired, w.Code,
		"preview provisions a real terminal — a past_due sub beyond grace "+
			"must be gated exactly like launch. Got %d. Body: %s",
		w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "subscription_past_due")
}

func TestPreviewScenario_403sStructuredWhenBudgetExhausted(t *testing.T) {
	db := freshTestDB(t)
	userID := "preview-budget-user"
	scenario := seedBudgetExhaustedUser(t, db, userID, "preview-budget-test")

	ttSrv := newLaunchTTBackend(t, 8.0, 25.0)
	configureTTServerForLaunch(t, ttSrv.URL)

	router := setupPreviewRouterWithAdminStub(t, db, userID)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/scenarios/"+scenario.ID.String()+"/preview", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertStructuredBudget403(t, w.Code, w.Body.String())
}

func TestAbandonSession_AllowedDuringProvisioning(t *testing.T) {
	db := freshTestDB(t)
	svc := services.NewScenarioSessionService(db, &mockFlagService{}, &mockVerificationService{})

	session := &models.ScenarioSession{
		BaseModel:  entityManagementModels.BaseModel{ID: uuid.New()},
		ScenarioID: uuid.New(),
		UserID:     "abandon-provisioning-user",
		Status:     "provisioning",
	}
	require.NoError(t, db.Create(session).Error)

	require.NoError(t, svc.AbandonSession(session.ID),
		"cancelling during provisioning is the provisioning overlay's cancel "+
			"button — it must abandon, not error")

	var reloaded models.ScenarioSession
	require.NoError(t, db.First(&reloaded, "id = ?", session.ID).Error)
	assert.Equal(t, "abandoned", reloaded.Status)
}

// TestAbandonSession_AllowedAfterSetupFailed covers the state a user most wants
// rid of: a run whose setup script died.
//
// Refusing it made a failed session unreachable — the abandon endpoint answered
// "session not found or not abandonable" and deleting the terminal failed too,
// tt-backend having already dropped its side. After a class launch went wrong,
// clearing five of them meant editing the table by hand.
func TestAbandonSession_AllowedAfterSetupFailed(t *testing.T) {
	db := freshTestDB(t)
	svc := services.NewScenarioSessionService(db, &mockFlagService{}, &mockVerificationService{})

	session := &models.ScenarioSession{
		BaseModel:  entityManagementModels.BaseModel{ID: uuid.New()},
		ScenarioID: uuid.New(),
		UserID:     "abandon-setup-failed-user",
		Status:     "setup_failed",
	}
	require.NoError(t, db.Create(session).Error)

	require.NoError(t, svc.AbandonSession(session.ID),
		"a run whose setup failed must be abandonable — it is the one the user "+
			"most wants to be rid of")

	var reloaded models.ScenarioSession
	require.NoError(t, db.First(&reloaded, "id = ?", session.ID).Error)
	assert.Equal(t, "abandoned", reloaded.Status)
}

func TestAbandonSession_StillRefusedForFinishedSessions(t *testing.T) {
	db := freshTestDB(t)
	svc := services.NewScenarioSessionService(db, &mockFlagService{}, &mockVerificationService{})

	session := &models.ScenarioSession{
		BaseModel:  entityManagementModels.BaseModel{ID: uuid.New()},
		ScenarioID: uuid.New(),
		UserID:     "abandon-completed-user",
		Status:     "completed",
	}
	require.NoError(t, db.Create(session).Error)

	err := svc.AbandonSession(session.ID)
	assert.ErrorIs(t, err, services.ErrSessionNotAbandonable,
		"a completed session must stay completed, and the refusal must be the "+
			"sentinel the controller answers 409 with — as a 500 it made an abandon "+
			"that had already succeeded look like one to retry")
}
