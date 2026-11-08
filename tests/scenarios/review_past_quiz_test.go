package scenarios_test

// Going back to a submitted quiz step: in learning mode the learner sees their
// answers and the correction again; in exam mode neither reaches the browser.

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

func submitAndReviewQuiz(t *testing.T, examMode bool) (*dto.CurrentStepResponse, []uuid.UUID) {
	t.Helper()
	sessionID, qIDs := quizFixture(t, true)
	if examMode {
		require.NoError(t, sharedTestDB.Model(&models.ScenarioStep{}).
			Where("title = ?", "Quiz Step").Update("show_immediate_feedback", false).Error)
	}
	svc := services.NewScenarioSessionService(sharedTestDB, &mockFlagService{}, &mockVerificationService{})
	_, err := svc.SubmitQuiz(sessionID, dto.SubmitQuizInput{Answers: map[uuid.UUID]string{
		qIDs[0]: "ls", qIDs[1]: "false", qIDs[2]: "/root",
	}})
	require.NoError(t, err)

	step, err := svc.GetStepByOrder(sessionID, 0)
	require.NoError(t, err)
	return step, qIDs
}

func TestGetStepByOrder_PastLearningQuiz_ReplaysAnswersAndCorrection(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	step, qIDs := submitAndReviewQuiz(t, false)

	assert.Equal(t, "false", step.QuizAnswers[qIDs[1]], "the learner's own answer comes back")
	require.NotNil(t, step.QuizResult)
	assert.Equal(t, 2, *step.QuizResult.CorrectCount)
	assert.Equal(t, 3, step.QuizResult.Total)
	require.Len(t, step.QuizResult.PerQuestionResults, 3)
	assert.False(t, step.QuizResult.PerQuestionResults[1].Correct)
	assert.Equal(t, "true", step.QuizResult.PerQuestionResults[1].CorrectAnswer)
}

func TestGetStepByOrder_PastExamQuiz_WithholdsAnswersAndCorrection(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	step, _ := submitAndReviewQuiz(t, true)

	assert.Nil(t, step.QuizAnswers)
	assert.Nil(t, step.QuizResult)
}

// A run that ended on its quiz opens in review mode on that step, read through
// GetCurrentStep: the replay must be there too.
func TestGetCurrentStep_RunEndedOnLearningQuiz_ReplaysAnswersAndCorrection(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	sessionID, qIDs := quizFixture(t, false)
	svc := services.NewScenarioSessionService(sharedTestDB, &mockFlagService{}, &mockVerificationService{})
	_, err := svc.SubmitQuiz(sessionID, dto.SubmitQuizInput{Answers: map[uuid.UUID]string{qIDs[0]: "ls"}})
	require.NoError(t, err)

	step, err := svc.GetCurrentStep(sessionID)
	require.NoError(t, err)
	require.NotNil(t, step.QuizResult)
	assert.Equal(t, 1, *step.QuizResult.CorrectCount)
	assert.Equal(t, "ls", step.QuizAnswers[qIDs[0]])
}
