// tests/scenarios/bulkAbandonTerminal_test.go
//
// A run abandoned in bulk must release its terminal the way the learner's own
// abandon does (scenarioProgressController.AbandonSession): a stopped or left
// running machine keeps its share of the learner's CPU/RAM budget. These tests
// drive the bulk paths through their routes, with the real terminal service
// pointed at a fake tt-backend, and check each abandoned run's terminal is
// deleted there and marked deleted locally.
package scenarios_test

import (
	"errors"
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
	groupModels "soli/formations/src/groups/models"
	orgModels "soli/formations/src/organizations/models"
	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
	terminalModels "soli/formations/src/terminalTrainer/models"
)

// seedBulkRun creates a run of the scenario for the learner. A non-empty
// terminalID links the run to it; withTerminalRow also creates that terminal,
// running, so the terminal service can find and delete it.
func seedBulkRun(t *testing.T, db *gorm.DB, scenarioID uuid.UUID, userID, status, terminalID string, withTerminalRow bool) uuid.UUID {
	t.Helper()
	run := &models.ScenarioSession{
		BaseModel:  entityManagementModels.BaseModel{ID: uuid.New()},
		ScenarioID: scenarioID,
		UserID:     userID,
		Status:     status,
		StartedAt:  time.Now(),
	}
	if terminalID != "" {
		run.TerminalSessionID = &terminalID
	}
	require.NoError(t, db.Create(run).Error)
	if withTerminalRow {
		key := terminalModels.UserTerminalKey{UserID: userID, APIKey: "key-" + terminalID, KeyName: "k-" + terminalID, IsActive: true}
		require.NoError(t, db.Create(&key).Error)
		require.NoError(t, db.Create(&terminalModels.Terminal{
			SessionID:         terminalID,
			UserID:            userID,
			State:             terminalModels.StateRunning,
			ExpiresAt:         time.Now().Add(time.Hour),
			InstanceType:      "ubuntu",
			UserTerminalKeyID: key.ID,
		}).Error)
	}
	return run.ID
}

func terminalState(t *testing.T, db *gorm.DB, sessionID string) terminalModels.TerminalState {
	t.Helper()
	var terminal terminalModels.Terminal
	require.NoError(t, db.First(&terminal, "session_id = ?", sessionID).Error)
	return terminal.State
}

// scenarioDeleteFixture seeds, on one org scenario: an open run on a terminal,
// an open run without one, an open run whose terminal row is gone (its
// deletion fails), and a finished run on a terminal.
type scenarioDeleteFixture struct {
	orgID, scenarioID                  uuid.UUID
	openRun, noTerminalRun, missingRun uuid.UUID
	finishedRun                        uuid.UUID
}

func seedScenarioDeleteFixture(t *testing.T, db *gorm.DB, manager string) scenarioDeleteFixture {
	t.Helper()
	orgID := createTestOrg(t, db, "org-owner")
	addOrgMember(t, db, orgID, manager, orgModels.OrgRoleManager)
	scenario := createTestScenarioForOrg(t, db, orgID, "bulk-abandon")
	groupID := createTestGroupInOrg(t, db, orgID, "org-owner")
	createScenarioAssignment(t, db, scenario.ID, &groupID, &orgID, "group")
	return scenarioDeleteFixture{
		orgID:         orgID,
		scenarioID:    scenario.ID,
		openRun:       seedBulkRun(t, db, scenario.ID, "learner-open", "active", "term-open", true),
		noTerminalRun: seedBulkRun(t, db, scenario.ID, "learner-noterm", "active", "", false),
		missingRun:    seedBulkRun(t, db, scenario.ID, "learner-missing", "active", "term-missing", false),
		finishedRun:   seedBulkRun(t, db, scenario.ID, "learner-done", "completed", "term-finished", true),
	}
}

func (f scenarioDeleteFixture) assertTerminalsReleased(t *testing.T, db *gorm.DB, rec *persistenceRecorder) {
	t.Helper()
	// Controls: the abandon itself happened, a failed deletion included.
	assert.Equal(t, "abandoned", sessionStatus(t, db, f.openRun))
	assert.Equal(t, "abandoned", sessionStatus(t, db, f.noTerminalRun))
	assert.Equal(t, "abandoned", sessionStatus(t, db, f.missingRun),
		"a terminal that cannot be deleted must not undo the abandon")
	assert.Equal(t, "completed", sessionStatus(t, db, f.finishedRun))

	rec.mu.Lock()
	deleted := append([]string(nil), rec.deleted...)
	rec.mu.Unlock()
	assert.Equal(t, []string{"term-open"}, deleted,
		"the abandoned run's terminal must be deleted on tt-backend, and only it")
	assert.Equal(t, terminalModels.StateDeleted, terminalState(t, db, "term-open"),
		"the abandoned run's terminal must be marked deleted, so it stops counting against the budget")
	assert.Equal(t, terminalModels.StateRunning, terminalState(t, db, "term-finished"),
		"a finished run's terminal is not the bulk abandon's to delete")
}

func TestDeleteScenario_ViaGenericRoute_DeletesTerminalsOfAbandonedRuns(t *testing.T) {
	srv, rec := newPersistenceTTBackend(t)
	configureTTServerForPersistence(t, srv.URL)
	db := freshTestDB(t)
	f := seedScenarioDeleteFixture(t, db, "org-manager")

	router := setupArchiveRouter(t, db, "org-manager", []string{"member"})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/v1/scenarios/"+f.scenarioID.String(), nil))
	require.Equal(t, http.StatusNoContent, w.Code, "body: %s", w.Body.String())

	f.assertTerminalsReleased(t, db, rec)
}

func TestOrgDeleteScenario_DeletesTerminalsOfAbandonedRuns(t *testing.T) {
	srv, rec := newPersistenceTTBackend(t)
	configureTTServerForPersistence(t, srv.URL)
	db := freshTestDB(t)
	f := seedScenarioDeleteFixture(t, db, "org-manager")

	router := setupOrgTestRouterWithUserAndRoles(t, db, "org-manager", []string{"Member"})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodDelete,
		"/api/v1/organizations/"+f.orgID.String()+"/scenarios/"+f.scenarioID.String(), nil))
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	f.assertTerminalsReleased(t, db, rec)
}

// Removing a class's assignment abandons its members' open runs: their
// terminals go too, a finished run's and a non-member's are left alone.
func TestDeleteScenarioAssignment_ViaGenericRoute_DeletesTerminalsOfAbandonedRuns(t *testing.T) {
	srv, rec := newPersistenceTTBackend(t)
	configureTTServerForPersistence(t, srv.URL)
	db := freshTestDB(t)
	orgID := createTestOrg(t, db, "org-owner")
	addOrgMember(t, db, orgID, "org-manager", orgModels.OrgRoleManager)
	scenario := createTestScenarioForOrg(t, db, orgID, "unassigned")
	groupID := createTestGroupInOrg(t, db, orgID, "org-owner")
	for _, learner := range []string{"learner-open", "learner-noterm", "learner-missing", "learner-done"} {
		addGroupMember(t, db, groupID, learner, groupModels.GroupMemberRoleMember)
	}
	assignment := createScenarioAssignment(t, db, scenario.ID, &groupID, &orgID, "group")
	openRun := seedBulkRun(t, db, scenario.ID, "learner-open", "active", "term-open", true)
	noTerminalRun := seedBulkRun(t, db, scenario.ID, "learner-noterm", "active", "", false)
	missingRun := seedBulkRun(t, db, scenario.ID, "learner-missing", "active", "term-missing", false)
	finishedRun := seedBulkRun(t, db, scenario.ID, "learner-done", "completed", "term-finished", true)
	outsiderRun := seedBulkRun(t, db, scenario.ID, "outsider", "active", "term-outsider", true)

	gin.SetMode(gin.TestMode)
	router := setupArchiveRouter(t, db, "org-manager", []string{"member"})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/v1/scenario-assignments/"+assignment.ID.String(), nil))
	require.Equal(t, http.StatusNoContent, w.Code, "body: %s", w.Body.String())

	// Controls: the abandon itself happened, a failed deletion included.
	assert.Equal(t, "abandoned", sessionStatus(t, db, openRun))
	assert.Equal(t, "abandoned", sessionStatus(t, db, noTerminalRun))
	assert.Equal(t, "abandoned", sessionStatus(t, db, missingRun),
		"a terminal that cannot be deleted must not undo the abandon")
	assert.Equal(t, "completed", sessionStatus(t, db, finishedRun))
	assert.Equal(t, "active", sessionStatus(t, db, outsiderRun), "a non-member's run is not touched")

	rec.mu.Lock()
	deleted := append([]string(nil), rec.deleted...)
	rec.mu.Unlock()
	assert.Equal(t, []string{"term-open"}, deleted,
		"the abandoned run's terminal must be deleted on tt-backend, and only it")
	assert.Equal(t, terminalModels.StateDeleted, terminalState(t, db, "term-open"),
		"the abandoned run's terminal must be marked deleted, so it stops counting against the budget")
	assert.Equal(t, terminalModels.StateRunning, terminalState(t, db, "term-finished"))
	assert.Equal(t, terminalModels.StateRunning, terminalState(t, db, "term-outsider"))
}

// A failed org delete rolls the abandon back, so no terminal may be released:
// the runs are still open and their learners still on them.
func TestOrgDeleteScenario_TransactionFails_ReleasesNoTerminal(t *testing.T) {
	srv, rec := newPersistenceTTBackend(t)
	configureTTServerForPersistence(t, srv.URL)
	db := freshTestDB(t)
	f := seedScenarioDeleteFixture(t, db, "org-manager")

	const failDelete = "test:fail_scenario_delete"
	require.NoError(t, db.Callback().Delete().Before("gorm:delete").Register(failDelete, func(tx *gorm.DB) {
		if tx.Statement.Table == "scenarios" {
			_ = tx.AddError(errors.New("simulated scenario delete failure"))
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Delete().Remove(failDelete) })

	router := setupOrgTestRouterWithUserAndRoles(t, db, "org-manager", []string{"Member"})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodDelete,
		"/api/v1/organizations/"+f.orgID.String()+"/scenarios/"+f.scenarioID.String(), nil))
	require.Equal(t, http.StatusInternalServerError, w.Code, "body: %s", w.Body.String())

	assert.Equal(t, "active", sessionStatus(t, db, f.openRun), "the abandon must roll back with the delete")
	rec.mu.Lock()
	defer rec.mu.Unlock()
	assert.Empty(t, rec.deleted, "nothing was committed, so no terminal may be released")
	assert.Equal(t, terminalModels.StateRunning, terminalState(t, db, "term-open"))
}

// seedLearnerOnTrialPlan gives the learner a plan, which a bulk start resolves
// before it builds the learner's container.
func seedLearnerOnTrialPlan(t *testing.T, db *gorm.DB, userID string) {
	t.Helper()
	plan := paymentModels.SubscriptionPlan{Name: userID + "-plan", IsActive: true, MaxSessionDurationMinutes: 240}
	require.NoError(t, db.Create(&plan).Error)
	require.NoError(t, db.Create(&paymentModels.UserSubscription{
		UserID: userID, SubscriptionPlanID: plan.ID, Status: "active",
		CurrentPeriodStart: time.Now().Add(-24 * time.Hour), CurrentPeriodEnd: time.Now().Add(30 * 24 * time.Hour),
	}).Error)
}

// Restarting a class replaces each learner's open run: the replaced run's
// terminal must go, the way the learner's own abandon destroys it, while the
// new run's terminal and a finished run's are kept.
func TestBulkStartScenario_ReplacedRun_DeletesItsTerminal(t *testing.T) {
	db := freshTestDB(t)
	scenario := models.Scenario{Name: "replace-term", Title: "Replace Term", InstanceType: "xs", CreatedByID: "c1"}
	require.NoError(t, db.Create(&scenario).Error)
	require.NoError(t, db.Create(&models.ScenarioStep{ScenarioID: scenario.ID, Order: 0, Title: "Step", TextContent: "content"}).Error)
	groupID := createClassGroup(t, db, "replace-term-class", "trainer", nil).ID
	const learner = "replace-term-s1"
	addGroupMember(t, db, groupID, learner, groupModels.GroupMemberRoleMember)
	seedLearnerOnTrialPlan(t, db, learner)
	finishedRun := seedBulkRun(t, db, scenario.ID, learner, "completed", "term-replace-done", false)
	oldRun := seedBulkRun(t, db, scenario.ID, learner, "active", "term-replace-old", false)

	ttMock := newMockTTService()
	ttMock.addKey(learner)
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, &mockVerificationService{})
	dashSvc := services.NewTeacherDashboardService(db, ttMock, sessionSvc)

	result, err := dashSvc.BulkStartScenario(groupID, scenario.ID, services.ScenarioProvisioning{Distribution: "Debian", Size: "xs"}, 0, "trainer")
	require.NoError(t, err)
	require.Empty(t, result.Errors)
	require.Equal(t, 1, result.Replaced)

	// Controls: the old run was replaced by a new one on a new terminal.
	assert.Equal(t, "abandoned", sessionStatus(t, db, oldRun))
	assert.Equal(t, "completed", sessionStatus(t, db, finishedRun))
	var newRun models.ScenarioSession
	require.NoError(t, db.First(&newRun, "user_id = ? AND scenario_id = ? AND status IN ?", learner, scenario.ID, models.OpenSessionStatuses).Error)
	require.NotNil(t, newRun.TerminalSessionID)
	assert.Equal(t, "terminal-"+learner, *newRun.TerminalSessionID)

	assert.Equal(t, []string{"term-replace-old"}, ttMock.DeletedSessions(),
		"the replaced run's terminal must be deleted, and neither the new run's nor a finished run's")
}

// A class reset abandons the learners' open runs: their terminals must go too,
// a finished run's is kept.
func TestResetGroupScenarioSessions_DeletesTerminalsOfAbandonedRuns(t *testing.T) {
	db := freshTestDB(t)
	groupID, scenarioID := uuid.New(), uuid.New()
	for _, learner := range []string{"reset-term-open", "reset-term-done"} {
		addGroupMember(t, db, groupID, learner, groupModels.GroupMemberRoleMember)
	}
	openRun := seedBulkRun(t, db, scenarioID, "reset-term-open", "active", "term-reset-open", false)
	finishedRun := seedBulkRun(t, db, scenarioID, "reset-term-done", "completed", "term-reset-done", false)

	ttMock := newMockTTService()
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, &mockVerificationService{})
	dashSvc := services.NewTeacherDashboardService(db, ttMock, sessionSvc)

	count, _, err := dashSvc.ResetGroupScenarioSessions(groupID, scenarioID)
	require.NoError(t, err)

	// Controls: the reset itself happened, and only on the open run.
	assert.Equal(t, int64(1), count)
	assert.Equal(t, "abandoned", sessionStatus(t, db, openRun))
	assert.Equal(t, "completed", sessionStatus(t, db, finishedRun))

	assert.Equal(t, []string{"term-reset-open"}, ttMock.DeletedSessions(),
		"the reset run's terminal must be deleted, and only it")
}

// When the new container cannot be built, the replaced run stays abandoned and
// its terminal released: the learner is left with no open run, the same as
// before the release existed, minus a terminal that could no longer be resumed.
func TestBulkStartScenario_ReplaceThenNewRunFails_LeavesOldRunAbandonedAndReleased(t *testing.T) {
	db := freshTestDB(t)
	scenario := models.Scenario{Name: "replace-fail", Title: "Replace Fail", InstanceType: "xs", CreatedByID: "c1"}
	require.NoError(t, db.Create(&scenario).Error)
	require.NoError(t, db.Create(&models.ScenarioStep{ScenarioID: scenario.ID, Order: 0, Title: "Step", TextContent: "content"}).Error)
	groupID := createClassGroup(t, db, "replace-fail-class", "trainer", nil).ID
	const learner = "replace-fail-s1"
	addGroupMember(t, db, groupID, learner, groupModels.GroupMemberRoleMember)
	oldRun := seedBulkRun(t, db, scenario.ID, learner, "active", "term-replace-fail-old", false)

	ttMock := newMockTTService()
	ttMock.addKey(learner)
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, &mockVerificationService{})
	dashSvc := services.NewTeacherDashboardService(db, ttMock, sessionSvc)

	// No plan for the learner: building the new container fails.
	result, err := dashSvc.BulkStartScenario(groupID, scenario.ID, services.ScenarioProvisioning{Distribution: "Debian", Size: "xs"}, 0, "trainer")
	require.NoError(t, err)
	require.Len(t, result.Errors, 1, "the new run must fail for this test to mean anything")
	assert.Equal(t, "failed to resolve subscription plan", result.Errors[0].Error)
	assert.Equal(t, 0, result.Created)

	assert.Equal(t, "abandoned", sessionStatus(t, db, oldRun))
	var open int64
	require.NoError(t, db.Model(&models.ScenarioSession{}).
		Where("user_id = ? AND scenario_id = ? AND status IN ?", learner, scenario.ID, models.OpenSessionStatuses).
		Count(&open).Error)
	assert.Zero(t, open, "the learner is left with no open run")
	assert.Equal(t, []string{"term-replace-fail-old"}, ttMock.DeletedSessions())
}

// relaunchAfterFailedSetup seeds a setup_failed run (on oldTerminal when not
// empty) and relaunches the scenario on a new terminal through the session
// service wired the way production wires it.
func relaunchAfterFailedSetup(t *testing.T, name, oldTerminal string) *mockTTService {
	t.Helper()
	db := freshTestDB(t)
	scenario := models.Scenario{Name: name, Title: name, InstanceType: "xs", CreatedByID: "c1"}
	require.NoError(t, db.Create(&scenario).Error)
	require.NoError(t, db.Create(&models.ScenarioStep{ScenarioID: scenario.ID, Order: 0, Title: "Step", TextContent: "content"}).Error)
	learner := name + "-learner"
	oldRun := seedBulkRun(t, db, scenario.ID, learner, "setup_failed", oldTerminal, false)

	tt := newMockTTService()
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, &mockVerificationService{})
	services.WireTerminalCallbacks(sessionSvc, tt)

	newRun, err := sessionSvc.StartScenario(learner, scenario.ID, name+"-term-new", "")
	require.NoError(t, err)
	assert.Equal(t, "abandoned", sessionStatus(t, db, oldRun), "the failed run is replaced by the relaunch")
	require.NotNil(t, newRun.TerminalSessionID)
	assert.Equal(t, name+"-term-new", *newRun.TerminalSessionID)
	return tt
}

// A failed setup only stops its terminal, and a stopped terminal still holds
// budget: the relaunch that abandons the failed run must delete it.
func TestStartScenario_RelaunchAfterFailedSetup_DeletesTheFailedRunsTerminal(t *testing.T) {
	tt := relaunchAfterFailedSetup(t, "relaunch-failed", "relaunch-failed-term-old")
	assert.Equal(t, []string{"relaunch-failed-term-old"}, tt.DeletedSessions(),
		"the failed run's terminal must be deleted, and not the new run's")
}

func TestStartScenario_RelaunchAfterFailedSetupWithoutTerminal_DeletesNothing(t *testing.T) {
	tt := relaunchAfterFailedSetup(t, "relaunch-noterm", "")
	assert.Empty(t, tt.DeletedSessions())
}
