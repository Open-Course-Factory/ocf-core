package scenarios_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	configModels "soli/formations/src/configuration/models"
	groupModels "soli/formations/src/groups/models"
	groupServices "soli/formations/src/groups/services"
	paymentModels "soli/formations/src/payment/models"
	scenarioHooks "soli/formations/src/scenarios/hooks"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
	terminalServices "soli/formations/src/terminalTrainer/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setDesktopSwitches(t *testing.T, db *gorm.DB, localMachines, desktop bool) {
	t.Helper()
	require.NoError(t, db.Create(&configModels.Feature{Key: terminalServices.LocalMachinesFeatureKey, Name: "x", Enabled: localMachines}).Error)
	require.NoError(t, db.Create(&configModels.Feature{Key: terminalServices.LocalMachineDesktopFeatureKey, Name: "y", Enabled: desktop}).Error)
}

// A public desktop scenario and a public terminal one, both local.
func seedDesktopAndTerminalScenarios(t *testing.T, db *gorm.DB) (desktop, terminal models.Scenario) {
	t.Helper()
	desktop = models.Scenario{Name: "desk", Title: "Desk", InstanceType: "debian", IsPublic: true,
		CreatedByID: "desk-author", Machine: models.ScenarioMachineLocal, MachineDisplay: models.ScenarioMachineDisplayDesktop}
	terminal = models.Scenario{Name: "term", Title: "Term", InstanceType: "debian", IsPublic: true,
		CreatedByID: "desk-author", Machine: models.ScenarioMachineLocal, MachineDisplay: models.ScenarioMachineDisplayTerminal}
	require.NoError(t, db.Create(&desktop).Error)
	require.NoError(t, db.Create(&terminal).Error)
	return desktop, terminal
}

func availableScenarioNames(t *testing.T, db *gorm.DB, userID string, roles []string) []string {
	t.Helper()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/scenario-sessions/available", nil)
	setupAvailableRouter(db, userID, roles).ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var cards []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &cards))
	names := make([]string, 0, len(cards))
	for _, c := range cards {
		names = append(names, c["name"].(string))
	}
	return names
}

func TestLocalMachineDesktopEnabled_NeedsBothSwitches(t *testing.T) {
	for _, c := range []struct {
		local, desktop, want bool
	}{{false, false, false}, {true, false, false}, {false, true, false}, {true, true, true}} {
		db := freshTestDB(t)
		setDesktopSwitches(t, db, c.local, c.desktop)
		assert.Equal(t, c.want, terminalServices.LocalMachineDesktopEnabled(db), "local=%v desktop=%v", c.local, c.desktop)
	}
	assert.False(t, terminalServices.LocalMachineDesktopEnabled(freshTestDB(t)), "no rows: off")
}

func TestGetAvailableScenarios_DesktopOff_HidesDesktopScenarios(t *testing.T) {
	for name, local := range map[string]bool{"local machines on": true, "local machines off": false} {
		t.Run(name, func(t *testing.T) {
			db := freshTestDB(t)
			setDesktopSwitches(t, db, local, false)
			seedDesktopAndTerminalScenarios(t, db)

			assert.Equal(t, []string{"term"}, availableScenarioNames(t, db, "desk-learner", []string{"member"}))
			assert.Equal(t, []string{"term"}, availableScenarioNames(t, db, "desk-admin", []string{"administrator"}),
				"nobody can launch it, so no launcher offers it")
		})
	}
}

func TestGetAvailableScenarios_LocalOffDesktopOn_StillHidden(t *testing.T) {
	db := freshTestDB(t)
	setDesktopSwitches(t, db, false, true)
	seedDesktopAndTerminalScenarios(t, db)

	assert.Equal(t, []string{"term"}, availableScenarioNames(t, db, "desk-learner", []string{"member"}))
}

func TestGetAvailableScenarios_BothOn_ShowsDesktopScenarios(t *testing.T) {
	db := freshTestDB(t)
	setDesktopSwitches(t, db, true, true)
	seedDesktopAndTerminalScenarios(t, db)

	assert.ElementsMatch(t, []string{"desk", "term"}, availableScenarioNames(t, db, "desk-learner", []string{"member"}))
}

// The editor's list keeps the scenario, so the desktop switch never loses content.
func TestListableScenarioIDs_DesktopOff_AuthorStillSeesIt(t *testing.T) {
	db := freshTestDB(t)
	setDesktopSwitches(t, db, true, false)
	desktop := models.Scenario{Name: "desk-private", Title: "Desk", InstanceType: "debian",
		CreatedByID: "desk-author", Machine: models.ScenarioMachineLocal, MachineDisplay: models.ScenarioMachineDisplayDesktop}
	require.NoError(t, db.Create(&desktop).Error)

	ids, err := scenarioHooks.ListableScenarioIDs(db, groupServices.NewGroupService(db), "desk-author")
	require.NoError(t, err)
	assert.Contains(t, ids, desktop.ID.String())
}

func TestRunMachine_DesktopOff_RefusesDesktopScenario(t *testing.T) {
	plan := &paymentModels.SubscriptionPlan{LocalMachines: paymentModels.LocalMachinesAllowed}
	for name, local := range map[string]bool{"local machines on": true, "local machines off": false} {
		t.Run(name, func(t *testing.T) {
			db := freshTestDB(t)
			setDesktopSwitches(t, db, local, false)
			desktop, terminal := seedDesktopAndTerminalScenarios(t, db)

			_, err := services.ReadMachineSwitches(db).RunMachine(db, &desktop, services.RunRequest{Plan: plan})
			require.ErrorIs(t, err, services.ErrScenarioNeedsDesktop)

			_, err = services.ReadMachineSwitches(db).RunMachine(db, &terminal, services.RunRequest{Plan: plan})
			require.NoError(t, err, "a terminal scenario is unaffected")
		})
	}
}

func TestRunMachine_BothOn_DesktopScenarioRunsLocally(t *testing.T) {
	db := freshTestDB(t)
	setDesktopSwitches(t, db, true, true)
	desktop, _ := seedDesktopAndTerminalScenarios(t, db)

	got, err := services.ReadMachineSwitches(db).RunMachine(db, &desktop, services.RunRequest{Plan: &paymentModels.SubscriptionPlan{LocalMachines: paymentModels.LocalMachinesAllowed}})
	require.NoError(t, err)
	assert.Equal(t, models.ScenarioMachineLocal, got.Machine)
}

func TestLaunchScenario_DesktopOff_RefusedWithReason(t *testing.T) {
	db, userID, scenario, rec := localLaunchFixture(t, true, paymentModels.LocalMachinesAllowed, models.ScenarioMachineLocal)
	require.NoError(t, db.Model(scenario).Update("machine_display", models.ScenarioMachineDisplayDesktop).Error)

	w := launchScenarioForTest(t, setupPersistenceRouter(t, db, userID), scenario.ID)
	require.Equal(t, http.StatusUnprocessableEntity, w.Code, "body=%s", w.Body.String())
	assert.Contains(t, w.Body.String(), "desktop")
	assert.Equal(t, 0, rec.calls)

	var runs int64
	require.NoError(t, db.Model(&models.ScenarioSession{}).Where("user_id = ?", userID).Count(&runs).Error)
	assert.Zero(t, runs)
}

// Bulk start skips RunMachine for Incus scenarios; a desktop one still reaches
// the refusal, whatever its machine says.
func TestBulkStartScenario_DesktopOff_RefusesEveryLearner(t *testing.T) {
	db := freshTestDB(t)
	setDesktopSwitches(t, db, true, false)
	scenario := models.Scenario{Name: "bulk-desk", Title: "Bulk Desk", InstanceType: "s", CreatedByID: "c1",
		MachineDisplay: models.ScenarioMachineDisplayDesktop}
	require.NoError(t, db.Create(&scenario).Error)
	require.NoError(t, db.Create(&models.ScenarioStep{ScenarioID: scenario.ID, Order: 0, Title: "Step", StepType: "terminal"}).Error)

	groupID := createClassGroup(t, db, "bulk-desk-class", "trainer", nil).ID
	ttMock := newMockTTService()
	const learner = "bulk-desk-learner"
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: groupID, UserID: learner, Role: "member", JoinedAt: time.Now(), IsActive: true,
	}).Error)
	seedPersistencePlan(t, db, learner, false)
	ttMock.addKey(learner)

	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, &mockVerificationService{})
	result, err := services.NewTeacherDashboardService(db, ttMock, sessionSvc).BulkStartScenario(groupID, scenario.ID,
		services.ScenarioProvisioning{Distribution: "debian", Size: "s"}, 0, "trainer")
	require.NoError(t, err)

	assert.Zero(t, result.Created)
	var runs int64
	require.NoError(t, db.Model(&models.ScenarioSession{}).Where("user_id = ?", learner).Count(&runs).Error)
	assert.Zero(t, runs)
}
