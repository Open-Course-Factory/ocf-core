package scenarios_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

// Decision-game scoring: each option of a multiple_choice question carries its
// own points, a step can sit outside the grade, and later scripts can read the
// answers already given.

func pointedQuestion(optionPoints string) models.ScenarioStepQuestion {
	return models.ScenarioStepQuestion{
		BaseModel:    entityManagementModels.BaseModel{ID: uuid.New()},
		QuestionType: "multiple_choice", Options: `["a","b","c","d"]`,
		CorrectAnswer: "0", OptionPoints: optionPoints, Points: 1,
	}
}

func TestScoreQuiz_OptionPoints_EarnsTheChosenOptionsPoints(t *testing.T) {
	cases := []struct {
		name        string
		mode        string
		answer      string
		wantScore   float64
		wantEarned  float64
		wantCorrect bool
	}{
		{"best option", "", "0", 1, 3, true},
		{"second best", "", "3", 2.0 / 3, 2, false},
		{"zero-point option", "", "2", 0, 0, false},
		{"empty answer", "", "", 0, 0, false},
		{"not an index", "", "x", 0, 0, false},
		{"out of range", "", "9", 0, 0, false},
		{"partial mode takes no penalty from an option", models.QuizScoringPartial, "1", 1.0 / 3, 1, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q := pointedQuestion("[3,1,0,2]")
			got := services.ScoreQuiz(quizScoringStep(tc.mode, nil),
				[]models.ScenarioStepQuestion{q}, map[uuid.UUID]string{q.ID: tc.answer})

			assert.InDelta(t, tc.wantScore, got.Score, 1e-9)
			require.Len(t, got.Questions, 1)
			assert.InDelta(t, tc.wantEarned, got.Questions[0].Earned, 1e-9)
			assert.InDelta(t, 3, got.Questions[0].Max, 1e-9)
			assert.Equal(t, tc.wantCorrect, got.Questions[0].Correct)
		})
	}
}

func TestScoreQuiz_OptionPoints_WeighAgainstPlainQuestionsByPoints(t *testing.T) {
	pointed := pointedQuestion("[3,1,0,2]")
	plain := singleChoiceQuestion("1", 1)
	got := services.ScoreQuiz(quizScoringStep("", nil),
		[]models.ScenarioStepQuestion{pointed, plain},
		map[uuid.UUID]string{pointed.ID: "3", plain.ID: "1"})

	// (2 + 1) / (3 + 1)
	assert.InDelta(t, 0.75, got.Score, 1e-9)
	assert.Equal(t, 1, got.CorrectCount)
}

func TestScenarioContentProblems_OptionPoints(t *testing.T) {
	cases := []struct {
		name         string
		optionPoints string
		correct      string
		wantProblem  bool
	}{
		{"absent", "", "0", false},
		{"valid", "[3,1,0,2]", "0", false},
		{"best answer tied for max", "[3,3,0,2]", "1", false},
		{"not JSON", "3,1,0,2", "0", true},
		{"wrong length", "[3,1,0]", "0", true},
		{"negative", "[3,-1,0,2]", "0", true},
		{"all zero", "[0,0,0,0]", "0", true},
		{"correct answer is not the best option", "[3,1,0,2]", "1", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q := pointedQuestion(tc.optionPoints)
			q.QuestionText = "Which?"
			q.CorrectAnswer = tc.correct
			scenario := &models.Scenario{Title: "t", Steps: []models.ScenarioStep{
				{Title: "decide", StepType: models.StepTypeQuiz, Questions: []models.ScenarioStepQuestion{q}},
			}}
			problems := services.ScenarioContentProblems(scenario)
			if tc.wantProblem {
				assert.NotEmpty(t, problems)
			} else {
				assert.Empty(t, problems)
			}
		})
	}
}

func TestScenarioContentProblems_OptionPoints_OnlyOnMultipleChoice(t *testing.T) {
	q := multiAnswerQuestion("[0,2]", 1)
	q.QuestionText = "Which?"
	q.OptionPoints = "[1,0,1,0]"
	scenario := &models.Scenario{Title: "t", Steps: []models.ScenarioStep{
		{Title: "decide", StepType: models.StepTypeQuiz, Questions: []models.ScenarioStepQuestion{q}},
	}}
	assert.NotEmpty(t, services.ScenarioContentProblems(scenario))
}

func TestWeightedGrade_UngradedSteps_AreLeftOut(t *testing.T) {
	half := 0.5
	steps := []models.ScenarioStep{
		{Order: 0, StepType: models.StepTypeTerminal, Ungraded: true},
		{Order: 1, StepType: models.StepTypeQuiz},
		{Order: 2, StepType: models.StepTypeQuiz, Ungraded: true},
	}
	progress := []models.ScenarioStepProgress{
		{StepOrder: 0, Status: "completed"},
		{StepOrder: 1, QuizScore: &half},
		{StepOrder: 2, QuizScore: &half},
	}

	assert.InDelta(t, 50, services.ComputeWeightedGradeFromLoaded(steps, progress, nil), 1e-9)
	assert.True(t, services.HasGradableStep(steps))
	assert.False(t, services.HasGradableStep(steps[:1]))
	assert.Zero(t, services.ComputeWeightedGradeFromLoaded(steps[:1], progress, nil))
}

func TestComputeCorrectCounts_UngradedQuiz_Ignored(t *testing.T) {
	graded, ungraded := uuid.New(), uuid.New()
	steps := []models.ScenarioStep{
		{BaseModel: entityManagementModels.BaseModel{ID: graded}, Order: 0, StepType: models.StepTypeQuiz},
		{BaseModel: entityManagementModels.BaseModel{ID: ungraded}, Order: 1, StepType: models.StepTypeQuiz, Ungraded: true},
	}
	one := 1
	progress := []models.ScenarioStepProgress{
		{StepOrder: 0, QuizCorrectCount: &one},
		{StepOrder: 1, QuizCorrectCount: &one},
	}
	correct, total := services.ComputeCorrectCountsFromLoaded(steps, progress, nil,
		map[uuid.UUID]int{graded: 2, ungraded: 2})

	assert.Equal(t, int64(1), correct)
	assert.Equal(t, int64(2), total)
}

func TestQuizAnswersDocument_KeysByStepAndQuestionOrder(t *testing.T) {
	q0, q1, s0 := uuid.New(), uuid.New(), uuid.New()
	steps := []models.ScenarioStep{
		{Order: 1, StepType: models.StepTypeQuiz, Questions: []models.ScenarioStepQuestion{
			{BaseModel: entityManagementModels.BaseModel{ID: q0}, Order: 0, CorrectAnswer: "2", Explanation: "secret"},
			{BaseModel: entityManagementModels.BaseModel{ID: q1}, Order: 1, CorrectAnswer: "[0,1]"},
		}},
		{Order: 3, StepType: models.StepTypeSurvey, Questions: []models.ScenarioStepQuestion{
			{BaseModel: entityManagementModels.BaseModel{ID: s0}, Order: 0},
		}},
		{Order: 4, StepType: models.StepTypeQuiz, Questions: []models.ScenarioStepQuestion{
			{BaseModel: entityManagementModels.BaseModel{ID: uuid.New()}, Order: 0},
		}},
	}
	answers := func(m map[uuid.UUID]string) string { b, _ := json.Marshal(m); return string(b) }
	progress := []models.ScenarioStepProgress{
		{StepOrder: 1, QuizAnswers: answers(map[uuid.UUID]string{q0: "1", q1: "[0,2]"})},
		{StepOrder: 3, QuizAnswers: answers(map[uuid.UUID]string{s0: "free text"})},
		{StepOrder: 4},           // not answered yet
		{StepOrder: 9, QuizAnswers: "not json"}, // no such step
	}

	doc := services.QuizAnswersDocument(steps, progress)

	var got map[string]map[string]string
	require.NoError(t, json.Unmarshal([]byte(doc), &got))
	assert.Equal(t, map[string]map[string]string{
		"1": {"0": "1", "1": "[0,2]"},
		"3": {"0": "free text"},
	}, got)
	assert.NotContains(t, doc, "secret")
}

func TestQuizAnswersDocument_NothingAnswered_IsEmpty(t *testing.T) {
	assert.Equal(t, "", services.QuizAnswersDocument(nil, nil))
	assert.Equal(t, "", services.QuizAnswersDocument(
		[]models.ScenarioStep{{Order: 0, StepType: models.StepTypeQuiz}},
		[]models.ScenarioStepProgress{{StepOrder: 0, QuizAnswers: ""}}))
}
