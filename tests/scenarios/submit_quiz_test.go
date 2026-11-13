package scenarios_test

// Quiz answer submission (#283): scoring, per-question feedback, progress
// persistence, and input validation on ScenarioSessionService.SubmitQuiz.
//
// PerQuestionResults (incl. CorrectAnswer + Explanation) is only returned in
// learning mode (show_immediate_feedback=true). In exam mode the breakdown is
// omitted entirely: the flag is the teacher's control over whether answers
// ever reach the learner's browser, so it must gate the API response, not
// just the UI rendering (scenarios-e2e-test-plan.md §7.5, decided 2026-08-06).

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"
	scenarioController "soli/formations/src/scenarios/routes"
	"soli/formations/src/scenarios/services"
)

// quizFixture creates a quiz step with three questions and a primed session.
// Returns the session ID, the three question IDs (in order), and the database
// handle.
func quizFixture(t *testing.T, withFollowingStep bool) (uuid.UUID, []uuid.UUID) {
	t.Helper()
	db := freshTestDB(t)

	scenario := models.Scenario{
		Name: "quiz-submit", Title: "Quiz Submit", InstanceType: "ubuntu:22.04", CreatedByID: "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	step0 := models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 0, Title: "Quiz Step", StepType: "quiz",
		// Learning mode: the per-question breakdown (incl. correct answers)
		// is returned. Exam mode (false) omits it — see the ExamMode test.
		ShowImmediateFeedback: true,
	}
	require.NoError(t, db.Create(&step0).Error)

	if withFollowingStep {
		require.NoError(t, db.Create(&models.ScenarioStep{
			ScenarioID: scenario.ID, Order: 1, Title: "Next", StepType: "terminal",
		}).Error)
	}

	q1 := models.ScenarioStepQuestion{
		StepID: step0.ID, Order: 1, QuestionText: "What lists files?",
		QuestionType: "multiple_choice", Options: `["ls", "cd", "rm"]`,
		CorrectAnswer: "ls", Points: 1,
	}
	q2 := models.ScenarioStepQuestion{
		StepID: step0.ID, Order: 2, QuestionText: "/dev/null discards data?",
		QuestionType: "true_false", CorrectAnswer: "true", Points: 1,
	}
	q3 := models.ScenarioStepQuestion{
		StepID: step0.ID, Order: 3, QuestionText: "Root home?",
		QuestionType: "free_text", CorrectAnswer: "/root", Points: 1,
	}
	require.NoError(t, db.Create(&q1).Error)
	require.NoError(t, db.Create(&q2).Error)
	require.NoError(t, db.Create(&q3).Error)

	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "student-1", CurrentStep: 0,
		Status: "active", StartedAt: time.Now(),
	}
	require.NoError(t, db.Create(&session).Error)
	require.NoError(t, db.Create(&models.ScenarioStepProgress{
		SessionID: session.ID, StepOrder: 0, Status: "active",
	}).Error)
	if withFollowingStep {
		require.NoError(t, db.Create(&models.ScenarioStepProgress{
			SessionID: session.ID, StepOrder: 1, Status: "locked",
		}).Error)
	}

	return session.ID, []uuid.UUID{q1.ID, q2.ID, q3.ID}
}

// TestSubmitQuiz_AllCorrect_ReturnsFullScore — when every answer matches the
// correct_answer, the response must report score=1.0, correct_count==total,
// and a populated next_step.
func TestSubmitQuiz_AllCorrect_ReturnsFullScore(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	sessionID, qIDs := quizFixture(t, true /* withFollowingStep */)

	input := dto.SubmitQuizInput{
		Answers: map[uuid.UUID]string{
			qIDs[0]: "ls",
			qIDs[1]: "true",
			qIDs[2]: "/root",
		},
	}

	svc := services.NewScenarioSessionService(sharedTestDB, &mockFlagService{}, &mockVerificationService{})
	result, err := svc.SubmitQuiz(sessionID, input)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.InDelta(t, 1.0, *result.Score, 0.001, "all-correct quiz must score 1.0")
	assert.Equal(t, 3, *result.CorrectCount)
	assert.Equal(t, 3, result.Total)
	require.NotNil(t, result.NextStep, "all-correct quiz must advance to next step")
	assert.Equal(t, 1, *result.NextStep)
}

// TestSubmitQuiz_AllWrong_ReturnsZeroScore — when no answer matches, score=0
// but the session still advances (quiz is graded, not gated).
func TestSubmitQuiz_AllWrong_ReturnsZeroScore(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	sessionID, qIDs := quizFixture(t, true /* withFollowingStep */)

	input := dto.SubmitQuizInput{
		Answers: map[uuid.UUID]string{
			qIDs[0]: "WRONG",
			qIDs[1]: "false",
			qIDs[2]: "/home",
		},
	}

	svc := services.NewScenarioSessionService(sharedTestDB, &mockFlagService{}, &mockVerificationService{})
	result, err := svc.SubmitQuiz(sessionID, input)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.InDelta(t, 0.0, *result.Score, 0.001, "all-wrong quiz must score 0.0")
	assert.Equal(t, 0, *result.CorrectCount)
	assert.Equal(t, 3, result.Total)
	require.NotNil(t, result.NextStep, "all-wrong quiz must still advance — quiz is graded, not gated")
	assert.Equal(t, 1, *result.NextStep)
}

// TestSubmitQuiz_PartialCorrect_ReturnsPartialScore — 2 of 3 correct → score
// ≈ 0.67. Score is correct_count / total.
func TestSubmitQuiz_PartialCorrect_ReturnsPartialScore(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	sessionID, qIDs := quizFixture(t, true)

	input := dto.SubmitQuizInput{
		Answers: map[uuid.UUID]string{
			qIDs[0]: "ls",     // correct
			qIDs[1]: "true",   // correct
			qIDs[2]: "/wrong", // wrong
		},
	}

	svc := services.NewScenarioSessionService(sharedTestDB, &mockFlagService{}, &mockVerificationService{})
	result, err := svc.SubmitQuiz(sessionID, input)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.InDelta(t, 2.0/3.0, *result.Score, 0.01, "2 of 3 correct must score ~0.67")
	assert.Equal(t, 2, *result.CorrectCount)
	assert.Equal(t, 3, result.Total)
}

// TestSubmitQuiz_PerQuestionResults_IncludesCorrectness — the response must
// include per_question_results with each question's id and correctness flag.
// The spec allows correct_answer to be exposed post-submission (no longer
// secret); we assert the correctness signal regardless.
func TestSubmitQuiz_PerQuestionResults_IncludesCorrectness(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	sessionID, qIDs := quizFixture(t, true)

	input := dto.SubmitQuizInput{
		Answers: map[uuid.UUID]string{
			qIDs[0]: "ls",     // correct
			qIDs[1]: "false",  // wrong
			qIDs[2]: "/root",  // correct
		},
	}

	svc := services.NewScenarioSessionService(sharedTestDB, &mockFlagService{}, &mockVerificationService{})
	result, err := svc.SubmitQuiz(sessionID, input)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.PerQuestionResults, 3,
		"per_question_results must include one entry per submitted answer")

	byID := make(map[uuid.UUID]dto.QuizQuestionResult, 3)
	for _, r := range result.PerQuestionResults {
		byID[r.QuestionID] = r
	}

	require.Contains(t, byID, qIDs[0])
	assert.True(t, byID[qIDs[0]].Correct, "Q1 (ls) should be correct")

	require.Contains(t, byID, qIDs[1])
	assert.False(t, byID[qIDs[1]].Correct, "Q2 (false) should be incorrect")

	require.Contains(t, byID, qIDs[2])
	assert.True(t, byID[qIDs[2]].Correct, "Q3 (/root) should be correct")
}

// TestSubmitQuiz_StoresProgress — after submission, the matching
// ScenarioStepProgress row must record StepType="quiz", QuizScore, and the
// JSON-encoded QuizAnswers. This lets the teacher dashboard see scored quiz
// attempts.
func TestSubmitQuiz_StoresProgress(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	sessionID, qIDs := quizFixture(t, true)

	input := dto.SubmitQuizInput{
		Answers: map[uuid.UUID]string{
			qIDs[0]: "ls",
			qIDs[1]: "true",
			qIDs[2]: "/wrong",
		},
	}

	svc := services.NewScenarioSessionService(sharedTestDB, &mockFlagService{}, &mockVerificationService{})
	_, err := svc.SubmitQuiz(sessionID, input)
	require.NoError(t, err)

	var progress models.ScenarioStepProgress
	require.NoError(t, sharedTestDB.
		Where("session_id = ? AND step_order = ?", sessionID, 0).
		First(&progress).Error)

	assert.Equal(t, "quiz", progress.StepType,
		"ScenarioStepProgress.StepType must be set to \"quiz\" on quiz submission")
	require.NotNil(t, progress.QuizScore, "QuizScore must be persisted on quiz submission")
	assert.InDelta(t, 2.0/3.0, *progress.QuizScore, 0.01)
	assert.NotEmpty(t, progress.QuizAnswers,
		"QuizAnswers must be persisted as JSON on quiz submission")
	// Sanity: each submitted answer ID appears in the JSON blob.
	for _, qID := range qIDs {
		assert.Contains(t, progress.QuizAnswers, qID.String(),
			"QuizAnswers JSON should reference each submitted question ID")
	}
}

// TestSubmitQuiz_NonQuizStep_ReturnsError — calling SubmitQuiz on a step that
// is not step_type=quiz must error out (400/422 at the controller; service
// returns error).
func TestSubmitQuiz_NonQuizStep_ReturnsError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db := freshTestDB(t)

	scenario := models.Scenario{
		Name: "submit-quiz-on-terminal", Title: "Wrong Step Type", InstanceType: "ubuntu:22.04", CreatedByID: "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	step0 := models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 0, Title: "Terminal", StepType: "terminal",
	}
	require.NoError(t, db.Create(&step0).Error)

	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "student-1", CurrentStep: 0,
		Status: "active", StartedAt: time.Now(),
	}
	require.NoError(t, db.Create(&session).Error)
	require.NoError(t, db.Create(&models.ScenarioStepProgress{
		SessionID: session.ID, StepOrder: 0, Status: "active",
	}).Error)

	svc := services.NewScenarioSessionService(db, &mockFlagService{}, &mockVerificationService{})
	result, err := svc.SubmitQuiz(session.ID, dto.SubmitQuizInput{
		Answers: map[uuid.UUID]string{uuid.New(): "anything"},
	})

	assert.Error(t, err, "submit-quiz on a non-quiz step must error")
	assert.Nil(t, result)
}

// TestSubmitQuiz_MissingAnswers_ReturnsError — request body without an answers
// map (nil/empty) must error so the controller can return 400.
func TestSubmitQuiz_MissingAnswers_ReturnsError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	sessionID, _ := quizFixture(t, true)

	svc := services.NewScenarioSessionService(sharedTestDB, &mockFlagService{}, &mockVerificationService{})

	// Nil map → error
	resultNil, errNil := svc.SubmitQuiz(sessionID, dto.SubmitQuizInput{Answers: nil})
	assert.Error(t, errNil, "nil answers map must error")
	assert.Nil(t, resultNil)

	// Empty map → error (no answers submitted)
	resultEmpty, errEmpty := svc.SubmitQuiz(sessionID, dto.SubmitQuizInput{
		Answers: map[uuid.UUID]string{},
	})
	assert.Error(t, errEmpty, "empty answers map must error")
	assert.Nil(t, resultEmpty)
}

// TestSubmitQuiz_WrongQuestionIds_ReturnsError — answers referencing question
// IDs that don't belong to the current step must error (422 territory).
func TestSubmitQuiz_WrongQuestionIds_ReturnsError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	sessionID, _ := quizFixture(t, true)

	svc := services.NewScenarioSessionService(sharedTestDB, &mockFlagService{}, &mockVerificationService{})

	bogusID := uuid.New()
	result, err := svc.SubmitQuiz(sessionID, dto.SubmitQuizInput{
		Answers: map[uuid.UUID]string{bogusID: "anything"},
	})

	assert.Error(t, err, "answers referencing non-existent question IDs must error")
	assert.Nil(t, result)
}

// TestSubmitQuiz_ExamMode_OmitsPerQuestionResults — with
// show_immediate_feedback=false the teacher has chosen to withhold answers:
// the response must carry the score line ONLY. Anything more ships the
// correct answers to the learner's browser where devtools can read them,
// making "exam mode" purely cosmetic (§7.5).
func TestSubmitQuiz_ExamMode_OmitsPerQuestionResults(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	sessionID, qIDs := quizFixture(t, true)

	// Flip the fixture's quiz step to exam mode.
	require.NoError(t, sharedTestDB.Model(&models.ScenarioStep{}).
		Where("step_type = ?", "quiz").
		Update("show_immediate_feedback", false).Error)

	input := dto.SubmitQuizInput{
		Answers: map[uuid.UUID]string{
			qIDs[0]: "ls",     // correct
			qIDs[1]: "false",  // wrong
			qIDs[2]: "/root",  // correct
		},
	}

	svc := services.NewScenarioSessionService(sharedTestDB, &mockFlagService{}, &mockVerificationService{})
	result, err := svc.SubmitQuiz(sessionID, input)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, result.PerQuestionResults,
		"exam mode must not return per-question results — the breakdown "+
			"carries correct answers and explanations")
	// The grade itself is withheld too: a learner who sees "2/3" in exam mode
	// knows how they did, which is what the teacher chose to keep for later.
	assert.Nil(t, result.Score, "exam mode must not return the score")
	assert.Nil(t, result.CorrectCount, "exam mode must not return the correct count")
	assert.Equal(t, 3, result.Total)
	require.NotNil(t, result.NextStep, "exam mode still advances")

	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(encoded, &payload))
	for _, key := range []string{"score", "correct_count", "per_question_results"} {
		assert.NotContains(t, payload, key, "exam-mode payload must not carry %s", key)
	}
	assert.Contains(t, payload, "total")

	// The grade is still recorded for the teacher.
	var progress models.ScenarioStepProgress
	require.NoError(t, sharedTestDB.Where("session_id = ? AND step_order = 0", sessionID).First(&progress).Error)
	require.NotNil(t, progress.QuizScore)
	assert.InDelta(t, 2.0/3.0, *progress.QuizScore, 0.01)
}

// TestSubmitQuiz_ExamModePreview_RevealsFullCorrection — a preview session is
// the author testing their own quiz, so exam mode must not hide anything
// from them: they need the score and every correct answer to check it.
func TestSubmitQuiz_ExamModePreview_RevealsFullCorrection(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	sessionID, qIDs := quizFixture(t, true)
	require.NoError(t, sharedTestDB.Model(&models.ScenarioStep{}).
		Where("step_type = ?", "quiz").
		Update("show_immediate_feedback", false).Error)
	require.NoError(t, sharedTestDB.Model(&models.ScenarioSession{}).
		Where("id = ?", sessionID).Update("is_preview", true).Error)

	svc := services.NewScenarioSessionService(sharedTestDB, &mockFlagService{}, &mockVerificationService{})
	result, err := svc.SubmitQuiz(sessionID, dto.SubmitQuizInput{
		Answers: map[uuid.UUID]string{qIDs[0]: "cd", qIDs[1]: "true", qIDs[2]: "/root"},
	})

	require.NoError(t, err)
	require.NotNil(t, result.Score)
	assert.InDelta(t, 2.0/3.0, *result.Score, 0.01)
	require.NotNil(t, result.CorrectCount)
	assert.Equal(t, 2, *result.CorrectCount)
	require.Len(t, result.PerQuestionResults, 3)
	assert.Equal(t, "ls", result.PerQuestionResults[0].CorrectAnswer)
	assert.False(t, result.PerQuestionResults[0].Correct)
}

// TestSubmitQuiz_PartialMode_ScoresTicksAndPersists — a partial-scoring step
// gives credit per right tick, reports each question's points, and stores the
// same score on the progress row.
func TestSubmitQuiz_PartialMode_ScoresTicksAndPersists(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	sessionID, qIDs := quizFixture(t, true)
	var step models.ScenarioStep
	require.NoError(t, sharedTestDB.Where("step_type = ?", "quiz").First(&step).Error)
	require.NoError(t, sharedTestDB.Model(&step).Update("quiz_scoring_mode", models.QuizScoringPartial).Error)
	require.NoError(t, sharedTestDB.Model(&models.ScenarioStepQuestion{}).Where("id = ?", qIDs[0]).
		Updates(map[string]any{"question_type": "multi_answer", "options": `["ls","cd","rm","dir"]`, "correct_answer": "[0,3]"}).Error)

	svc := services.NewScenarioSessionService(sharedTestDB, &mockFlagService{}, &mockVerificationService{})
	result, err := svc.SubmitQuiz(sessionID, dto.SubmitQuizInput{
		// Q1: one right tick of two (+0.5) and one wrong (-0.5) → 0;
		// Q2 right (+1); Q3 wrong (-1). Total max(0, 0) / 3.
		Answers: map[uuid.UUID]string{qIDs[0]: "[0,1]", qIDs[1]: "true", qIDs[2]: "/home"},
	})

	require.NoError(t, err)
	require.NotNil(t, result.Score)
	assert.InDelta(t, 0.0, *result.Score, 1e-9)
	assert.Equal(t, 1, *result.CorrectCount)
	require.Len(t, result.PerQuestionResults, 3)
	assert.False(t, result.PerQuestionResults[0].Correct)
	assert.True(t, result.PerQuestionResults[1].Correct)
	assert.False(t, result.PerQuestionResults[2].Correct)

	var progress models.ScenarioStepProgress
	require.NoError(t, sharedTestDB.Where("session_id = ? AND step_order = 0", sessionID).First(&progress).Error)
	require.NotNil(t, progress.QuizScore)
	assert.InDelta(t, 0.0, *progress.QuizScore, 1e-9)
	require.NotNil(t, progress.QuizCorrectCount)
	assert.Equal(t, 1, *progress.QuizCorrectCount)
}

// TestSubmitQuiz_LearningMode_ExposesCorrectAnswers — with
// show_immediate_feedback=true the breakdown is the point: correct answers
// and explanations are deliberately revealed after submission.
func TestSubmitQuiz_LearningMode_ExposesCorrectAnswers(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	sessionID, qIDs := quizFixture(t, true)

	input := dto.SubmitQuizInput{
		Answers: map[uuid.UUID]string{
			qIDs[0]: "cd", // wrong on purpose — reveal must include the right answer
			qIDs[1]: "true",
			qIDs[2]: "/root",
		},
	}

	svc := services.NewScenarioSessionService(sharedTestDB, &mockFlagService{}, &mockVerificationService{})
	result, err := svc.SubmitQuiz(sessionID, input)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.PerQuestionResults, 3)
	byID := make(map[uuid.UUID]dto.QuizQuestionResult, 3)
	for _, r := range result.PerQuestionResults {
		byID[r.QuestionID] = r
	}
	assert.Equal(t, "ls", byID[qIDs[0]].CorrectAnswer,
		"learning mode reveals the correct answer after submission")
}

// --- Survey steps (#568) ---
//
// A survey reuses /submit-quiz but is never graded: every question is
// optional, nothing is scored, and no correction is ever returned — not even
// to the author in a preview, since a survey has no right answers to show.

// surveyFixture creates a survey step (two questions, followed by a terminal
// step) and a session sitting on it. Returns the session ID and the question
// IDs in order.
func surveyFixture(t *testing.T) (uuid.UUID, []uuid.UUID) {
	t.Helper()
	db := freshTestDB(t)

	scenario := models.Scenario{
		Name: "survey-submit", Title: "Survey Submit", InstanceType: "ubuntu:22.04", CreatedByID: "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	step0 := models.ScenarioStep{ScenarioID: scenario.ID, Order: 0, Title: "Feedback", StepType: "survey"}
	require.NoError(t, db.Create(&step0).Error)
	require.NoError(t, db.Create(&models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 1, Title: "Next", StepType: "terminal",
	}).Error)

	q1 := models.ScenarioStepQuestion{
		StepID: step0.ID, Order: 1, QuestionText: "How clear was the lab?",
		QuestionType: "multiple_choice", Options: `["Clear","Unclear"]`,
	}
	q2 := models.ScenarioStepQuestion{
		StepID: step0.ID, Order: 2, QuestionText: "What would you change?",
		QuestionType: "free_text",
	}
	require.NoError(t, db.Create(&q1).Error)
	require.NoError(t, db.Create(&q2).Error)

	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "student-1", CurrentStep: 0,
		Status: "active", StartedAt: time.Now(),
	}
	require.NoError(t, db.Create(&session).Error)
	require.NoError(t, db.Create(&models.ScenarioStepProgress{
		SessionID: session.ID, StepOrder: 0, Status: "active",
	}).Error)
	require.NoError(t, db.Create(&models.ScenarioStepProgress{
		SessionID: session.ID, StepOrder: 1, Status: "locked",
	}).Error)

	return session.ID, []uuid.UUID{q1.ID, q2.ID}
}

// assertNoCorrection checks a survey response carries the question count and
// the next step, and nothing that reads as a grade.
func assertNoCorrection(t *testing.T, result *dto.SubmitQuizResponse) {
	t.Helper()
	require.NotNil(t, result)
	assert.Nil(t, result.Score, "a survey is never scored")
	assert.Nil(t, result.CorrectCount, "a survey has no correct answers")
	assert.Empty(t, result.PerQuestionResults, "a survey returns no correction")
	assert.Equal(t, 2, result.Total)
	require.NotNil(t, result.NextStep, "a survey advances the session")
	assert.Equal(t, 1, *result.NextStep)

	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(encoded, &payload))
	for _, key := range []string{"score", "correct_count", "per_question_results"} {
		assert.NotContains(t, payload, key, "a survey payload must not carry %s", key)
	}
}

// Every survey question is optional: sending nothing at all still moves on.
func TestSubmitQuiz_SurveyEmptyAnswers_AdvancesWithoutScore(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	sessionID, _ := surveyFixture(t)

	svc := services.NewScenarioSessionService(sharedTestDB, &mockFlagService{}, &mockVerificationService{})
	result, err := svc.SubmitQuiz(sessionID, dto.SubmitQuizInput{Answers: map[uuid.UUID]string{}})

	require.NoError(t, err, "an empty survey submission must be accepted")
	assertNoCorrection(t, result)
}

func TestSubmitQuiz_SurveyStoresAnswersWithoutScore(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	sessionID, qIDs := surveyFixture(t)
	answers := map[uuid.UUID]string{qIDs[0]: "1", qIDs[1]: "More time on pipes"}

	svc := services.NewScenarioSessionService(sharedTestDB, &mockFlagService{}, &mockVerificationService{})
	result, err := svc.SubmitQuiz(sessionID, dto.SubmitQuizInput{Answers: answers})
	require.NoError(t, err)
	assertNoCorrection(t, result)

	var progress models.ScenarioStepProgress
	require.NoError(t, sharedTestDB.Where("session_id = ? AND step_order = 0", sessionID).First(&progress).Error)
	assert.Equal(t, "survey", progress.StepType)
	assert.Nil(t, progress.QuizScore, "a survey stores no score")
	assert.Nil(t, progress.QuizCorrectCount, "a survey stores no correct count")

	var stored map[uuid.UUID]string
	require.NoError(t, json.Unmarshal([]byte(progress.QuizAnswers), &stored),
		"the learner's answers are stored as JSON for the trainer")
	assert.Equal(t, answers, stored)
}

// A preview reveals a quiz's correction to its author; a survey has none.
func TestSubmitQuiz_SurveyPreview_ReturnsNoCorrection(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	sessionID, qIDs := surveyFixture(t)
	require.NoError(t, sharedTestDB.Model(&models.ScenarioSession{}).
		Where("id = ?", sessionID).Update("is_preview", true).Error)

	svc := services.NewScenarioSessionService(sharedTestDB, &mockFlagService{}, &mockVerificationService{})
	result, err := svc.SubmitQuiz(sessionID, dto.SubmitQuizInput{
		Answers: map[uuid.UUID]string{qIDs[0]: "0"},
	})

	require.NoError(t, err)
	assertNoCorrection(t, result)
}

func TestSubmitQuiz_SurveyUnknownQuestion_IsRejected(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	sessionID, qIDs := surveyFixture(t)

	svc := services.NewScenarioSessionService(sharedTestDB, &mockFlagService{}, &mockVerificationService{})
	result, err := svc.SubmitQuiz(sessionID, dto.SubmitQuizInput{
		Answers: map[uuid.UUID]string{qIDs[0]: "0", uuid.New(): "anything"},
	})

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "unknown question",
		"the survey must be accepted as a step and refused for the stray answer")
}

// Regression: only surveys may be submitted empty — a quiz still needs answers.
func TestSubmitQuiz_QuizEmptyAnswers_StillRejected(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	sessionID, _ := quizFixture(t, true)

	svc := services.NewScenarioSessionService(sharedTestDB, &mockFlagService{}, &mockVerificationService{})
	result, err := svc.SubmitQuiz(sessionID, dto.SubmitQuizInput{Answers: map[uuid.UUID]string{}})

	assert.Error(t, err, "an empty quiz submission must still be refused")
	assert.Nil(t, result)
}

// --- Survey answer limits and choice validation (review of !685) ---

// addSurveyQuestion adds a question to the survey step the session sits on.
func addSurveyQuestion(t *testing.T, sessionID uuid.UUID, q models.ScenarioStepQuestion) uuid.UUID {
	t.Helper()
	var session models.ScenarioSession
	require.NoError(t, sharedTestDB.First(&session, "id = ?", sessionID).Error)
	var step models.ScenarioStep
	require.NoError(t, sharedTestDB.Where("scenario_id = ? AND step_type = ?", session.ScenarioID, "survey").First(&step).Error)
	q.StepID = step.ID
	require.NoError(t, sharedTestDB.Create(&q).Error)
	return q.ID
}

func submitSurvey(t *testing.T, sessionID uuid.UUID, answers map[uuid.UUID]string) error {
	t.Helper()
	svc := services.NewScenarioSessionService(sharedTestDB, &mockFlagService{}, &mockVerificationService{})
	_, err := svc.SubmitQuiz(sessionID, dto.SubmitQuizInput{Answers: answers})
	return err
}

// A free-text survey answer is capped at 4000 characters — characters, not
// bytes, so a French answer gets the same room as an English one.
func TestSubmitQuiz_SurveyFreeTextOverLimit_IsRejected(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	sessionID, qIDs := surveyFixture(t)

	err := submitSurvey(t, sessionID, map[uuid.UUID]string{qIDs[1]: strings.Repeat("é", 4001)})

	require.Error(t, err, "4001 characters is over the cap")
	assert.Contains(t, err.Error(), "4000", "the error must say what the limit is")
}

func TestSubmitQuiz_SurveyFreeTextAtLimit_IsAccepted(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	sessionID, qIDs := surveyFixture(t)

	// 4000 runes, 8000 bytes: a byte count would refuse it.
	assert.NoError(t, submitSurvey(t, sessionID, map[uuid.UUID]string{qIDs[1]: strings.Repeat("é", 4000)}))
}

// Every survey question is optional, but an answer that is given must be one
// the question could have produced.
func TestSubmitQuiz_SurveyChoiceAnswers_AreValidated(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	cases := []struct {
		name     string
		multi    bool
		answer   string
		accepted bool
	}{
		{"multiple_choice in range", false, "1", true},
		{"multiple_choice empty", false, "", true},
		{"multiple_choice out of range", false, "99", false},
		{"multiple_choice negative", false, "-1", false},
		{"multiple_choice not a number", false, "Clear", false},
		{"multi_answer in range", true, "[0,2]", true},
		{"multi_answer empty array", true, "[]", true},
		{"multi_answer empty string", true, "", true},
		{"multi_answer out of range", true, "[0,5]", false},
		{"multi_answer not an array", true, "0", false},
		{"multi_answer array of strings", true, `["grep"]`, false},
		{"multi_answer not JSON", true, "grep, sed", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sessionID, qIDs := surveyFixture(t) // q1: multiple_choice with 2 options
			target := qIDs[0]
			if tc.multi {
				target = addSurveyQuestion(t, sessionID, models.ScenarioStepQuestion{
					Order: 3, QuestionText: "Topics?", QuestionType: "multi_answer", Options: `["grep","sed","awk"]`,
				})
			}

			err := submitSurvey(t, sessionID, map[uuid.UUID]string{target: tc.answer})

			if tc.accepted {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err, "answer %q must be refused", tc.answer)
			}
		})
	}
}

// The body is capped before it is parsed, like test-verify's script: an
// oversized submission never reaches the service.
func TestSubmitQuizRoute_BodyOver64KiB_IsRejected(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	sessionID, qIDs := surveyFixture(t)
	require.NoError(t, sharedTestDB.Model(&models.ScenarioSession{}).
		Where("id = ?", sessionID).Update("user_id", "survey-owner").Error)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api/v1")
	api.Use(func(c *gin.Context) {
		c.Set("userId", "survey-owner")
		c.Set("userRoles", []string{"member"})
		c.Next()
	})
	api.POST("/scenario-sessions/:id/submit-quiz", scenarioController.NewScenarioProgressController(sharedTestDB).SubmitQuiz)

	// Valid JSON for a real question: only the size is wrong with it.
	body := fmt.Sprintf(`{"answers":{%q:%q}}`, qIDs[1].String(), strings.Repeat("x", 64*1024+1))
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/scenario-sessions/"+sessionID.String()+"/submit-quiz", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code,
		"a body over 64 KiB must be refused at the door with 413: %s", w.Body.String())
}
