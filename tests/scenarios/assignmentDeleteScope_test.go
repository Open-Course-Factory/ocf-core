// tests/scenarios/assignmentDeleteScope_test.go
//
// Removing a class's assignment abandons (and releases the terminal of) only
// the runs that assignment gave: a plain learner's, the way
// ResetGroupScenarioSessions scopes a class reset. A class manager's or
// owner's run, a preview run, and the run of a learner who still reaches the
// scenario through another assignment are left alone.
package scenarios_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	groupModels "soli/formations/src/groups/models"
	orgModels "soli/formations/src/organizations/models"
	"soli/formations/src/scenarios/models"
	terminalModels "soli/formations/src/terminalTrainer/models"
)

type assignmentDeleteOutcome struct {
	runs    map[string]uuid.UUID // by user
	deleted []string             // terminals deleted on tt-backend
}

// deleteClassAssignment seeds one class whose members each hold an open run
// on a terminal "term-<user>", deletes the class's assignment through the
// generic route, and reports what happened.
func deleteClassAssignment(t *testing.T) (*assignmentDeleteOutcome, func(user string) string) {
	t.Helper()
	srv, rec := newPersistenceTTBackend(t)
	configureTTServerForPersistence(t, srv.URL)
	db := freshTestDB(t)
	orgID := createTestOrg(t, db, "org-owner")
	addOrgMember(t, db, orgID, "org-manager", orgModels.OrgRoleManager)
	scenario := createTestScenarioForOrg(t, db, orgID, "assignment-scope")
	groupID := createTestGroupInOrg(t, db, orgID, "org-owner")
	members := map[string]groupModels.GroupMemberRole{
		"plain-learner":   groupModels.GroupMemberRoleMember,
		"class-manager":   groupModels.GroupMemberRoleManager,
		"class-owner":     groupModels.GroupMemberRoleOwner,
		"preview-learner": groupModels.GroupMemberRoleMember,
		"dual-learner":    groupModels.GroupMemberRoleMember,
	}
	for user, role := range members {
		addGroupMember(t, db, groupID, user, role)
	}
	assignment := createScenarioAssignment(t, db, scenario.ID, &groupID, &orgID, "group")

	// dual-learner is also in a second class the scenario stays assigned to.
	otherClass := createTestGroupInOrg(t, db, orgID, "org-owner")
	addGroupMember(t, db, otherClass, "dual-learner", groupModels.GroupMemberRoleMember)
	createScenarioAssignment(t, db, scenario.ID, &otherClass, &orgID, "group")

	out := &assignmentDeleteOutcome{runs: map[string]uuid.UUID{}}
	for user := range members {
		out.runs[user] = seedBulkRun(t, db, scenario.ID, user, "active", "term-"+user, true)
	}
	require.NoError(t, db.Model(&models.ScenarioSession{}).Where("id = ?", out.runs["preview-learner"]).
		Update("is_preview", true).Error)

	router := setupArchiveRouter(t, db, "org-manager", []string{"member"})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/v1/scenario-assignments/"+assignment.ID.String(), nil))
	require.Equal(t, http.StatusNoContent, w.Code, "body: %s", w.Body.String())

	// Control: the run only that assignment gave is abandoned and released.
	assert.Equal(t, "abandoned", sessionStatus(t, db, out.runs["plain-learner"]))
	assert.Equal(t, terminalModels.StateDeleted, terminalState(t, db, "term-plain-learner"))

	rec.mu.Lock()
	out.deleted = append([]string(nil), rec.deleted...)
	rec.mu.Unlock()
	return out, func(user string) string { return sessionStatus(t, db, out.runs[user]) }
}

func assertRunKept(t *testing.T, user string) {
	t.Helper()
	out, status := deleteClassAssignment(t)
	assert.Equal(t, "active", status(user), "%s's run must not be abandoned", user)
	assert.NotContains(t, out.deleted, "term-"+user, "%s's terminal must not be deleted", user)
}

func TestDeleteScenarioAssignment_KeepsClassManagerRun(t *testing.T) {
	assertRunKept(t, "class-manager")
}

func TestDeleteScenarioAssignment_KeepsClassOwnerRun(t *testing.T) {
	assertRunKept(t, "class-owner")
}

func TestDeleteScenarioAssignment_KeepsPreviewRun(t *testing.T) {
	assertRunKept(t, "preview-learner")
}

func TestDeleteScenarioAssignment_KeepsRunReachedThroughAnotherAssignment(t *testing.T) {
	assertRunKept(t, "dual-learner")
}

// --- what "still reaches the scenario" counts ------------------------------

func deleteAssignmentAs(t *testing.T, db *gorm.DB, userID string, roles []string, assignmentID uuid.UUID) {
	t.Helper()
	router := setupArchiveRouter(t, db, userID, roles)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/v1/scenario-assignments/"+assignmentID.String(), nil))
	require.Equal(t, http.StatusNoContent, w.Code, "body: %s", w.Body.String())
}

// twoClassLearner seeds a learner in two classes of one org that both assign
// the scenario, with an open run on their own terminal.
type twoClassLearner struct {
	db                       *gorm.DB
	rec                      *persistenceRecorder
	scenarioID               uuid.UUID
	classA, classB           uuid.UUID
	assignmentA, assignmentB uuid.UUID
	run                      uuid.UUID
}

func seedTwoClassLearner(t *testing.T) twoClassLearner {
	t.Helper()
	srv, rec := newPersistenceTTBackend(t)
	configureTTServerForPersistence(t, srv.URL)
	db := freshTestDB(t)
	orgID := createTestOrg(t, db, "org-owner")
	addOrgMember(t, db, orgID, "org-manager", orgModels.OrgRoleManager)
	scenario := createTestScenarioForOrg(t, db, orgID, "two-classes")
	f := twoClassLearner{db: db, rec: rec, scenarioID: scenario.ID}
	f.classA = createTestGroupInOrg(t, db, orgID, "org-owner")
	f.classB = createTestGroupInOrg(t, db, orgID, "org-owner")
	for _, class := range []uuid.UUID{f.classA, f.classB} {
		addGroupMember(t, db, class, "two-class-learner", groupModels.GroupMemberRoleMember)
	}
	f.assignmentA = createScenarioAssignment(t, db, scenario.ID, &f.classA, &orgID, "group").ID
	f.assignmentB = createScenarioAssignment(t, db, scenario.ID, &f.classB, &orgID, "group").ID
	f.run = seedBulkRun(t, db, scenario.ID, "two-class-learner", "active", "term-two-class", true)
	return f
}

func (f twoClassLearner) deleted() []string {
	f.rec.mu.Lock()
	defer f.rec.mu.Unlock()
	return append([]string(nil), f.rec.deleted...)
}

func TestDeleteScenarioAssignment_BothClassAssignmentsDeleted_SecondAbandons(t *testing.T) {
	f := seedTwoClassLearner(t)

	deleteAssignmentAs(t, f.db, "org-manager", []string{"member"}, f.assignmentA)
	assert.Equal(t, "active", sessionStatus(t, f.db, f.run), "class B still assigns the scenario")

	deleteAssignmentAs(t, f.db, "org-manager", []string{"member"}, f.assignmentB)
	assert.Equal(t, "abandoned", sessionStatus(t, f.db, f.run), "nothing assigns it any more")
	assert.Equal(t, []string{"term-two-class"}, f.deleted())
}

func TestDeleteScenarioAssignment_OtherAssignmentInactive_Abandons(t *testing.T) {
	f := seedTwoClassLearner(t)
	require.NoError(t, f.db.Model(&models.ScenarioAssignment{}).Where("id = ?", f.assignmentB).Update("is_active", false).Error)

	deleteAssignmentAs(t, f.db, "org-manager", []string{"member"}, f.assignmentA)

	assert.Equal(t, "abandoned", sessionStatus(t, f.db, f.run), "an inactive assignment reaches nobody")
	assert.Equal(t, []string{"term-two-class"}, f.deleted())
}

func TestDeleteScenarioAssignment_OtherClassArchived_Abandons(t *testing.T) {
	f := seedTwoClassLearner(t)
	require.NoError(t, f.db.Model(&groupModels.ClassGroup{}).Where("id = ?", f.classB).Update("archived_at", time.Now()).Error)

	deleteAssignmentAs(t, f.db, "org-manager", []string{"member"}, f.assignmentA)

	assert.Equal(t, "abandoned", sessionStatus(t, f.db, f.run), "an archived class's assignment reaches nobody")
	assert.Equal(t, []string{"term-two-class"}, f.deleted())
}

// Pins current behaviour (unchanged from main): the cleanup only follows class
// assignments, so removing an organisation-wide one abandons nothing.
func TestDeleteScenarioAssignment_OrgWide_AbandonsNothing(t *testing.T) {
	srv, rec := newPersistenceTTBackend(t)
	configureTTServerForPersistence(t, srv.URL)
	db := freshTestDB(t)
	orgID := createTestOrg(t, db, "org-owner")
	addOrgMember(t, db, orgID, "org-manager", orgModels.OrgRoleManager)
	addOrgMember(t, db, orgID, "org-learner", orgModels.OrgRoleMember)
	scenario := createTestScenarioForOrg(t, db, orgID, "org-wide")
	assignment := createScenarioAssignment(t, db, scenario.ID, nil, &orgID, "org")
	run := seedBulkRun(t, db, scenario.ID, "org-learner", "active", "term-org-learner", true)

	deleteAssignmentAs(t, db, "org-manager", []string{"member"}, assignment.ID)

	assert.Equal(t, "active", sessionStatus(t, db, run))
	rec.mu.Lock()
	defer rec.mu.Unlock()
	assert.Empty(t, rec.deleted)
}

// Pins the choice made with the fix: a scenario of the public catalogue
// reaches every learner, so removing a class's assignment keeps their runs.
func TestDeleteScenarioAssignment_PublicScenario_KeepsRuns(t *testing.T) {
	srv, rec := newPersistenceTTBackend(t)
	configureTTServerForPersistence(t, srv.URL)
	db := freshTestDB(t)
	scenario := models.Scenario{Name: "public-assigned", Title: "Public Assigned", InstanceType: "xs", CreatedByID: "author", IsPublic: true}
	require.NoError(t, db.Create(&scenario).Error)
	class := createClassGroup(t, db, "public-class", "trainer", nil).ID
	addGroupMember(t, db, class, "public-learner", groupModels.GroupMemberRoleMember)
	assignment := createScenarioAssignment(t, db, scenario.ID, &class, nil, "group")
	run := seedBulkRun(t, db, scenario.ID, "public-learner", "active", "term-public-learner", true)

	deleteAssignmentAs(t, db, "platform-admin", []string{"administrator"}, assignment.ID)

	assert.Equal(t, "active", sessionStatus(t, db, run))
	rec.mu.Lock()
	defer rec.mu.Unlock()
	assert.Empty(t, rec.deleted)
}
