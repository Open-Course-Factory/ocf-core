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
	"soli/formations/src/scenarios/models"
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
