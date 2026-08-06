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

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	entityManagementModels "soli/formations/src/entityManagement/models"
	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/scenarios/models"
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

func TestLaunchScenario_403sStructuredWhenBudgetExhausted(t *testing.T) {
	db := freshTestDB(t)
	userID := "launch-budget-user"

	// Plan whose RAM budget (256 MiB) cannot fit the scenario's resolved M
	// size (1 GiB) — reserveBudget must reject before any tt-backend call.
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
		Name:         "launch-budget-test",
		Title:        "Launch Budget Test",
		InstanceType: "M",
		OsType:       "deb",
		IsPublic:     true,
		CreatedByID:  userID,
	}
	require.NoError(t, db.Create(scenario).Error)

	// Pre-seed the tt user key so the launch reaches the budget check
	// instead of failing on key auto-provisioning (the fake tt-backend
	// serves no admin endpoints).
	require.NoError(t, db.Create(&terminalModels.UserTerminalKey{
		UserID:   userID,
		APIKey:   "test-user-key",
		KeyName:  "test",
		IsActive: true,
	}).Error)

	ttSrv := newLaunchTTBackend(t, 8.0, 25.0)
	configureTTServerForLaunch(t, ttSrv.URL)

	router := setupLaunchRouterWithProdMiddleware(t, db, userID)

	body, _ := json.Marshal(map[string]string{"scenario_id": scenario.ID.String()})
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/scenario-sessions/launch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code,
		"budget exhaustion on scenario launch must answer the structured 403 "+
			"the terminal route emits, not a generic 500. Got %d. Body: %s",
		w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"source":"budget"`,
		"403 body must carry source=budget. Got %q", w.Body.String())
	assert.Contains(t, w.Body.String(), "budget_exhausted",
		"403 body must carry the coarse budget_exhausted reason (never the "+
			"per-axis internal reason). Got %q", w.Body.String())
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

	assert.Error(t, svc.AbandonSession(session.ID),
		"a completed session must stay completed — abandon only applies to "+
			"active and provisioning sessions")
}
