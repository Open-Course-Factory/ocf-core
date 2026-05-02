package scenarios_test

// Red-phase tests for quiz answer submission (#283).
//
// These tests assert the EXPECTED API of a new SubmitQuiz path on
// ScenarioSessionService. They will fail to compile until:
//   - dto.SubmitQuizInput is defined: { Answers map[uuid.UUID]string }
//   - dto.SubmitQuizResponse is defined with: Score float64, CorrectCount int,
//     Total int, NextStep *int, PerQuestionResults []QuizQuestionResult
//   - ScenarioSessionService.SubmitQuiz(sessionID, input) is implemented
//   - ScenarioStepProgress gains StepType, QuizScore, QuizAnswers fields
//
// Per task spec, PerQuestionResults MAY include CorrectAnswer (since after
// submission the answer is no longer secret). We assert that when the answer
// is exposed it matches the expected value, but tolerate either shape by only
// requiring the per-question correctness flag.

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"
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
	assert.InDelta(t, 1.0, result.Score, 0.001, "all-correct quiz must score 1.0")
	assert.Equal(t, 3, result.CorrectCount)
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
	assert.InDelta(t, 0.0, result.Score, 0.001, "all-wrong quiz must score 0.0")
	assert.Equal(t, 0, result.CorrectCount)
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
	assert.InDelta(t, 2.0/3.0, result.Score, 0.01, "2 of 3 correct must score ~0.67")
	assert.Equal(t, 2, result.CorrectCount)
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
