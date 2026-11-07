package scenarios_test

// Exam grades are withheld from the learner until the teacher publishes the
// class's results: an exam quiz that hides its score on submit must not leak
// it through the learner's session list or session info instead.

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
	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

const withheldLearner = "learner-withheld"

type gradedRun struct {
	scenario models.Scenario
	group    groupModels.ClassGroup
	session  models.ScenarioSession
}

// gradedRunFixture is a completed, graded run of a scenario assigned to the
// learner's class. examQuiz makes its quiz step exam mode.
func gradedRunFixture(t *testing.T, db *gorm.DB, examQuiz, preview bool) gradedRun {
	t.Helper()
	scenario := models.Scenario{Name: "graded-" + uuid.NewString(), Title: "Graded", InstanceType: "ubuntu:22.04", CreatedByID: "teacher-1"}
	require.NoError(t, db.Create(&scenario).Error)
	require.NoError(t, db.Create(&models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 0, Title: "Quiz", StepType: "quiz", ShowImmediateFeedback: !examQuiz,
	}).Error)

	group := newGroupWithMember(t, db, "class-"+uuid.NewString(), withheldLearner)
	createScenarioAssignment(t, db, scenario.ID, &group.ID, nil, "group")

	grade := 80.0
	now := time.Now()
	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: withheldLearner, Status: "completed",
		StartedAt: now, CompletedAt: &now, Grade: &grade, IsPreview: preview,
	}
	require.NoError(t, db.Create(&session).Error)
	return gradedRun{scenario: scenario, group: group, session: session}
}

func mySession(t *testing.T, sessionID uuid.UUID) dto.MySessionResponse {
	t.Helper()
	svc := services.NewScenarioSessionService(sharedTestDB, &mockFlagService{}, &mockVerificationService{})
	sessions, err := svc.GetMySessions(withheldLearner)
	require.NoError(t, err)
	for _, s := range sessions {
		if s.ID == sessionID {
			return s
		}
	}
	t.Fatalf("session %s not in the learner's list", sessionID)
	return dto.MySessionResponse{}
}

func assertGradeWithheld(t *testing.T, s dto.MySessionResponse) {
	t.Helper()
	assert.Nil(t, s.Grade, "the grade must not reach the learner")
	assert.True(t, s.GradeWithheld)
}

func assertGradeVisible(t *testing.T, s dto.MySessionResponse) {
	t.Helper()
	require.NotNil(t, s.Grade)
	assert.InDelta(t, 80.0, *s.Grade, 1e-9)
	assert.False(t, s.GradeWithheld)
}

func TestGradeWithheld_ExamQuizBeforePublish_HidesGradeFromLearner(t *testing.T) {
	run := gradedRunFixture(t, freshTestDB(t), true, false)
	assertGradeWithheld(t, mySession(t, run.session.ID))
	assert.True(t, services.GradeWithheldForLearner(sharedTestDB, &run.session))

	var stored models.ScenarioSession
	require.NoError(t, sharedTestDB.First(&stored, "id = ?", run.session.ID).Error)
	require.NotNil(t, stored.Grade, "withholding never touches the stored grade")
	assert.InDelta(t, 80.0, *stored.Grade, 1e-9)
}

func TestGradeWithheld_AfterPublish_ShowsGradeAndUnpublishHidesItAgain(t *testing.T) {
	run := gradedRunFixture(t, freshTestDB(t), true, false)
	dashboard := services.NewTeacherDashboardService(sharedTestDB, nil, nil)

	publishedAt, err := dashboard.PublishScenarioResults(run.group.ID, run.scenario.ID)
	require.NoError(t, err)
	require.NotNil(t, publishedAt)
	assertGradeVisible(t, mySession(t, run.session.ID))

	require.NoError(t, dashboard.UnpublishScenarioResults(run.group.ID, run.scenario.ID))
	assertGradeWithheld(t, mySession(t, run.session.ID))
}

func TestGradeWithheld_PublishedForAnotherClass_StaysHidden(t *testing.T) {
	run := gradedRunFixture(t, freshTestDB(t), true, false)
	other := createClassGroup(t, sharedTestDB, "other-class", "teacher-1", nil)
	createScenarioAssignment(t, sharedTestDB, run.scenario.ID, &other.ID, nil, "group")

	_, err := services.NewTeacherDashboardService(sharedTestDB, nil, nil).PublishScenarioResults(other.ID, run.scenario.ID)
	require.NoError(t, err)
	assertGradeWithheld(t, mySession(t, run.session.ID))
}

func TestGradeWithheld_NoExamQuiz_ShowsGrade(t *testing.T) {
	run := gradedRunFixture(t, freshTestDB(t), false, false)
	assertGradeVisible(t, mySession(t, run.session.ID))
}

func TestGradeWithheld_PreviewSession_IsNeverWithheld(t *testing.T) {
	run := gradedRunFixture(t, freshTestDB(t), true, true)
	assertGradeVisible(t, mySession(t, run.session.ID))
	assert.False(t, services.GradeWithheldForLearner(sharedTestDB, &run.session))
}

func TestGradeWithheld_TeacherSessionDetail_IsNeverRedacted(t *testing.T) {
	run := gradedRunFixture(t, freshTestDB(t), true, false)
	detail, err := services.NewTeacherDashboardService(sharedTestDB, nil, nil).GetSessionDetail(run.group.ID, run.session.ID)
	require.NoError(t, err)
	require.NotNil(t, detail.Grade)
	assert.InDelta(t, 80.0, *detail.Grade, 1e-9)
}

func TestPublishScenarioResults_ScenarioNotAssignedToGroup_ReturnsNotAssigned(t *testing.T) {
	run := gradedRunFixture(t, freshTestDB(t), true, false)
	stranger := createClassGroup(t, sharedTestDB, "stranger-class", "teacher-1", nil)
	dashboard := services.NewTeacherDashboardService(sharedTestDB, nil, nil)

	_, err := dashboard.PublishScenarioResults(stranger.ID, run.scenario.ID)
	assert.ErrorIs(t, err, services.ErrScenarioNotAssignedToGroup)
	assert.ErrorIs(t, dashboard.UnpublishScenarioResults(stranger.ID, run.scenario.ID), services.ErrScenarioNotAssignedToGroup)
}

func publishRequest(t *testing.T, router http.Handler, method string, groupID, scenarioID uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(method, "/api/v1/teacher/groups/"+groupID.String()+"/scenarios/"+scenarioID.String()+"/results/publish", nil)
	router.ServeHTTP(w, req)
	return w
}

func TestPublishResultsRoute_Learner_IsForbidden(t *testing.T) {
	db := freshTestDB(t)
	run := gradedRunFixture(t, db, true, false)
	router := setupRealTeacherRouter(t, db, withheldLearner, []string{"member"})

	assert.Equal(t, http.StatusForbidden, publishRequest(t, router, http.MethodPost, run.group.ID, run.scenario.ID).Code)
	assertGradeWithheld(t, mySession(t, run.session.ID))
}

func TestPublishResultsRoute_ClassManager_PublishesThenUnpublishes(t *testing.T) {
	db := freshTestDB(t)
	run := gradedRunFixture(t, db, true, false)
	addGroupMember(t, db, run.group.ID, "manager-1", groupModels.GroupMemberRoleManager)
	router := setupRealTeacherRouter(t, db, "manager-1", []string{"member"})

	w := publishRequest(t, router, http.MethodPost, run.group.ID, run.scenario.ID)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	publishedAt, ok := body["results_published_at"].(string)
	require.True(t, ok, "results_published_at must be an RFC3339 timestamp, got %v", body["results_published_at"])
	_, err := time.Parse(time.RFC3339, publishedAt)
	assert.NoError(t, err)
	assertGradeVisible(t, mySession(t, run.session.ID))

	w = publishRequest(t, router, http.MethodDelete, run.group.ID, run.scenario.ID)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Contains(t, body, "results_published_at")
	assert.Nil(t, body["results_published_at"])
	assertGradeWithheld(t, mySession(t, run.session.ID))
}

func TestPublishResultsRoute_ScenarioNotAssigned_Returns404(t *testing.T) {
	db := freshTestDB(t)
	run := gradedRunFixture(t, db, true, false)
	addGroupMember(t, db, run.group.ID, "manager-1", groupModels.GroupMemberRoleManager)
	router := setupRealTeacherRouter(t, db, "manager-1", []string{"member"})

	assert.Equal(t, http.StatusNotFound, publishRequest(t, router, http.MethodPost, run.group.ID, uuid.New()).Code)
	assert.Equal(t, http.StatusNotFound, publishRequest(t, router, http.MethodDelete, run.group.ID, uuid.New()).Code)
}

// The player's session info is the other learner path to the grade.
func TestGetSessionInfo_ExamGradeUnpublished_IsWithheld(t *testing.T) {
	db := freshTestDB(t)
	run := gradedRunFixture(t, db, true, false)
	require.NoError(t, db.Model(&run.session).Update("user_id", "test-user-123").Error)
	addGroupMember(t, db, run.group.ID, "test-user-123", groupModels.GroupMemberRoleMember)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/scenario-sessions/"+run.session.ID.String()+"/info", nil)
	setupTestRouter(db).ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Nil(t, body["grade"])
	assert.Equal(t, true, body["grade_withheld"])
}

// The publish button is offered only where grades actually wait for it.
func TestTeacherAssignmentViews_FlagExamScenarios(t *testing.T) {
	db := freshTestDB(t)
	exam := gradedRunFixture(t, db, true, false)
	dashboard := services.NewTeacherDashboardService(sharedTestDB, nil, nil)

	progress, err := dashboard.GetGroupAssignmentsProgress(exam.group.ID)
	require.NoError(t, err)
	require.Len(t, progress, 1)
	assert.True(t, progress[0].HasExamQuiz)

	learning := gradedRunFixture(t, db, false, false)
	progress, err = dashboard.GetGroupAssignmentsProgress(learning.group.ID)
	require.NoError(t, err)
	flags := map[uuid.UUID]bool{}
	for _, p := range progress {
		flags[p.ScenarioID] = p.HasExamQuiz
	}
	assert.False(t, flags[learning.scenario.ID])
	assert.True(t, flags[exam.scenario.ID], "the same learner's exam scenario keeps its flag")
}

// With no class of the learner assigning the scenario there is no teacher to
// publish, so withholding would hide the grade forever.
func TestGradeWithheld_NoClassAssignsTheScenario_ShowsGrade(t *testing.T) {
	db := freshTestDB(t)
	run := gradedRunFixture(t, db, true, false)
	require.NoError(t, db.Where("scenario_id = ?", run.scenario.ID).Delete(&models.ScenarioAssignment{}).Error)
	orgID := uuid.New()
	createScenarioAssignment(t, db, run.scenario.ID, nil, &orgID, "org")

	assertGradeVisible(t, mySession(t, run.session.ID))
}

func TestGradeWithheld_ClassMembershipInactive_ShowsGrade(t *testing.T) {
	db := freshTestDB(t)
	run := gradedRunFixture(t, db, true, false)
	require.NoError(t, db.Model(&groupModels.GroupMember{}).
		Where("group_id = ? AND user_id = ?", run.group.ID, withheldLearner).Update("is_active", false).Error)

	assertGradeVisible(t, mySession(t, run.session.ID))
}
