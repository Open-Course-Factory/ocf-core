package scenarios_test

// A class teacher can lock a scenario at a step: learners of the class who
// finish the step before it wait there, and continue once the teacher releases
// the gate. The teacher can also validate a learner's current step by hand.

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

	groupModels "soli/formations/src/groups/models"
	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"
	scenarioController "soli/formations/src/scenarios/routes"
	"soli/formations/src/scenarios/services"
)

const gatedLearner = "learner-gated"

type gatedRun struct {
	scenario   models.Scenario
	group      groupModels.ClassGroup
	assignment *models.ScenarioAssignment
	session    models.ScenarioSession
}

// gatedRunFixture is an active run, on its first step, of a scenario whose
// steps have the given types (orders 0..n-1), assigned to the learner's class.
func gatedRunFixture(t *testing.T, db *gorm.DB, stepTypes ...string) gatedRun {
	t.Helper()
	return gatedRunFixtureFrom(t, db, 0, stepTypes...)
}

// gatedRunFixtureFrom is gatedRunFixture with step orders starting at
// firstOrder: imported scenarios start at 0, editor-made ones at 1.
func gatedRunFixtureFrom(t *testing.T, db *gorm.DB, firstOrder int, stepTypes ...string) gatedRun {
	t.Helper()
	scenario := models.Scenario{Name: "gated-" + uuid.NewString(), Title: "Gated", InstanceType: "ubuntu:22.04", CreatedByID: "teacher-1"}
	require.NoError(t, db.Create(&scenario).Error)
	for i, stepType := range stepTypes {
		step := models.ScenarioStep{ScenarioID: scenario.ID, Order: firstOrder + i, Title: "Step", StepType: stepType, ShowImmediateFeedback: true}
		require.NoError(t, db.Create(&step).Error)
		if stepType == models.StepTypeQuiz {
			require.NoError(t, db.Create(&models.ScenarioStepQuestion{
				StepID: step.ID, Order: 1, QuestionText: "Root home?", QuestionType: "free_text", CorrectAnswer: "/root", Points: 1,
			}).Error)
		}
	}

	group := newGroupWithMember(t, db, "class-"+uuid.NewString(), gatedLearner)
	assignment := createScenarioAssignment(t, db, scenario.ID, &group.ID, nil, "group")

	terminalID := "terminal-" + uuid.NewString()
	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: gatedLearner, Status: "active", CurrentStep: firstOrder,
		StartedAt: time.Now(), TerminalSessionID: &terminalID,
	}
	require.NoError(t, db.Create(&session).Error)
	for i := range stepTypes {
		status := "locked"
		if i == 0 {
			status = "active"
		}
		require.NoError(t, db.Create(&models.ScenarioStepProgress{SessionID: session.ID, StepOrder: firstOrder + i, Status: status}).Error)
	}
	return gatedRun{scenario: scenario, group: group, assignment: assignment, session: session}
}

func setAssignmentGate(t *testing.T, db *gorm.DB, assignmentID uuid.UUID, gate *int) {
	t.Helper()
	require.NoError(t, db.Model(&models.ScenarioAssignment{}).Where("id = ?", assignmentID).Update("step_gate", gate).Error)
}

func gateAt(order int) *int { return &order }

func gateSessionService(db *gorm.DB) *services.ScenarioSessionService {
	return services.NewScenarioSessionService(db, &mockFlagService{}, &mockVerificationService{passed: true})
}

// --- the gate in advanceToNextStep ------------------------------------------

func TestStepGate_NextStepBeforeGate_Advances(t *testing.T) {
	db := freshTestDB(t)
	run := gatedRunFixture(t, db, "terminal", "terminal", "terminal")
	setAssignmentGate(t, db, run.assignment.ID, gateAt(2))

	resp, err := gateSessionService(db).VerifyCurrentStep(run.session.ID)
	require.NoError(t, err)
	require.NotNil(t, resp.NextStep)
	assert.Equal(t, 1, *resp.NextStep)
	assert.False(t, resp.WaitingAtGate)
	assert.Equal(t, 1, reloadSession(t, run.session.ID).CurrentStep)
}

func TestStepGate_NextStepAtGate_CompletesStepButHolds(t *testing.T) {
	db := freshTestDB(t)
	run := gatedRunFixture(t, db, "terminal", "terminal", "terminal")
	setAssignmentGate(t, db, run.assignment.ID, gateAt(1))

	resp, err := gateSessionService(db).VerifyCurrentStep(run.session.ID)
	require.NoError(t, err)
	assert.True(t, resp.Passed)
	assert.Nil(t, resp.NextStep)
	assert.True(t, resp.WaitingAtGate)

	stored := reloadSession(t, run.session.ID)
	assert.Equal(t, 0, stored.CurrentStep, "a held learner stays on the step they finished")
	assert.Equal(t, "active", stored.Status)
	statuses := stepStatuses(t, db, &run.session)
	assert.Equal(t, map[int]string{0: "completed", 1: "locked", 2: "locked"}, statuses)
}

func TestStepGate_LastStep_IsNeverHeld(t *testing.T) {
	db := freshTestDB(t)
	run := gatedRunFixture(t, db, "terminal", "terminal")
	require.NoError(t, db.Model(&models.ScenarioSession{}).Where("id = ?", run.session.ID).Update("current_step", 1).Error)
	require.NoError(t, db.Model(&models.ScenarioStepProgress{}).Where("session_id = ? AND step_order = ?", run.session.ID, 1).Update("status", "active").Error)
	setAssignmentGate(t, db, run.assignment.ID, gateAt(0))

	resp, err := gateSessionService(db).VerifyCurrentStep(run.session.ID)
	require.NoError(t, err)
	assert.False(t, resp.WaitingAtGate)
	stored := reloadSession(t, run.session.ID)
	assert.Equal(t, "completed", stored.Status, "finishing the scenario is not gated")
	assert.NotNil(t, stored.Grade)
}

func TestStepGate_PreviewRun_IsNeverHeld(t *testing.T) {
	db := freshTestDB(t)
	run := gatedRunFixture(t, db, "terminal", "terminal")
	require.NoError(t, db.Model(&models.ScenarioSession{}).Where("id = ?", run.session.ID).Update("is_preview", true).Error)
	setAssignmentGate(t, db, run.assignment.ID, gateAt(1))

	resp, err := gateSessionService(db).VerifyCurrentStep(run.session.ID)
	require.NoError(t, err)
	assert.False(t, resp.WaitingAtGate)
	assert.Equal(t, 1, reloadSession(t, run.session.ID).CurrentStep)
}

func TestStepGate_InactiveClassMembership_IsNotHeld(t *testing.T) {
	db := freshTestDB(t)
	run := gatedRunFixture(t, db, "terminal", "terminal")
	setAssignmentGate(t, db, run.assignment.ID, gateAt(1))
	require.NoError(t, db.Model(&groupModels.GroupMember{}).
		Where("group_id = ? AND user_id = ?", run.group.ID, gatedLearner).Update("is_active", false).Error)

	resp, err := gateSessionService(db).VerifyCurrentStep(run.session.ID)
	require.NoError(t, err)
	assert.False(t, resp.WaitingAtGate)
	assert.Equal(t, 1, reloadSession(t, run.session.ID).CurrentStep)
}

func TestStepGate_TwoClasses_StrictestGateHolds(t *testing.T) {
	db := freshTestDB(t)
	run := gatedRunFixture(t, db, "terminal", "terminal", "terminal")
	setAssignmentGate(t, db, run.assignment.ID, gateAt(2))
	strict := newGroupWithMember(t, db, "strict-"+uuid.NewString(), gatedLearner)
	strictAssignment := createScenarioAssignment(t, db, run.scenario.ID, &strict.ID, nil, "group")
	setAssignmentGate(t, db, strictAssignment.ID, gateAt(1))

	resp, err := gateSessionService(db).VerifyCurrentStep(run.session.ID)
	require.NoError(t, err)
	assert.True(t, resp.WaitingAtGate)
	assert.Equal(t, 0, reloadSession(t, run.session.ID).CurrentStep)
}

func TestStepGate_InfoStep_Holds(t *testing.T) {
	db := freshTestDB(t)
	run := gatedRunFixture(t, db, "info", "terminal")
	setAssignmentGate(t, db, run.assignment.ID, gateAt(1))

	resp, err := gateSessionService(db).VerifyCurrentStep(run.session.ID)
	require.NoError(t, err)
	assert.True(t, resp.WaitingAtGate)
	assert.Nil(t, resp.NextStep)
}

func TestStepGate_QuizSubmit_Holds(t *testing.T) {
	db := freshTestDB(t)
	run := gatedRunFixture(t, db, models.StepTypeQuiz, "terminal")
	setAssignmentGate(t, db, run.assignment.ID, gateAt(1))

	var question models.ScenarioStepQuestion
	require.NoError(t, db.Joins("JOIN scenario_steps ON scenario_steps.id = scenario_step_questions.step_id").
		Where("scenario_steps.scenario_id = ?", run.scenario.ID).First(&question).Error)
	resp, err := gateSessionService(db).SubmitQuiz(run.session.ID, dto.SubmitQuizInput{Answers: map[uuid.UUID]string{question.ID: "/root"}})
	require.NoError(t, err)
	assert.True(t, resp.WaitingAtGate)
	assert.Nil(t, resp.NextStep)
	assert.Equal(t, 0, reloadSession(t, run.session.ID).CurrentStep)

	_, err = gateSessionService(db).SubmitQuiz(run.session.ID, dto.SubmitQuizInput{Answers: map[uuid.UUID]string{question.ID: "/tmp"}})
	assert.ErrorIs(t, err, services.ErrSessionNotActive, "a held learner cannot resubmit the quiz they finished")
	var progress models.ScenarioStepProgress
	require.NoError(t, db.First(&progress, "session_id = ? AND step_order = ?", run.session.ID, 0).Error)
	require.NotNil(t, progress.QuizScore)
	assert.InDelta(t, 1.0, *progress.QuizScore, 1e-9)
}

func TestStepGate_FlagSubmit_Holds(t *testing.T) {
	db := freshTestDB(t)
	run := gatedRunFixture(t, db, "flag", "terminal")
	setAssignmentGate(t, db, run.assignment.ID, gateAt(1))
	require.NoError(t, db.Create(&models.ScenarioFlag{SessionID: run.session.ID, StepOrder: 0, ExpectedFlag: "FLAG{ok}"}).Error)

	svc := services.NewScenarioSessionService(db, &mockFlagService{validateRes: true}, &mockVerificationService{})
	resp, err := svc.SubmitFlag(run.session.ID, "FLAG{ok}")
	require.NoError(t, err)
	assert.True(t, resp.Correct)
	assert.True(t, resp.WaitingAtGate)
	assert.Nil(t, resp.NextStep)
}

func TestStepGate_VerifyWhileHeld_IsRefused(t *testing.T) {
	db := freshTestDB(t)
	run := gatedRunFixture(t, db, "terminal", "terminal")
	setAssignmentGate(t, db, run.assignment.ID, gateAt(1))
	svc := gateSessionService(db)
	_, err := svc.VerifyCurrentStep(run.session.ID)
	require.NoError(t, err)

	_, err = svc.VerifyCurrentStep(run.session.ID)
	assert.ErrorIs(t, err, services.ErrSessionNotActive)
}

// --- current step and continue ---------------------------------------------

func TestStepGate_CurrentStep_ReportsWaitingUntilReleased(t *testing.T) {
	db := freshTestDB(t)
	run := gatedRunFixture(t, db, "terminal", "terminal")
	svc := gateSessionService(db)

	current, err := svc.GetCurrentStep(run.session.ID)
	require.NoError(t, err)
	assert.False(t, current.WaitingAtGate, "an open step is not waiting")

	setAssignmentGate(t, db, run.assignment.ID, gateAt(1))
	_, err = svc.VerifyCurrentStep(run.session.ID)
	require.NoError(t, err)
	current, err = svc.GetCurrentStep(run.session.ID)
	require.NoError(t, err)
	assert.Equal(t, "completed", current.Status)
	assert.True(t, current.WaitingAtGate)

	setAssignmentGate(t, db, run.assignment.ID, nil)
	current, err = svc.GetCurrentStep(run.session.ID)
	require.NoError(t, err)
	assert.False(t, current.WaitingAtGate, "released: the next continue moves on")
}

func TestContinueAfterGate_StillHeld_ChangesNothing(t *testing.T) {
	db := freshTestDB(t)
	run := gatedRunFixture(t, db, "terminal", "terminal")
	setAssignmentGate(t, db, run.assignment.ID, gateAt(1))
	svc := gateSessionService(db)
	_, err := svc.VerifyCurrentStep(run.session.ID)
	require.NoError(t, err)

	resp, err := svc.ContinueAfterGate(run.session.ID)
	require.NoError(t, err)
	assert.True(t, resp.WaitingAtGate)
	assert.Nil(t, resp.NextStep)
	assert.Equal(t, 0, reloadSession(t, run.session.ID).CurrentStep)
}

func TestContinueAfterGate_Released_AdvancesOnce(t *testing.T) {
	db := freshTestDB(t)
	run := gatedRunFixture(t, db, "terminal", "terminal", "terminal")
	setAssignmentGate(t, db, run.assignment.ID, gateAt(1))
	svc := gateSessionService(db)
	_, err := svc.VerifyCurrentStep(run.session.ID)
	require.NoError(t, err)

	setAssignmentGate(t, db, run.assignment.ID, gateAt(2))
	resp, err := svc.ContinueAfterGate(run.session.ID)
	require.NoError(t, err)
	assert.False(t, resp.WaitingAtGate)
	require.NotNil(t, resp.NextStep)
	assert.Equal(t, 1, *resp.NextStep)
	assert.Equal(t, 1, reloadSession(t, run.session.ID).CurrentStep)
	assert.Equal(t, map[int]string{0: "completed", 1: "active", 2: "locked"}, stepStatuses(t, db, &run.session))

	_, err = svc.ContinueAfterGate(run.session.ID)
	assert.ErrorIs(t, err, services.ErrStepNotCompleted, "a second call must not advance again")
	assert.Equal(t, 1, reloadSession(t, run.session.ID).CurrentStep)
}

func TestContinueAfterGate_StepNotCompleted_IsRefused(t *testing.T) {
	db := freshTestDB(t)
	run := gatedRunFixture(t, db, "terminal", "terminal")

	_, err := gateSessionService(db).ContinueAfterGate(run.session.ID)
	assert.ErrorIs(t, err, services.ErrStepNotCompleted)
	assert.Equal(t, 0, reloadSession(t, run.session.ID).CurrentStep)
}

func continueRequest(t *testing.T, db *gorm.DB, userID string, sessionID uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	api.Use(func(c *gin.Context) { c.Set("userId", userID); c.Next() })
	api.POST("/scenario-sessions/:id/continue", scenarioController.NewScenarioProgressController(db).ContinueAfterGate)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/scenario-sessions/"+sessionID.String()+"/continue", nil)
	r.ServeHTTP(w, req)
	return w
}

func TestContinueRoute_MapsStatesToStatusCodes(t *testing.T) {
	db := freshTestDB(t)
	run := gatedRunFixture(t, db, "terminal", "terminal")

	assert.Equal(t, http.StatusConflict, continueRequest(t, db, gatedLearner, run.session.ID).Code, "step not completed")
	assert.Equal(t, http.StatusForbidden, continueRequest(t, db, "someone-else", run.session.ID).Code, "owner only")

	setAssignmentGate(t, db, run.assignment.ID, gateAt(1))
	_, err := gateSessionService(db).VerifyCurrentStep(run.session.ID)
	require.NoError(t, err)
	w := continueRequest(t, db, gatedLearner, run.session.ID)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, true, body["waiting_at_gate"])

	setAssignmentGate(t, db, run.assignment.ID, nil)
	w = continueRequest(t, db, gatedLearner, run.session.ID)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, false, body["waiting_at_gate"])
	assert.EqualValues(t, 1, body["next_step"])
}

// --- teacher gate route -----------------------------------------------------

func stepGateRequest(t *testing.T, router http.Handler, groupID, scenarioID uuid.UUID, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPut, "/api/v1/teacher/groups/"+groupID.String()+"/scenarios/"+scenarioID.String()+"/step-gate", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	return w
}

func storedGate(t *testing.T, db *gorm.DB, assignmentID uuid.UUID) *int {
	t.Helper()
	var a models.ScenarioAssignment
	require.NoError(t, db.First(&a, "id = ?", assignmentID).Error)
	return a.StepGate
}

func TestStepGateRoute_ClassManager_SetsThenClearsEveryAssignment(t *testing.T) {
	db := freshTestDB(t)
	run := gatedRunFixture(t, db, "terminal", "terminal", "terminal")
	second := createScenarioAssignment(t, db, run.scenario.ID, &run.group.ID, nil, "group")
	addGroupMember(t, db, run.group.ID, "manager-1", groupModels.GroupMemberRoleManager)
	router := setupRealTeacherRouter(t, db, "manager-1", []string{"member"})

	w := stepGateRequest(t, router, run.group.ID, run.scenario.ID, `{"step_number": 3}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, `{"step_gate": 3}`, w.Body.String())
	assert.Equal(t, gateAt(2), storedGate(t, db, run.assignment.ID), "step 3 of orders 0,1,2 is order 2")
	assert.Equal(t, gateAt(2), storedGate(t, db, second.ID))

	w = stepGateRequest(t, router, run.group.ID, run.scenario.ID, `{"step_number": null}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, `{"step_gate": null}`, w.Body.String())
	assert.Nil(t, storedGate(t, db, run.assignment.ID))
}

func TestStepGateRoute_StepNotInScenario_Returns400(t *testing.T) {
	db := freshTestDB(t)
	run := gatedRunFixture(t, db, "terminal", "terminal")
	addGroupMember(t, db, run.group.ID, "manager-1", groupModels.GroupMemberRoleManager)
	router := setupRealTeacherRouter(t, db, "manager-1", []string{"member"})

	for _, number := range []string{"0", "3", "-1"} {
		assert.Equal(t, http.StatusBadRequest, stepGateRequest(t, router, run.group.ID, run.scenario.ID, `{"step_number": `+number+`}`).Code, "step %s of 2", number)
	}
	assert.Nil(t, storedGate(t, db, run.assignment.ID))
}

func TestStepGateRoute_LearnerAndOtherClassManager_AreForbidden(t *testing.T) {
	db := freshTestDB(t)
	run := gatedRunFixture(t, db, "terminal", "terminal")
	other := createClassGroup(t, db, "other-class", "teacher-2", nil)
	addGroupMember(t, db, other.ID, "manager-2", groupModels.GroupMemberRoleManager)

	learner := setupRealTeacherRouter(t, db, gatedLearner, []string{"member"})
	assert.Equal(t, http.StatusForbidden, stepGateRequest(t, learner, run.group.ID, run.scenario.ID, `{"step_number": 1}`).Code)
	stranger := setupRealTeacherRouter(t, db, "manager-2", []string{"member"})
	assert.Equal(t, http.StatusForbidden, stepGateRequest(t, stranger, run.group.ID, run.scenario.ID, `{"step_number": 1}`).Code)
	assert.Nil(t, storedGate(t, db, run.assignment.ID))
}

func TestStepGate_IsReadOnlyOnTheAssignmentEntity(t *testing.T) {
	input := dto.EditScenarioAssignmentInput{}
	raw, err := json.Marshal(input)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "step_gate", "no PATCH may set the gate")
	raw, err = json.Marshal(dto.CreateScenarioAssignmentInput{})
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "step_gate", "no create may set the gate")
}

// --- teacher validate-step --------------------------------------------------

func validateStepRequest(t *testing.T, router http.Handler, groupID, sessionID uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/teacher/groups/"+groupID.String()+"/sessions/"+sessionID.String()+"/validate-step", nil)
	router.ServeHTTP(w, req)
	return w
}

func managerRouterFor(t *testing.T, db *gorm.DB, run gatedRun) http.Handler {
	t.Helper()
	addGroupMember(t, db, run.group.ID, "manager-1", groupModels.GroupMemberRoleManager)
	return setupRealTeacherRouter(t, db, "manager-1", []string{"member"})
}

func TestValidateStepRoute_PassesTheStepAndAdvances(t *testing.T) {
	db := freshTestDB(t)
	run := gatedRunFixture(t, db, "terminal", "terminal")
	require.NoError(t, db.Model(&models.ScenarioStep{}).Where("scenario_id = ?", run.scenario.ID).Update("verify_script", "exit 1").Error)

	w := validateStepRequest(t, managerRouterFor(t, db, run), run.group.ID, run.session.ID)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp dto.VerifyStepResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.Passed)
	require.NotNil(t, resp.NextStep)
	assert.Equal(t, 1, *resp.NextStep)
	assert.Equal(t, map[int]string{0: "completed", 1: "active"}, stepStatuses(t, db, &run.session))
}

func TestValidateStepRoute_GateApplies(t *testing.T) {
	db := freshTestDB(t)
	run := gatedRunFixture(t, db, "terminal", "terminal")
	setAssignmentGate(t, db, run.assignment.ID, gateAt(1))

	w := validateStepRequest(t, managerRouterFor(t, db, run), run.group.ID, run.session.ID)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, true, body["waiting_at_gate"])
	assert.Equal(t, 0, reloadSession(t, run.session.ID).CurrentStep)
	assert.Equal(t, "completed", stepStatuses(t, db, &run.session)[0])
}

func TestValidateStepRoute_LastStep_CompletesTheRunWithItsGrade(t *testing.T) {
	db := freshTestDB(t)
	run := gatedRunFixture(t, db, "terminal")

	w := validateStepRequest(t, managerRouterFor(t, db, run), run.group.ID, run.session.ID)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	stored := reloadSession(t, run.session.ID)
	assert.Equal(t, "completed", stored.Status)
	require.NotNil(t, stored.Grade)
	assert.InDelta(t, 100.0, *stored.Grade, 1e-9)
}

func TestValidateStepRoute_QuizStep_Returns400(t *testing.T) {
	db := freshTestDB(t)
	run := gatedRunFixture(t, db, models.StepTypeQuiz, "terminal")

	assert.Equal(t, http.StatusBadRequest, validateStepRequest(t, managerRouterFor(t, db, run), run.group.ID, run.session.ID).Code)
	assert.Equal(t, "active", stepStatuses(t, db, &run.session)[0])
}

func TestValidateStepRoute_SurveyStep_Passes(t *testing.T) {
	db := freshTestDB(t)
	run := gatedRunFixture(t, db, models.StepTypeSurvey, "terminal")

	assert.Equal(t, http.StatusOK, validateStepRequest(t, managerRouterFor(t, db, run), run.group.ID, run.session.ID).Code)
	assert.Equal(t, 1, reloadSession(t, run.session.ID).CurrentStep)
}

func TestValidateStepRoute_Learner_IsForbidden(t *testing.T) {
	db := freshTestDB(t)
	run := gatedRunFixture(t, db, "terminal", "terminal")
	router := setupRealTeacherRouter(t, db, gatedLearner, []string{"member"})

	assert.Equal(t, http.StatusForbidden, validateStepRequest(t, router, run.group.ID, run.session.ID).Code)
	assert.Equal(t, 0, reloadSession(t, run.session.ID).CurrentStep)
}

// --- live progress ----------------------------------------------------------

func TestGetGroupLiveProgress_ReportsGateAndWaitingLearners(t *testing.T) {
	db := freshTestDB(t)
	run := gatedRunFixture(t, db, "terminal", "terminal")
	setAssignmentGate(t, db, run.assignment.ID, gateAt(1))
	dashboard := services.NewTeacherDashboardService(db, nil, nil)

	rows, err := dashboard.GetGroupLiveProgress(run.group.ID)
	require.NoError(t, err)
	progress := liveProgressByUserID(rows)[gatedLearner].Assignments[0]
	assert.Equal(t, gateAt(2), progress.StepGate, "order 1 is step 2 when orders start at 0")
	assert.Equal(t, 1, progress.CurrentStepNumber)
	assert.False(t, progress.WaitingAtGate, "still working on the step")

	_, err = gateSessionService(db).VerifyCurrentStep(run.session.ID)
	require.NoError(t, err)
	rows, err = dashboard.GetGroupLiveProgress(run.group.ID)
	require.NoError(t, err)
	assert.True(t, liveProgressByUserID(rows)[gatedLearner].Assignments[0].WaitingAtGate)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/teacher/groups/"+run.group.ID.String()+"/live-progress", nil)
	managerRouterFor(t, db, run).ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body, 1)
	assignment := body[0]["assignments"].([]any)[0].(map[string]any)
	assert.EqualValues(t, 2, assignment["step_gate"])
	assert.EqualValues(t, 1, assignment["current_step_number"])
	assert.Equal(t, true, assignment["waiting_at_gate"])
}

// Editor-made scenarios number their orders from 1: step 2 is order 2 there.
func TestStepGateRoute_OrdersStartingAtOne_ConvertsBothWays(t *testing.T) {
	db := freshTestDB(t)
	run := gatedRunFixtureFrom(t, db, 1, "terminal", "terminal", "terminal")
	router := managerRouterFor(t, db, run)

	w := stepGateRequest(t, router, run.group.ID, run.scenario.ID, `{"step_number": 2}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, `{"step_gate": 2}`, w.Body.String())
	assert.Equal(t, gateAt(2), storedGate(t, db, run.assignment.ID))
	assert.Equal(t, http.StatusBadRequest, stepGateRequest(t, router, run.group.ID, run.scenario.ID, `{"step_number": 4}`).Code)

	_, err := gateSessionService(db).VerifyCurrentStep(run.session.ID)
	require.NoError(t, err)
	rows, err := services.NewTeacherDashboardService(db, nil, nil).GetGroupLiveProgress(run.group.ID)
	require.NoError(t, err)
	progress := liveProgressByUserID(rows)[gatedLearner].Assignments[0]
	assert.Equal(t, gateAt(2), progress.StepGate)
	assert.Equal(t, 1, progress.CurrentStepNumber, "order 1 is step 1 here")
	assert.Equal(t, 1, progress.CurrentStep, "the raw order is kept")
	assert.True(t, progress.WaitingAtGate)
}

func TestGetGroupLiveProgress_NotStarted_HasStepNumberZero(t *testing.T) {
	db := freshTestDB(t)
	run := gatedRunFixtureFrom(t, db, 1, "terminal", "terminal")
	require.NoError(t, db.Delete(&run.session).Error)

	rows, err := services.NewTeacherDashboardService(db, nil, nil).GetGroupLiveProgress(run.group.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, liveProgressByUserID(rows)[gatedLearner].Assignments[0].CurrentStepNumber)
}

// A teacher who deactivates an assignment no longer sees it in the class view,
// so its gate must not hold anyone.
func TestStepGate_InactiveAssignment_IsNotHeld(t *testing.T) {
	db := freshTestDB(t)
	run := gatedRunFixture(t, db, "terminal", "terminal")
	setAssignmentGate(t, db, run.assignment.ID, gateAt(1))
	require.NoError(t, db.Model(&models.ScenarioAssignment{}).Where("id = ?", run.assignment.ID).Update("is_active", false).Error)

	resp, err := gateSessionService(db).VerifyCurrentStep(run.session.ID)
	require.NoError(t, err)
	assert.False(t, resp.WaitingAtGate)
	assert.Equal(t, 1, reloadSession(t, run.session.ID).CurrentStep)
}
