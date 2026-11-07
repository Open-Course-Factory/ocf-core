package scenarios_test

// An exam gets one attempt per learner: once an exam-mode quiz step is
// submitted, no new run of the scenario starts until the teacher allows a
// retake. Every start path enforces it, and the teacher's bulk actions skip a
// locked learner rather than wiping their run.

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

	"soli/formations/src/entityManagement/hooks"
	groupModels "soli/formations/src/groups/models"
	"soli/formations/src/scenarios/models"
	scenarioHooks "soli/formations/src/scenarios/hooks"
	"soli/formations/src/scenarios/services"
)

const examQuizOrder = 5

// addExamQuizStep gives the scenario an exam-mode quiz step.
func addExamQuizStep(t *testing.T, db *gorm.DB, scenarioID uuid.UUID) {
	t.Helper()
	require.NoError(t, db.Create(&models.ScenarioStep{
		ScenarioID: scenarioID, Order: examQuizOrder, Title: "Exam", StepType: "quiz", ShowImmediateFeedback: false,
	}).Error)
}

// seedExamRun records a run of the scenario by userID, who is enrolled in a
// class assigning it: the lock only exists where a teacher can lift it.
// submitted writes the exam step's score, which is what a submission leaves
// behind.
func seedExamRun(t *testing.T, db *gorm.DB, scenarioID uuid.UUID, userID, status string, submitted, preview bool) models.ScenarioSession {
	t.Helper()
	enrolInExamClass(t, db, scenarioID, userID)
	session := models.ScenarioSession{
		ScenarioID: scenarioID, UserID: userID, Status: status, StartedAt: time.Now(), IsPreview: preview,
	}
	require.NoError(t, db.Create(&session).Error)
	progress := models.ScenarioStepProgress{SessionID: session.ID, StepOrder: examQuizOrder, Status: "active"}
	if submitted {
		score := 0.5
		progress.QuizScore = &score
		progress.Status = "completed"
	}
	require.NoError(t, db.Create(&progress).Error)
	return session
}

// enrolInExamClass makes userID an active member of the class assigned the
// scenario, creating that class on first use.
func enrolInExamClass(t *testing.T, db *gorm.DB, scenarioID uuid.UUID, userID string) groupModels.ClassGroup {
	t.Helper()
	name := "exam-class-" + scenarioID.String()
	var group groupModels.ClassGroup
	if db.Where("name = ?", name).First(&group).Error != nil {
		group = createClassGroup(t, db, name, "teacher-1", nil)
		createScenarioAssignment(t, db, scenarioID, &group.ID, nil, "group")
	}
	var enrolled int64
	require.NoError(t, db.Model(&groupModels.GroupMember{}).
		Where("group_id = ? AND user_id = ?", group.ID, userID).Count(&enrolled).Error)
	if enrolled == 0 {
		addGroupMember(t, db, group.ID, userID, groupModels.GroupMemberRoleMember)
	}
	return group
}

func examScenario(t *testing.T, db *gorm.DB) models.Scenario {
	t.Helper()
	scenario := models.Scenario{Name: "exam-" + uuid.NewString(), Title: "Exam", InstanceType: "ubuntu:22.04", CreatedByID: "teacher-1"}
	require.NoError(t, db.Create(&scenario).Error)
	require.NoError(t, db.Create(&models.ScenarioStep{ScenarioID: scenario.ID, Order: 0, Title: "Intro", StepType: "terminal"}).Error)
	addExamQuizStep(t, db, scenario.ID)
	return scenario
}

func examLocked(t *testing.T, scenarioID uuid.UUID, userID string) bool {
	t.Helper()
	locked, err := services.ExamLocked(sharedTestDB, userID, scenarioID)
	require.NoError(t, err)
	return locked
}

func TestExamLock_SubmittedExam_LocksTheLearner(t *testing.T) {
	scenario := examScenario(t, freshTestDB(t))
	seedExamRun(t, sharedTestDB, scenario.ID, "learner-a", "completed", true, false)
	assert.True(t, examLocked(t, scenario.ID, "learner-a"))
	assert.False(t, examLocked(t, scenario.ID, "learner-b"), "the lock is per learner")
}

// No class of the learner assigns the scenario: no teacher could ever allow a
// retake, so there is no lock.
func TestExamLock_NoClassAssignsTheScenario_DoesNotLock(t *testing.T) {
	scenario := examScenario(t, freshTestDB(t))
	seedExamRun(t, sharedTestDB, scenario.ID, "learner-a", "completed", true, false)
	require.NoError(t, sharedTestDB.Where("scenario_id = ?", scenario.ID).Delete(&models.ScenarioAssignment{}).Error)
	orgID := uuid.New()
	createScenarioAssignment(t, sharedTestDB, scenario.ID, nil, &orgID, "org")

	assert.False(t, examLocked(t, scenario.ID, "learner-a"))
}

// A learner who left the class counts as having no class.
func TestExamLock_ClassMembershipInactive_DoesNotLock(t *testing.T) {
	scenario := examScenario(t, freshTestDB(t))
	seedExamRun(t, sharedTestDB, scenario.ID, "learner-a", "completed", true, false)
	require.NoError(t, sharedTestDB.Model(&groupModels.GroupMember{}).
		Where("user_id = ?", "learner-a").Update("is_active", false).Error)

	assert.False(t, examLocked(t, scenario.ID, "learner-a"))
}

func TestExamLock_ExamNotYetSubmitted_DoesNotLock(t *testing.T) {
	scenario := examScenario(t, freshTestDB(t))
	seedExamRun(t, sharedTestDB, scenario.ID, "learner-a", "active", false, false)
	assert.False(t, examLocked(t, scenario.ID, "learner-a"))
}

func TestExamLock_PreviewRun_NeverLocks(t *testing.T) {
	scenario := examScenario(t, freshTestDB(t))
	seedExamRun(t, sharedTestDB, scenario.ID, "teacher-1", "completed", true, true)
	assert.False(t, examLocked(t, scenario.ID, "teacher-1"))
}

func TestExamLock_LearningQuizSubmitted_DoesNotLock(t *testing.T) {
	db := freshTestDB(t)
	scenario := models.Scenario{Name: "learning-" + uuid.NewString(), Title: "Learning", InstanceType: "ubuntu:22.04", CreatedByID: "teacher-1"}
	require.NoError(t, db.Create(&scenario).Error)
	require.NoError(t, db.Create(&models.ScenarioStep{
		ScenarioID: scenario.ID, Order: examQuizOrder, Title: "Practice", StepType: "quiz", ShowImmediateFeedback: true,
	}).Error)
	seedExamRun(t, db, scenario.ID, "learner-a", "completed", true, false)
	assert.False(t, examLocked(t, scenario.ID, "learner-a"))
}

func TestStartScenario_ExamLocked_RefusesANewRun(t *testing.T) {
	scenario := examScenario(t, freshTestDB(t))
	seedExamRun(t, sharedTestDB, scenario.ID, "learner-a", "completed", true, false)

	svc := services.NewScenarioSessionService(sharedTestDB, &mockFlagService{}, &mockVerificationService{})
	_, err := svc.StartScenario("learner-a", scenario.ID, "", "")
	assert.ErrorIs(t, err, services.ErrExamAlreadySubmitted)
}

// Launch covers the first run and the relaunch after completion alike: it
// must refuse before any terminal is built.
func TestLaunchScenario_ExamLocked_Returns409BeforeCreatingTerminal(t *testing.T) {
	db, userID, scenario := seedLaunchableScenario(t, "exam-locked-launch")
	addExamQuizStep(t, db, scenario.ID)
	seedExamRun(t, db, scenario.ID, userID, "completed", true, false)

	ttSrv, rec := newPersistenceTTBackend(t)
	configureTTServerForPersistence(t, ttSrv.URL)
	w := launchScenarioForTest(t, setupPersistenceRouter(t, db, userID), scenario.ID)

	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "exam_already_submitted", body["reason"])
	assert.Equal(t, 0, rec.calls, "no terminal may be created for a refused launch")
}

// Start-over is an abandon followed by a launch: the abandon is what would
// lose the run, so it is the one refused.
func TestAbandonSession_ExamSubmittedRun_Returns409AndKeepsTheRun(t *testing.T) {
	scenario := examScenario(t, freshTestDB(t))
	run := seedExamRun(t, sharedTestDB, scenario.ID, "test-user-123", "active", true, false)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/scenario-sessions/"+run.ID.String()+"/abandon", nil)
	setupTestRouter(sharedTestDB).ServeHTTP(w, req)

	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"reason":"exam_already_submitted"`)
	var stored models.ScenarioSession
	require.NoError(t, sharedTestDB.First(&stored, "id = ?", run.ID).Error)
	assert.Equal(t, "active", stored.Status)
}

func TestAbandonSession_ExamNotYetSubmitted_IsAllowed(t *testing.T) {
	scenario := examScenario(t, freshTestDB(t))
	run := seedExamRun(t, sharedTestDB, scenario.ID, "test-user-123", "active", false, false)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/scenario-sessions/"+run.ID.String()+"/abandon", nil)
	setupTestRouter(sharedTestDB).ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestGetMySessions_ExamLocked_FlagsTheRun(t *testing.T) {
	scenario := examScenario(t, freshTestDB(t))
	run := seedExamRun(t, sharedTestDB, scenario.ID, withheldLearner, "completed", true, false)
	assert.True(t, mySession(t, run.ID).ExamLocked)
}

// --- teacher side -------------------------------------------------------

type lockedClass struct {
	scenario models.Scenario
	group    groupModels.ClassGroup
	locked   models.ScenarioSession
}

// lockedClassFixture is a class with one learner locked on an exam and one
// who has not started.
func lockedClassFixture(t *testing.T) lockedClass {
	t.Helper()
	db := freshTestDB(t)
	scenario := examScenario(t, db)
	group := createClassGroup(t, db, "exam-class-"+uuid.NewString(), "teacher-1", nil)
	addGroupMember(t, db, group.ID, "locked-learner", groupModels.GroupMemberRoleMember)
	addGroupMember(t, db, group.ID, "fresh-learner", groupModels.GroupMemberRoleMember)
	createScenarioAssignment(t, db, scenario.ID, &group.ID, nil, "group")
	locked := seedExamRun(t, db, scenario.ID, "locked-learner", "active", true, false)
	return lockedClass{scenario: scenario, group: group, locked: locked}
}

func dashboardWithTT() *services.TeacherDashboardService {
	ttMock := newMockTTService()
	ttMock.addKey("locked-learner")
	ttMock.addKey("fresh-learner")
	sessionSvc := services.NewScenarioSessionService(sharedTestDB, &mockFlagService{}, &mockVerificationService{})
	return services.NewTeacherDashboardService(sharedTestDB, ttMock, sessionSvc)
}

func assertRunUntouched(t *testing.T, run models.ScenarioSession) {
	t.Helper()
	var stored models.ScenarioSession
	require.NoError(t, sharedTestDB.First(&stored, "id = ?", run.ID).Error)
	assert.Equal(t, run.Status, stored.Status, "the locked learner's run must not be touched")
	var runs int64
	require.NoError(t, sharedTestDB.Model(&models.ScenarioSession{}).
		Where("user_id = ? AND scenario_id = ?", run.UserID, run.ScenarioID).Count(&runs).Error)
	assert.Equal(t, int64(1), runs, "no new run may be started for the locked learner")
}

func TestBulkStartScenario_ExamLockedLearner_IsSkippedAndUntouched(t *testing.T) {
	class := lockedClassFixture(t)

	result, err := dashboardWithTT().BulkStartScenario(class.group.ID, class.scenario.ID, services.ScenarioProvisioning{}, 0, "")
	require.NoError(t, err)

	assert.Equal(t, 1, result.Created, "the other learner still gets their run")
	assert.Equal(t, 1, result.Skipped)
	require.Len(t, result.SkippedUsers, 1)
	assert.Equal(t, "locked-learner", result.SkippedUsers[0].UserID)
	assert.Equal(t, "exam_already_submitted", result.SkippedUsers[0].Reason)
	assertRunUntouched(t, class.locked)
}

func TestResetGroupScenarioSessions_ExamLockedLearner_IsSkippedAndUntouched(t *testing.T) {
	class := lockedClassFixture(t)
	fresh := seedExamRun(t, sharedTestDB, class.scenario.ID, "fresh-learner", "active", false, false)

	abandoned, skipped, err := dashboardWithTT().ResetGroupScenarioSessions(class.group.ID, class.scenario.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), abandoned)
	require.Len(t, skipped, 1)
	assert.Equal(t, "locked-learner", skipped[0].UserID)
	assertRunUntouched(t, class.locked)

	var freshStored models.ScenarioSession
	require.NoError(t, sharedTestDB.First(&freshStored, "id = ?", fresh.ID).Error)
	assert.Equal(t, "abandoned", freshStored.Status)
}

func TestAllowRetake_UnlocksOnceAndASecondSubmissionLocksAgain(t *testing.T) {
	class := lockedClassFixture(t)
	dashboard := dashboardWithTT()

	retakeAt, err := dashboard.AllowRetake(class.group.ID, class.locked.ID)
	require.NoError(t, err)
	require.NotNil(t, retakeAt)
	assert.False(t, examLocked(t, class.scenario.ID, "locked-learner"))

	var history models.ScenarioSession
	require.NoError(t, sharedTestDB.First(&history, "id = ?", class.locked.ID).Error)
	require.NotNil(t, history.RetakeAllowedAt, "the old run stays, marked as released")

	_, err = dashboard.AllowRetake(class.group.ID, class.locked.ID)
	assert.ErrorIs(t, err, services.ErrExamNotLocked, "a learner who is not locked has nothing to release")

	seedExamRun(t, sharedTestDB, class.scenario.ID, "locked-learner", "completed", true, false)
	assert.True(t, examLocked(t, class.scenario.ID, "locked-learner"), "the retake is one more attempt, not an open door")
}

func TestScenarioResults_RowsCarryExamLockAndRetake(t *testing.T) {
	class := lockedClassFixture(t)
	dashboard := dashboardWithTT()

	results, err := dashboard.GetScenarioResults(class.group.ID, class.scenario.ID, nil, nil)
	require.NoError(t, err)
	require.Len(t, results.Items, 1)
	assert.True(t, results.Items[0].ExamLocked)
	assert.Nil(t, results.Items[0].RetakeAllowedAt)

	_, err = dashboard.AllowRetake(class.group.ID, class.locked.ID)
	require.NoError(t, err)
	results, err = dashboard.GetScenarioResults(class.group.ID, class.scenario.ID, nil, nil)
	require.NoError(t, err)
	assert.False(t, results.Items[0].ExamLocked)
	assert.NotNil(t, results.Items[0].RetakeAllowedAt)
}

func allowRetakeRequest(router http.Handler, groupID, sessionID uuid.UUID) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/teacher/groups/"+groupID.String()+"/sessions/"+sessionID.String()+"/allow-retake", nil)
	router.ServeHTTP(w, req)
	return w
}

func TestAllowRetakeRoute_Learner_IsForbidden(t *testing.T) {
	class := lockedClassFixture(t)
	router := setupRealTeacherRouter(t, sharedTestDB, "locked-learner", []string{"member"})
	assert.Equal(t, http.StatusForbidden, allowRetakeRequest(router, class.group.ID, class.locked.ID).Code)
	assert.True(t, examLocked(t, class.scenario.ID, "locked-learner"))
}

func TestAllowRetakeRoute_ClassManager_ReleasesThen409s(t *testing.T) {
	class := lockedClassFixture(t)
	addGroupMember(t, sharedTestDB, class.group.ID, "manager-1", groupModels.GroupMemberRoleManager)
	router := setupRealTeacherRouter(t, sharedTestDB, "manager-1", []string{"member"})

	w := allowRetakeRequest(router, class.group.ID, class.locked.ID)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	retakeAt, ok := body["retake_allowed_at"].(string)
	require.True(t, ok, "retake_allowed_at must be a timestamp, got %v", body["retake_allowed_at"])
	_, err := time.Parse(time.RFC3339, retakeAt)
	assert.NoError(t, err)

	w = allowRetakeRequest(router, class.group.ID, class.locked.ID)
	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), `"reason":"exam_not_locked"`)
}

func TestAllowRetakeRoute_SessionOutsideTheGroup_Returns404(t *testing.T) {
	class := lockedClassFixture(t)
	addGroupMember(t, sharedTestDB, class.group.ID, "manager-1", groupModels.GroupMemberRoleManager)
	outsider := seedExamRun(t, sharedTestDB, class.scenario.ID, "outsider", "completed", true, false)
	router := setupRealTeacherRouter(t, sharedTestDB, "manager-1", []string{"member"})

	assert.Equal(t, http.StatusNotFound, allowRetakeRequest(router, class.group.ID, outsider.ID).Code)
	assert.Equal(t, http.StatusNotFound, allowRetakeRequest(router, class.group.ID, uuid.New()).Code)
	assert.True(t, examLocked(t, class.scenario.ID, "outsider"))
}

// The generic session create would otherwise be a launch with no checks.
func TestScenarioSessionExamLockHook_LockedLearner_RefusesCreate(t *testing.T) {
	scenario := examScenario(t, freshTestDB(t))
	seedExamRun(t, sharedTestDB, scenario.ID, "learner-a", "completed", true, false)
	hook := scenarioHooks.NewScenarioSessionExamLockHook(sharedTestDB)

	locked := &hooks.HookContext{HookType: hooks.BeforeCreate,
		NewEntity: &models.ScenarioSession{ScenarioID: scenario.ID, UserID: "learner-a"}}
	assert.Error(t, hook.Execute(locked))

	free := &hooks.HookContext{HookType: hooks.BeforeCreate,
		NewEntity: &models.ScenarioSession{ScenarioID: scenario.ID, UserID: "learner-b"}}
	assert.NoError(t, hook.Execute(free))
}
