package scenarios_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"soli/formations/src/entityManagement/hooks"
	"soli/formations/src/scenarios/models"
	scenarioHooks "soli/formations/src/scenarios/hooks"
	"soli/formations/src/scenarios/services"
)

func TestQuizScoringHook_InvalidSettings_AreRefused(t *testing.T) {
	hook := scenarioHooks.NewScenarioStepQuizScoringHook()
	cases := []struct {
		name    string
		ctx     hooks.HookContext
		wantErr bool
	}{
		{"create, default settings", hooks.HookContext{HookType: hooks.BeforeCreate, NewEntity: &models.ScenarioStep{}}, false},
		{"create, partial with penalty 0", hooks.HookContext{HookType: hooks.BeforeCreate,
			NewEntity: &models.ScenarioStep{QuizScoringMode: models.QuizScoringPartial, QuizWrongPenalty: penaltyOf(0)}}, false},
		{"update, partial question floor", hooks.HookContext{HookType: hooks.BeforeUpdate,
			NewEntity: map[string]any{"quiz_scoring_mode": models.QuizScoringPartialQuestionFloor}}, false},
		{"create, unknown mode",hooks.HookContext{HookType: hooks.BeforeCreate,
			NewEntity: &models.ScenarioStep{QuizScoringMode: "bonus"}}, true},
		{"create, negative penalty", hooks.HookContext{HookType: hooks.BeforeCreate,
			NewEntity: &models.ScenarioStep{QuizWrongPenalty: penaltyOf(-0.5)}}, true},
		{"update, penalty 10", hooks.HookContext{HookType: hooks.BeforeUpdate,
			NewEntity: map[string]any{"quiz_wrong_penalty": 10.0}}, false},
		{"update, penalty above 10", hooks.HookContext{HookType: hooks.BeforeUpdate,
			NewEntity: map[string]any{"quiz_wrong_penalty": 10.5}}, true},
		{"update, unknown mode", hooks.HookContext{HookType: hooks.BeforeUpdate,
			NewEntity: map[string]any{"quiz_scoring_mode": "weighted"}}, true},
		{"update, unrelated fields", hooks.HookContext{HookType: hooks.BeforeUpdate,
			NewEntity: map[string]any{"title": "x"}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := tc.ctx
			err := hook.Execute(&ctx)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestScenarioContentProblems_InvalidQuizScoring_IsReported(t *testing.T) {
	scenario := &models.Scenario{Title: "t", Steps: []models.ScenarioStep{{
		Title: "q", StepType: "quiz", QuizScoringMode: "bonus",
		Questions: []models.ScenarioStepQuestion{{QuestionText: "?", QuestionType: "true_false", CorrectAnswer: "true"}},
	}}}
	problems := services.ScenarioContentProblems(scenario)
	assert.Len(t, problems, 1)
	assert.Contains(t, problems[0], "quiz_scoring_mode")
}

// A survey is never scored, so the scoring hook must let one through with its
// default (empty) scoring settings, on create and on an update that only
// touches the survey's own fields (#568).
func TestQuizScoringHook_SurveyWithDefaults_IsAccepted(t *testing.T) {
	hook := scenarioHooks.NewScenarioStepQuizScoringHook()

	create := hooks.HookContext{HookType: hooks.BeforeCreate,
		NewEntity: &models.ScenarioStep{Title: "Feedback", StepType: "survey"}}
	assert.NoError(t, hook.Execute(&create))

	update := hooks.HookContext{HookType: hooks.BeforeUpdate,
		NewEntity: map[string]any{"step_type": "survey", "show_immediate_feedback": false}}
	assert.NoError(t, hook.Execute(&update))
}
