package scenarios_test

// Step orders are data-driven: editor-created and imported scenarios are
// 1-based, legacy seeds are 0-based. The frontend used to display
// `step_order + 1`, which reads "Étape 3 / 2" on the last step of a 1-based
// two-step scenario (and shifts the progress dots the whole run). Step
// responses therefore carry the step's POSITION (1-based index among the
// ordered steps) and the full ordered list of step orders, so no client ever
// does arithmetic on the raw order again.

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

// seedPositionScenario creates a scenario whose two steps use the given
// orders, plus an active session sitting on currentOrder.
func seedPositionScenario(t *testing.T, orders []int, currentOrder int) uuid.UUID {
	t.Helper()
	scenario := models.Scenario{
		Name: "position-test", Title: "Position", InstanceType: "xs", CreatedByID: "c1",
	}
	require.NoError(t, sharedTestDB.Create(&scenario).Error)
	for _, o := range orders {
		require.NoError(t, sharedTestDB.Create(&models.ScenarioStep{
			ScenarioID: scenario.ID, Order: o, Title: "Step", StepType: "info",
		}).Error)
	}
	session := models.ScenarioSession{
		BaseModel:  entityManagementModels.BaseModel{ID: uuid.New()},
		ScenarioID: scenario.ID, UserID: "position-user", CurrentStep: currentOrder,
		Status: "active", StartedAt: time.Now(),
	}
	require.NoError(t, sharedTestDB.Create(&session).Error)
	for i, o := range orders {
		status := "locked"
		if o == currentOrder || i == 0 {
			status = "active"
		}
		require.NoError(t, sharedTestDB.Create(&models.ScenarioStepProgress{
			SessionID: session.ID, StepOrder: o, Status: status,
		}).Error)
	}
	return session.ID
}

func TestGetCurrentStep_PositionOnOneBasedOrders(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	freshTestDB(t)
	sessionID := seedPositionScenario(t, []int{1, 2}, 2)

	svc := services.NewScenarioSessionService(sharedTestDB, &mockFlagService{}, &mockVerificationService{})
	resp, err := svc.GetCurrentStep(sessionID)
	require.NoError(t, err)

	assert.Equal(t, 2, resp.StepOrder, "raw order stays untouched")
	assert.Equal(t, 2, resp.Position,
		"the LAST of two 1-based steps is position 2 — step_order+1 would say 3/2")
	assert.Equal(t, 2, resp.TotalSteps)
	assert.Equal(t, []int{1, 2}, resp.StepOrders)
}

func TestGetCurrentStep_PositionOnZeroBasedOrders(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	freshTestDB(t)
	sessionID := seedPositionScenario(t, []int{0, 1}, 0)

	svc := services.NewScenarioSessionService(sharedTestDB, &mockFlagService{}, &mockVerificationService{})
	resp, err := svc.GetCurrentStep(sessionID)
	require.NoError(t, err)

	assert.Equal(t, 0, resp.StepOrder)
	assert.Equal(t, 1, resp.Position, "the first 0-based step is position 1")
	assert.Equal(t, []int{0, 1}, resp.StepOrders)
}

func TestGetStepByOrder_CarriesPosition(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	freshTestDB(t)
	sessionID := seedPositionScenario(t, []int{1, 2}, 2)

	svc := services.NewScenarioSessionService(sharedTestDB, &mockFlagService{}, &mockVerificationService{})
	resp, err := svc.GetStepByOrder(sessionID, 1)
	require.NoError(t, err)

	assert.Equal(t, 1, resp.Position)
	assert.Equal(t, []int{1, 2}, resp.StepOrders)
}
