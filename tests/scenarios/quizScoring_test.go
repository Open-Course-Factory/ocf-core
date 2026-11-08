package scenarios_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

func quizScoringStep(mode string, penalty *float64) models.ScenarioStep {
	return models.ScenarioStep{StepType: "quiz", QuizScoringMode: mode, QuizWrongPenalty: penalty}
}

func penaltyOf(v float64) *float64 { return &v }

func multiAnswerQuestion(correct string, points int) models.ScenarioStepQuestion {
	return models.ScenarioStepQuestion{
		BaseModel:    entityManagementModels.BaseModel{ID: uuid.New()},
		QuestionType: "multi_answer", Options: `["a","b","c","d"]`,
		CorrectAnswer: correct, Points: points,
	}
}

func singleChoiceQuestion(correct string, points int) models.ScenarioStepQuestion {
	return models.ScenarioStepQuestion{
		BaseModel:    entityManagementModels.BaseModel{ID: uuid.New()},
		QuestionType: "multiple_choice", Options: `["a","b","c"]`,
		CorrectAnswer: correct, Points: points,
	}
}

func TestScoreQuiz_PartialMultiAnswer_ScoresPerTick(t *testing.T) {
	cases := []struct {
		name        string
		penalty     *float64
		ticks       string
		wantScore   float64
		wantCorrect bool
	}{
		{"all right ticks", nil, "[0,2]", 1.0, true},
		{"one wrong tick, default penalty 1", nil, "[0,1,2]", 0.5, false},
		{"one wrong tick, penalty 0.5", penaltyOf(0.5), "[0,1,2]", 0.75, false},
		{"one wrong tick, penalty 0", penaltyOf(0), "[0,1,2]", 1.0, false},
		{"half the right ticks", nil, "[2]", 0.5, false},
		{"empty answer", nil, "", 0, false},
		{"empty tick list", nil, "[]", 0, false},
		{"only wrong ticks floor at zero", nil, "[1,3]", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q := multiAnswerQuestion("[0,2]", 1)
			got := services.ScoreQuiz(quizScoringStep(models.QuizScoringPartial, tc.penalty),
				[]models.ScenarioStepQuestion{q}, map[uuid.UUID]string{q.ID: tc.ticks})

			assert.InDelta(t, tc.wantScore, got.Score, 1e-9)
			require.Len(t, got.Questions, 1)
			assert.Equal(t, tc.wantCorrect, got.Questions[0].Correct)
			assert.InDelta(t, 1.0, got.Questions[0].Max, 1e-9)
		})
	}
}

func TestScoreQuiz_PartialSingleChoiceWrong_SubtractsPenaltyAndFloorsAtZero(t *testing.T) {
	right := multiAnswerQuestion("[0,2]", 1)
	wrong := singleChoiceQuestion("0", 1)
	got := services.ScoreQuiz(quizScoringStep(models.QuizScoringPartial, nil),
		[]models.ScenarioStepQuestion{right, wrong},
		map[uuid.UUID]string{right.ID: "[0,2]", wrong.ID: "1"})

	assert.InDelta(t, -1.0, got.Questions[1].Earned, 1e-9, "a wrong single answer costs penalty * points")
	assert.InDelta(t, 0.0, got.Score, 1e-9, "+1 - 1 over 2 points")
	assert.Equal(t, 1, got.CorrectCount)

	alone := services.ScoreQuiz(quizScoringStep(models.QuizScoringPartial, nil),
		[]models.ScenarioStepQuestion{wrong}, map[uuid.UUID]string{wrong.ID: "2"})
	assert.InDelta(t, 0.0, alone.Score, 1e-9, "a negative total floors at 0")
}

func TestScoreQuiz_PartialQuestionFloor_NoQuestionGoesBelowZero(t *testing.T) {
	right := multiAnswerQuestion("[0,2]", 1)
	wrongTicks := multiAnswerQuestion("[0,2]", 1)
	wrongSingle := singleChoiceQuestion("0", 1)
	halfAndWrong := multiAnswerQuestion("[0,2]", 1)
	got := services.ScoreQuiz(quizScoringStep(models.QuizScoringPartialQuestionFloor, penaltyOf(0.25)),
		[]models.ScenarioStepQuestion{right, wrongTicks, wrongSingle, halfAndWrong},
		map[uuid.UUID]string{right.ID: "[0,2]", wrongTicks.ID: "[1,3]", wrongSingle.ID: "1", halfAndWrong.ID: "[0,1]"})

	assert.InDelta(t, 0.0, got.Questions[1].Earned, 1e-9, "only wrong ticks floor at 0, not -0.25")
	assert.InDelta(t, 0.0, got.Questions[2].Earned, 1e-9, "a wrong single answer floors at 0")
	assert.InDelta(t, 0.5-0.125, got.Questions[3].Earned, 1e-9, "a positive question keeps its penalty")
	assert.InDelta(t, (1+0.375)/4, got.Score, 1e-9)
	assert.Equal(t, 1, got.CorrectCount)

	plain := services.ScoreQuiz(quizScoringStep(models.QuizScoringPartial, penaltyOf(0.25)),
		[]models.ScenarioStepQuestion{right, wrongTicks}, map[uuid.UUID]string{right.ID: "[0,2]", wrongTicks.ID: "[1,3]"})
	assert.InDelta(t, -0.25, plain.Questions[1].Earned, 1e-9, "plain partial still lets a question go negative")
}

func TestScoreQuiz_PartialEmptySingleAnswer_EarnsNothing(t *testing.T) {
	q := singleChoiceQuestion("0", 1)
	got := services.ScoreQuiz(quizScoringStep(models.QuizScoringPartial, nil),
		[]models.ScenarioStepQuestion{q}, map[uuid.UUID]string{})

	assert.InDelta(t, 0.0, got.Questions[0].Earned, 1e-9)
	assert.InDelta(t, 0.0, got.Score, 1e-9)
}

func TestScoreQuiz_PartialPoints_WeightQuestions(t *testing.T) {
	heavy := multiAnswerQuestion("[0,2]", 3)
	light := singleChoiceQuestion("0", 1)
	unpointed := singleChoiceQuestion("0", 0)
	got := services.ScoreQuiz(quizScoringStep(models.QuizScoringPartial, penaltyOf(0)),
		[]models.ScenarioStepQuestion{heavy, light, unpointed},
		map[uuid.UUID]string{heavy.ID: "[0,2]", light.ID: "2", unpointed.ID: "0"})

	// 3 + 0 + 1 (points <= 0 count as 1) out of 3 + 1 + 1.
	assert.InDelta(t, 4.0/5.0, got.Score, 1e-9)
	assert.InDelta(t, 3.0, got.Questions[0].Earned, 1e-9)
	assert.InDelta(t, 1.0, got.Questions[2].Max, 1e-9)
	assert.Equal(t, 2, got.CorrectCount)
}

func TestScoreQuiz_AllOrNothing_IgnoresPointsAndPartialTicks(t *testing.T) {
	heavy := multiAnswerQuestion("[0,2]", 3)
	light := singleChoiceQuestion("0", 1)
	answers := map[uuid.UUID]string{heavy.ID: "[0,2]", light.ID: "[0]"}

	for _, mode := range []string{"", models.QuizScoringAllOrNothing} {
		got := services.ScoreQuiz(quizScoringStep(mode, nil),
			[]models.ScenarioStepQuestion{heavy, light}, answers)
		assert.InDelta(t, 0.5, got.Score, 1e-9, "mode %q: correct_count / total", mode)
		assert.Equal(t, 1, got.CorrectCount)
	}

	nearMiss := services.ScoreQuiz(quizScoringStep("", nil),
		[]models.ScenarioStepQuestion{heavy}, map[uuid.UUID]string{heavy.ID: "[0]"})
	assert.InDelta(t, 0.0, nearMiss.Score, 1e-9, "all-or-nothing gives no credit for a partial tick set")
}

func TestScoreQuiz_NoQuestions_ScoresZero(t *testing.T) {
	got := services.ScoreQuiz(quizScoringStep(models.QuizScoringPartial, nil), nil, nil)
	assert.InDelta(t, 0.0, got.Score, 1e-9)
	assert.Empty(t, got.Questions)
}

func TestComputeCorrectCounts_PartialQuiz_UsesPersistedCorrectCount(t *testing.T) {
	step := models.ScenarioStep{BaseModel: entityManagementModels.BaseModel{ID: uuid.New()}, StepType: "quiz", Order: 0}
	zero, one := 0.0, 1
	// A partial-scored quiz can score 0 with one question exactly right
	// (penalties ate the rest); the score can no longer be multiplied back.
	progress := []models.ScenarioStepProgress{{StepOrder: 0, QuizScore: &zero, QuizCorrectCount: &one}}

	correct, total := services.ComputeCorrectCountsFromLoaded(
		[]models.ScenarioStep{step}, progress, nil, map[uuid.UUID]int{step.ID: 3})

	assert.Equal(t, int64(1), correct)
	assert.Equal(t, int64(3), total)

	legacyScore := 2.0 / 3.0
	legacy := []models.ScenarioStepProgress{{StepOrder: 0, QuizScore: &legacyScore}}
	correct, _ = services.ComputeCorrectCountsFromLoaded(
		[]models.ScenarioStep{step}, legacy, nil, map[uuid.UUID]int{step.ID: 3})
	assert.Equal(t, int64(2), correct, "rows without a stored count fall back to score × questions")
}
