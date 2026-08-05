package scenarios_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	groupModels "soli/formations/src/groups/models"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
	terminalModels "soli/formations/src/terminalTrainer/models"
)

// teacher_live_progress_test.go — GET /teacher/groups/:groupId/live-progress,
// the merged per-learner class view (ocf-front #310). The endpoint joins three
// previously separate answers — supervision presence, scenario position, and
// assignment results — into one row per ACTIVE group member.
//
// The invariant these tests exist to protect: EVERY active member gets a row.
// The view is used to invigilate exams, so a learner who has done nothing must
// appear as "not started", never vanish.

// --- seeding helpers -------------------------------------------------------

// seedScenarioWithSteps creates a scenario with `titles` steps, ordered 0..n-1.
func seedScenarioWithSteps(t *testing.T, db *gorm.DB, name string, titles ...string) models.Scenario {
	t.Helper()
	scenario := models.Scenario{
		Name: name, Title: name, InstanceType: "ubuntu:22.04", CreatedByID: "teacher-lp",
	}
	require.NoError(t, db.Create(&scenario).Error)
	for i, title := range titles {
		require.NoError(t, db.Create(&models.ScenarioStep{
			ScenarioID: scenario.ID, Order: i, Title: title, StepType: "terminal",
		}).Error)
	}
	return scenario
}

// seedStepProgress creates one scenario_step_progress row. completedAt nil means
// the step is still open.
func seedStepProgress(t *testing.T, db *gorm.DB, sessionID uuid.UUID, order int, status string, hints int, completedAt *time.Time) {
	t.Helper()
	require.NoError(t, db.Create(&models.ScenarioStepProgress{
		SessionID: sessionID, StepOrder: order, Status: status,
		HintsRevealed: hints, CompletedAt: completedAt,
	}).Error)
}

// liveProgressByUserID indexes the response so assertions name the learner they
// are about rather than a slice position.
func liveProgressByUserID(rows []services.LearnerLiveProgress) map[string]services.LearnerLiveProgress {
	byUser := make(map[string]services.LearnerLiveProgress, len(rows))
	for _, row := range rows {
		byUser[row.UserID] = row
	}
	return byUser
}

// --- service tests ---------------------------------------------------------

// TestGetGroupLiveProgress_JoinsPresenceStepAndHintsOnOneRow is the endpoint's
// reason to exist: a learner who is connected AND mid-scenario AND has used
// hints must carry all three facts on a SINGLE row, so the class view never has
// to fan out to three endpoints and join on the client.
func TestGetGroupLiveProgress_JoinsPresenceStepAndHintsOnOneRow(t *testing.T) {
	db := setupTestDB(t)
	orgID := uuid.New()
	group := createClassGroup(t, db, "exam-class", "teacher-lp", &orgID)
	addGroupMember(t, db, group.ID, "student-live", groupModels.GroupMemberRoleMember)

	scenario := seedScenarioWithSteps(t, db, "lp-scenario", "Boot the box", "Find the flag", "Harden sshd")
	assignment := createScenarioAssignment(t, db, scenario.ID, &group.ID, nil, "group")

	// Connected: a running terminal in the GROUP's organization.
	createTerminalSession(t, db, "student-live", terminalModels.StateRunning, &orgID, time.Now().Add(time.Hour))

	// Mid-scenario: on step 1 of 3, having finished step 0 three minutes ago.
	startedAt := time.Now().Add(-10 * time.Minute)
	stepZeroDone := time.Now().Add(-3 * time.Minute)
	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "student-live", Status: "active",
		CurrentStep: 1, StartedAt: startedAt,
	}
	require.NoError(t, db.Create(&session).Error)
	seedStepProgress(t, db, session.ID, 0, "completed", 1, &stepZeroDone)
	seedStepProgress(t, db, session.ID, 1, "active", 2, nil)

	svc := services.NewTeacherDashboardService(db, nil, nil)
	rows, err := svc.GetGroupLiveProgress(group.ID)
	require.NoError(t, err)
	require.Len(t, rows, 1)

	row := rows[0]
	assert.Equal(t, "student-live", row.UserID)
	assert.True(t, row.Connected, "a running terminal in the group's org means present")
	require.Len(t, row.Assignments, 1)

	progress := row.Assignments[0]
	assert.Equal(t, assignment.ID, progress.AssignmentID)
	assert.Equal(t, scenario.ID, progress.ScenarioID)
	assert.Equal(t, services.LearnerStatusInProgress, progress.Status)
	assert.Equal(t, 1, progress.CurrentStep)
	assert.Equal(t, "Find the flag", progress.CurrentStepTitle, "the title of the step they are ON, not the scenario title")
	assert.Equal(t, 3, progress.TotalSteps)
	assert.Equal(t, 3, progress.HintsUsed, "1 on step 0 + 2 on step 1")

	require.NotNil(t, progress.CurrentStepElapsedSeconds, "elapsed runs from the previous step's completion")
	assert.InDelta(t, 180, *progress.CurrentStepElapsedSeconds, 30)
}

// TestGetGroupLiveProgress_MemberWithNoSessionAndNoTerminal_StillGetsRow is the
// critical case. In an exam view a silently missing row hides a student who has
// not started — precisely the student the invigilator must notice.
func TestGetGroupLiveProgress_MemberWithNoSessionAndNoTerminal_StillGetsRow(t *testing.T) {
	db := setupTestDB(t)
	orgID := uuid.New()
	group := createClassGroup(t, db, "silent-class", "teacher-lp", &orgID)
	addGroupMember(t, db, group.ID, "student-idle", groupModels.GroupMemberRoleMember)

	scenario := seedScenarioWithSteps(t, db, "untouched", "Step one", "Step two")
	createScenarioAssignment(t, db, scenario.ID, &group.ID, nil, "group")

	svc := services.NewTeacherDashboardService(db, nil, nil)
	rows, err := svc.GetGroupLiveProgress(group.ID)
	require.NoError(t, err)
	require.Len(t, rows, 1, "a member who did nothing must still be listed")

	row := rows[0]
	assert.Equal(t, "student-idle", row.UserID)
	assert.False(t, row.Connected)
	assert.Nil(t, row.LastActivityAt)
	require.Len(t, row.Assignments, 1, "the assignment is reported as untouched, not omitted")

	progress := row.Assignments[0]
	assert.Equal(t, services.LearnerStatusNotStarted, progress.Status)
	assert.Nil(t, progress.SessionID)
	assert.Equal(t, 0, progress.HintsUsed)
	assert.Equal(t, 2, progress.TotalSteps, "total steps come from the scenario, not from a session")
	assert.Nil(t, progress.Grade)
}

// TestGetGroupLiveProgress_PersonalSession_ReportsNotConnected pins the org
// scoping: a terminal the learner launched OUTSIDE the group's organization
// (personal work, another org) is not supervisable and must not read as presence
// in this class. The predicate is models.SupervisableByGroupOrgScope, shared with
// the supervision wall.
func TestGetGroupLiveProgress_PersonalSession_ReportsNotConnected(t *testing.T) {
	db := setupTestDB(t)
	orgID := uuid.New()
	otherOrgID := uuid.New()
	group := createClassGroup(t, db, "scoped-class", "teacher-lp", &orgID)
	addGroupMember(t, db, group.ID, "student-personal", groupModels.GroupMemberRoleMember)
	addGroupMember(t, db, group.ID, "student-otherorg", groupModels.GroupMemberRoleMember)

	// Personal session: no organization at all.
	createTerminalSession(t, db, "student-personal", terminalModels.StateRunning, nil, time.Now().Add(time.Hour))
	// A session running in a DIFFERENT org.
	createTerminalSession(t, db, "student-otherorg", terminalModels.StateRunning, &otherOrgID, time.Now().Add(time.Hour))

	svc := services.NewTeacherDashboardService(db, nil, nil)
	rows, err := svc.GetGroupLiveProgress(group.ID)
	require.NoError(t, err)
	require.Len(t, rows, 2)

	byUser := liveProgressByUserID(rows)
	assert.False(t, byUser["student-personal"].Connected, "a personal (NULL-org) session is never supervisable")
	assert.False(t, byUser["student-otherorg"].Connected, "another org's session is not visible to this class")
}

// TestGetGroupLiveProgress_ExpiredTerminal_ReportsNotConnected pins that presence
// uses models.RunningDisplayScope — the expiry-aware "alive right now" predicate
// — so a zombie row (state still 'running', tt-backend session long gone) does
// not show a learner as present.
func TestGetGroupLiveProgress_ExpiredTerminal_ReportsNotConnected(t *testing.T) {
	db := setupTestDB(t)
	orgID := uuid.New()
	group := createClassGroup(t, db, "zombie-class", "teacher-lp", &orgID)
	addGroupMember(t, db, group.ID, "student-zombie", groupModels.GroupMemberRoleMember)
	addGroupMember(t, db, group.ID, "student-stopped", groupModels.GroupMemberRoleMember)

	createTerminalSession(t, db, "student-zombie", terminalModels.StateRunning, &orgID, time.Now().Add(-time.Minute))
	createTerminalSession(t, db, "student-stopped", terminalModels.StateStopped, &orgID, time.Now().Add(time.Hour))

	svc := services.NewTeacherDashboardService(db, nil, nil)
	rows, err := svc.GetGroupLiveProgress(group.ID)
	require.NoError(t, err)

	byUser := liveProgressByUserID(rows)
	assert.False(t, byUser["student-zombie"].Connected, "past-expiry rows are not alive")
	assert.False(t, byUser["student-stopped"].Connected, "a stopped session is not a present learner")
}

// TestGetGroupLiveProgress_CompletedLearner_CarriesGradeAndHints verifies the
// end state of the exam view: a finished learner reports completed, their grade,
// and the hints they consumed getting there.
func TestGetGroupLiveProgress_CompletedLearner_CarriesGradeAndHints(t *testing.T) {
	db := setupTestDB(t)
	orgID := uuid.New()
	group := createClassGroup(t, db, "graded-class", "teacher-lp", &orgID)
	addGroupMember(t, db, group.ID, "student-done", groupModels.GroupMemberRoleMember)

	scenario := seedScenarioWithSteps(t, db, "graded", "One", "Two")
	createScenarioAssignment(t, db, scenario.ID, &group.ID, nil, "group")

	grade := 87.5
	completedAt := time.Now().Add(-time.Minute)
	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "student-done", Status: "completed",
		CurrentStep: 1, StartedAt: time.Now().Add(-time.Hour),
		CompletedAt: &completedAt, Grade: &grade,
	}
	require.NoError(t, db.Create(&session).Error)
	seedStepProgress(t, db, session.ID, 0, "completed", 2, &completedAt)
	seedStepProgress(t, db, session.ID, 1, "completed", 1, &completedAt)

	svc := services.NewTeacherDashboardService(db, nil, nil)
	rows, err := svc.GetGroupLiveProgress(group.ID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Len(t, rows[0].Assignments, 1)

	progress := rows[0].Assignments[0]
	assert.Equal(t, services.LearnerStatusCompleted, progress.Status)
	require.NotNil(t, progress.Grade)
	assert.InDelta(t, 87.5, *progress.Grade, 0.01)
	assert.Equal(t, 3, progress.HintsUsed)
	assert.NotNil(t, progress.CompletedAt)
	assert.Nil(t, progress.CurrentStepElapsedSeconds, "a finished learner is not sitting on a step")
}

// TestGetGroupLiveProgress_PreviewSession_Ignored keeps the teacher view aligned
// with every other teacher aggregate: a trainer's own preview run is not a
// learner attempt.
func TestGetGroupLiveProgress_PreviewSession_Ignored(t *testing.T) {
	db := setupTestDB(t)
	orgID := uuid.New()
	group := createClassGroup(t, db, "preview-class", "teacher-lp", &orgID)
	addGroupMember(t, db, group.ID, "student-preview", groupModels.GroupMemberRoleMember)

	scenario := seedScenarioWithSteps(t, db, "previewed", "Only step")
	createScenarioAssignment(t, db, scenario.ID, &group.ID, nil, "group")

	require.NoError(t, db.Create(&models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "student-preview", Status: "active",
		CurrentStep: 0, StartedAt: time.Now(), IsPreview: true,
	}).Error)

	svc := services.NewTeacherDashboardService(db, nil, nil)
	rows, err := svc.GetGroupLiveProgress(group.ID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Len(t, rows[0].Assignments, 1)
	assert.Equal(t, services.LearnerStatusNotStarted, rows[0].Assignments[0].Status)
}

// TestGetGroupLiveProgress_InactiveMember_Excluded pins the population: ACTIVE
// memberships only, the same one every other teacher aggregate counts over.
func TestGetGroupLiveProgress_InactiveMember_Excluded(t *testing.T) {
	db := setupTestDB(t)
	orgID := uuid.New()
	group := createClassGroup(t, db, "churn-class", "teacher-lp", &orgID)
	addGroupMember(t, db, group.ID, "student-active", groupModels.GroupMemberRoleMember)
	addGroupMember(t, db, group.ID, "student-left", groupModels.GroupMemberRoleMember)
	// GroupMember.IsActive carries `gorm:"default:true"`, so Create rewrites an
	// explicit false back to true — the membership has to be deactivated after
	// insert to model a learner who left.
	require.NoError(t, db.Model(&groupModels.GroupMember{}).
		Where("group_id = ? AND user_id = ?", group.ID, "student-left").
		UpdateColumn("is_active", false).Error)

	svc := services.NewTeacherDashboardService(db, nil, nil)
	rows, err := svc.GetGroupLiveProgress(group.ID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "student-active", rows[0].UserID)
}

// TestGetGroupLiveProgress_EmptyGroup_ReturnsEmptySlice pins the zero-input case:
// a class with no member marshals to [], never null, so the frontend can render
// it without a nil guard.
func TestGetGroupLiveProgress_EmptyGroup_ReturnsEmptySlice(t *testing.T) {
	db := setupTestDB(t)
	orgID := uuid.New()
	group := createClassGroup(t, db, "empty-class", "teacher-lp", &orgID)

	svc := services.NewTeacherDashboardService(db, nil, nil)
	rows, err := svc.GetGroupLiveProgress(group.ID)
	require.NoError(t, err)
	assert.NotNil(t, rows, "must be an empty slice, not nil")
	assert.Empty(t, rows)
}

// TestGetGroupLiveProgress_UnknownGroup_ReturnsEmptySlice pins the other
// zero-input edge: a well-formed id for a group that does not exist yields an
// empty listing rather than an error or a panic.
func TestGetGroupLiveProgress_UnknownGroup_ReturnsEmptySlice(t *testing.T) {
	db := setupTestDB(t)

	svc := services.NewTeacherDashboardService(db, nil, nil)
	rows, err := svc.GetGroupLiveProgress(uuid.New())
	require.NoError(t, err)
	assert.NotNil(t, rows)
	assert.Empty(t, rows)
}

// --- HTTP / authorization tests --------------------------------------------

// TestGetGroupLiveProgressAPI_Manager_Returns200 verifies the route is reachable
// by a group manager and serialises as a JSON array.
func TestGetGroupLiveProgressAPI_Manager_Returns200(t *testing.T) {
	db := setupTestDB(t)
	orgID := uuid.New()
	group := createClassGroup(t, db, "api-class", "other-teacher", &orgID)
	addGroupMember(t, db, group.ID, "teacher-manager", groupModels.GroupMemberRoleManager)
	addGroupMember(t, db, group.ID, "student-api", groupModels.GroupMemberRoleMember)

	router := setupRealTeacherRouter(t, db, "teacher-manager", []string{"member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/teacher/groups/"+group.ID.String()+"/live-progress", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var rows []services.LearnerLiveProgress
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rows))
	require.Len(t, rows, 2, "both the manager and the student hold active memberships")

	byUser := liveProgressByUserID(rows)
	require.Contains(t, byUser, "student-api")
	assert.NotNil(t, byUser["student-api"].Assignments, "assignments marshal as [], never null")
}

// TestGetGroupLiveProgressAPI_PlainMember_Returns403 — a learner must not read
// their classmates' positions. Layer 2 GroupRole(manager) is the gate.
func TestGetGroupLiveProgressAPI_PlainMember_Returns403(t *testing.T) {
	db := setupTestDB(t)
	group := createClassGroup(t, db, "denied-class", "other-teacher", nil)
	addGroupMember(t, db, group.ID, "student-nosy", groupModels.GroupMemberRoleMember)

	router := setupRealTeacherRouter(t, db, "student-nosy", []string{"member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/teacher/groups/"+group.ID.String()+"/live-progress", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

// TestGetGroupLiveProgressAPI_NonMember_Returns403 — no tie to the class at all.
func TestGetGroupLiveProgressAPI_NonMember_Returns403(t *testing.T) {
	db := setupTestDB(t)
	group := createClassGroup(t, db, "stranger-class", "other-teacher", nil)

	router := setupRealTeacherRouter(t, db, "random-stranger", []string{"member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/teacher/groups/"+group.ID.String()+"/live-progress", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

// TestGetGroupLiveProgressAPI_GarbageGroupID_Returns4xx — an unparseable id is a
// client error, never a 500.
func TestGetGroupLiveProgressAPI_GarbageGroupID_Returns4xx(t *testing.T) {
	db := setupTestDB(t)
	router := setupRealTeacherRouter(t, db, "platform-admin", []string{"admin"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/teacher/groups/not-a-uuid/live-progress", nil)
	router.ServeHTTP(w, req)

	assert.GreaterOrEqual(t, w.Code, 400)
	assert.Less(t, w.Code, 500, "a malformed path parameter must not surface as a server error")
}

// --- "Mes classes" idle summary --------------------------------------------

// TestGetManagedGroupsOverview_ConnectedButStaleLearner_CountsAsIdle covers the
// console's "N inactifs" badge: a learner who is CONNECTED but whose last
// scenario activity is older than the threshold is the "stuck but not asking"
// signal the teacher needs. The threshold travels in the response so the label
// and the predicate can never drift apart.
func TestGetManagedGroupsOverview_ConnectedButStaleLearner_CountsAsIdle(t *testing.T) {
	db := setupTestDB(t)
	orgID := uuid.New()
	group := createClassGroup(t, db, "idle-class", "teacher-idle", &orgID)
	addGroupMember(t, db, group.ID, "student-stale", groupModels.GroupMemberRoleMember)
	addGroupMember(t, db, group.ID, "student-busy", groupModels.GroupMemberRoleMember)

	scenario := seedScenarioWithSteps(t, db, "idle-scenario", "One", "Two")
	createScenarioAssignment(t, db, scenario.ID, &group.ID, nil, "group")

	createTerminalSession(t, db, "student-stale", terminalModels.StateRunning, &orgID, time.Now().Add(time.Hour))
	createTerminalSession(t, db, "student-busy", terminalModels.StateRunning, &orgID, time.Now().Add(time.Hour))

	stale := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "student-stale", Status: "active",
		CurrentStep: 0, StartedAt: time.Now().Add(-2 * time.Hour),
	}
	require.NoError(t, db.Create(&stale).Error)
	busy := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "student-busy", Status: "active",
		CurrentStep: 0, StartedAt: time.Now().Add(-2 * time.Hour),
	}
	require.NoError(t, db.Create(&busy).Error)

	// GORM stamps updated_at on write, so both rows start "just active"; age the
	// stale learner's activity past the threshold.
	seedStepProgress(t, db, stale.ID, 0, "active", 0, nil)
	seedStepProgress(t, db, busy.ID, 0, "active", 0, nil)
	longAgo := time.Now().Add(-90 * time.Minute)
	require.NoError(t, db.Model(&models.ScenarioStepProgress{}).
		Where("session_id = ?", stale.ID).
		UpdateColumn("updated_at", longAgo).Error)

	svc := services.NewTeacherDashboardService(db, nil, nil)
	items, err := svc.GetManagedGroupsOverview("teacher-idle")
	require.NoError(t, err)
	require.Len(t, items, 1)

	summary := items[0]
	assert.Equal(t, 2, summary.LiveSessionCount)
	assert.Equal(t, 1, summary.IdleMemberCount, "only the learner with stale activity is idle")
	assert.Equal(t, int(services.LearnerIdleThreshold.Minutes()), summary.IdleThresholdMinutes,
		"the threshold travels with the count so the UI label cannot drift from the predicate")
}

// TestGetManagedGroupsOverview_DisconnectedLearner_NotCountedAsIdle keeps the two
// signals distinct: a learner with no live session is ABSENT, which the class
// view already shows as connected=false. Counting them as idle too would double
// report the same student under a label that means something else.
func TestGetManagedGroupsOverview_DisconnectedLearner_NotCountedAsIdle(t *testing.T) {
	db := setupTestDB(t)
	orgID := uuid.New()
	group := createClassGroup(t, db, "absent-class", "teacher-absent", &orgID)
	addGroupMember(t, db, group.ID, "student-away", groupModels.GroupMemberRoleMember)

	scenario := seedScenarioWithSteps(t, db, "absent-scenario", "One")
	createScenarioAssignment(t, db, scenario.ID, &group.ID, nil, "group")

	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "student-away", Status: "active",
		CurrentStep: 0, StartedAt: time.Now().Add(-2 * time.Hour),
	}
	require.NoError(t, db.Create(&session).Error)
	seedStepProgress(t, db, session.ID, 0, "active", 0, nil)
	require.NoError(t, db.Model(&models.ScenarioStepProgress{}).
		Where("session_id = ?", session.ID).
		UpdateColumn("updated_at", time.Now().Add(-90*time.Minute)).Error)

	svc := services.NewTeacherDashboardService(db, nil, nil)
	items, err := svc.GetManagedGroupsOverview("teacher-absent")
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, 0, items[0].IdleMemberCount, "no live session means absent, not idle")
}
