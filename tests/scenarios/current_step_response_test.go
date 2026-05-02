package scenarios_test

// Red-phase tests for step_type-aware GetCurrentStep responses (#283).
//
// These tests assert the EXPECTED public shape of CurrentStepResponse once the
// dev agent extends the DTO and ScenarioSessionService.GetCurrentStep to
// branch on step_type. They will fail to compile until:
//   - dto.CurrentStepResponse gains StepType, TextContent, and Questions fields
//   - dto.CurrentStepQuestion DTO is defined (sanitized — no CorrectAnswer/Explanation)
//   - GetCurrentStep populates StepType (defaulting empty → "terminal"),
//     populates TextContent for info steps, and populates Questions for quiz steps.

import (
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

// setupSessionWithStepType builds a fresh session at step 0 with one step of
// the given step_type and step progress = active. Returns the session UUID and
// the underlying step row so individual tests can tweak fields when needed.
func setupSessionWithStepType(t *testing.T, stepType string, mutate func(*models.ScenarioStep)) (uuid.UUID, models.ScenarioStep) {
	t.Helper()
	db := freshTestDB(t)

	scenario := models.Scenario{
		Name:         "current-step-" + stepType,
		Title:        "Current Step " + stepType,
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	step := models.ScenarioStep{
		ScenarioID:  scenario.ID,
		Order:       0,
		Title:       "First Step",
		StepType:    stepType,
		TextContent: "short summary",
	}
	if mutate != nil {
		mutate(&step)
	}
	require.NoError(t, db.Create(&step).Error)

	session := models.ScenarioSession{
		ScenarioID:  scenario.ID,
		UserID:      "student-1",
		CurrentStep: 0,
		Status:      "active",
		StartedAt:   time.Now(),
	}
	require.NoError(t, db.Create(&session).Error)

	progress := models.ScenarioStepProgress{
		SessionID: session.ID,
		StepOrder: 0,
		Status:    "active",
	}
	require.NoError(t, db.Create(&progress).Error)

	return session.ID, step
}

// TestGetCurrentStep_TerminalStep_IncludesStepType — when step is
// step_type=terminal, the response must include `step_type: "terminal"` so the
// frontend can pick the correct renderer.
func TestGetCurrentStep_TerminalStep_IncludesStepType(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	sessionID, _ := setupSessionWithStepType(t, "terminal", nil)

	svc := services.NewScenarioSessionService(sharedTestDB, &mockFlagService{}, &mockVerificationService{})
	response, err := svc.GetCurrentStep(sessionID)

	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Equal(t, "terminal", response.StepType,
		"terminal step must expose StepType=\"terminal\" in CurrentStepResponse")
}

// TestGetCurrentStep_LegacyStepWithoutType_DefaultsToTerminal — when a legacy
// step has empty step_type (older rows pre-migration default), GetCurrentStep
// must default the response to "terminal" so the frontend never sees an empty
// type.
func TestGetCurrentStep_LegacyStepWithoutType_DefaultsToTerminal(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	// Create the step with step_type="terminal" first, then force it to empty
	// directly via SQL — GORM's default tag would otherwise re-fill it on insert.
	sessionID, step := setupSessionWithStepType(t, "terminal", nil)
	require.NoError(t, sharedTestDB.Model(&models.ScenarioStep{}).
		Where("id = ?", step.ID).
		Update("step_type", "").Error)

	svc := services.NewScenarioSessionService(sharedTestDB, &mockFlagService{}, &mockVerificationService{})
	response, err := svc.GetCurrentStep(sessionID)

	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Equal(t, "terminal", response.StepType,
		"legacy step with empty step_type must default to \"terminal\" in response")
}

// TestGetCurrentStep_InfoStep_IncludesTextContent — when step_type=info, the
// response must include the full markdown via TextContent (separate from the
// existing short Text). The frontend renders the full content as a card.
func TestGetCurrentStep_InfoStep_IncludesTextContent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	const fullMarkdown = "# Welcome\n\nThis is a long-form info card.\n\n- bullet 1\n- bullet 2\n"

	sessionID, _ := setupSessionWithStepType(t, "info", func(step *models.ScenarioStep) {
		step.TextContent = fullMarkdown
	})

	svc := services.NewScenarioSessionService(sharedTestDB, &mockFlagService{}, &mockVerificationService{})
	response, err := svc.GetCurrentStep(sessionID)

	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Equal(t, "info", response.StepType, "info step must expose StepType=\"info\"")
	assert.Equal(t, fullMarkdown, response.TextContent,
		"info step must expose the full markdown TextContent in CurrentStepResponse")
}

// TestGetCurrentStep_QuizStep_IncludesSanitizedQuestions — when step_type=quiz,
// the response must include a `questions` array. Each question entry must have
// id, order, question_text, question_type, options — but MUST NEVER include
// correct_answer or explanation (those would leak the answer to the student).
func TestGetCurrentStep_QuizStep_IncludesSanitizedQuestions(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db := freshTestDB(t)

	scenario := models.Scenario{
		Name: "quiz-current", Title: "Quiz Current", InstanceType: "ubuntu:22.04", CreatedByID: "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	step := models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 0, Title: "Quiz Step", StepType: "quiz",
	}
	require.NoError(t, db.Create(&step).Error)

	q1 := models.ScenarioStepQuestion{
		StepID:        step.ID,
		Order:         1,
		QuestionText:  "Which command lists files?",
		QuestionType:  "multiple_choice",
		Options:       `["ls", "cd", "rm"]`,
		CorrectAnswer: "ls",
		Explanation:   "ls lists files.",
		Points:        1,
	}
	require.NoError(t, db.Create(&q1).Error)
	q2 := models.ScenarioStepQuestion{
		StepID:        step.ID,
		Order:         2,
		QuestionText:  "True or false: /dev/null discards data",
		QuestionType:  "true_false",
		CorrectAnswer: "true",
		Explanation:   "Correct.",
		Points:        1,
	}
	require.NoError(t, db.Create(&q2).Error)

	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "student-1", CurrentStep: 0,
		Status: "active", StartedAt: time.Now(),
	}
	require.NoError(t, db.Create(&session).Error)
	require.NoError(t, db.Create(&models.ScenarioStepProgress{
		SessionID: session.ID, StepOrder: 0, Status: "active",
	}).Error)

	svc := services.NewScenarioSessionService(db, &mockFlagService{}, &mockVerificationService{})
	response, err := svc.GetCurrentStep(session.ID)

	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Equal(t, "quiz", response.StepType, "quiz step must expose StepType=\"quiz\"")
	require.Len(t, response.Questions, 2,
		"quiz step must expose exactly the questions belonging to the step")

	// Identify questions by order so we can assert per-field shape regardless
	// of slice order.
	byOrder := make(map[int]int) // map[order]index
	for i := range response.Questions {
		byOrder[response.Questions[i].Order] = i
	}

	require.Contains(t, byOrder, 1, "expected question with order=1")
	q1got := response.Questions[byOrder[1]]
	assert.Equal(t, q1.ID, q1got.ID)
	assert.Equal(t, "Which command lists files?", q1got.QuestionText)
	assert.Equal(t, "multiple_choice", q1got.QuestionType)
	assert.Equal(t, `["ls", "cd", "rm"]`, q1got.Options)

	require.Contains(t, byOrder, 2, "expected question with order=2")
	q2got := response.Questions[byOrder[2]]
	assert.Equal(t, q2.ID, q2got.ID)
	assert.Equal(t, "true_false", q2got.QuestionType)

	// CRITICAL: assert no field on the public CurrentStepQuestion DTO leaks the
	// correct_answer or explanation. We do this with reflect to catch any
	// future mistake of adding such a field directly to the DTO.
	stShape := response.Questions[0]
	rt := reflect.TypeOf(stShape)
	for i := 0; i < rt.NumField(); i++ {
		name := rt.Field(i).Name
		assert.NotEqual(t, "CorrectAnswer", name,
			"CurrentStepQuestion DTO must NOT expose CorrectAnswer — that leaks the answer")
		assert.NotEqual(t, "Explanation", name,
			"CurrentStepQuestion DTO must NOT expose Explanation — explanations are revealed only after submission")
	}
}

// TestGetCurrentStep_QuizStep_IncludesShowImmediateFeedback verifies that the
// per-step show_immediate_feedback flag flows through GetCurrentStep so the
// player can decide whether to reveal correct/explanation after each question.
// Defaults to false (exam-safe); trainer opts in for learning quizzes.
func TestGetCurrentStep_QuizStep_IncludesShowImmediateFeedback(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db := freshTestDB(t)

	scenario := models.Scenario{
		Name: "quiz-feedback", Title: "Quiz Feedback", InstanceType: "ubuntu:22.04", CreatedByID: "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	step := models.ScenarioStep{
		ScenarioID:            scenario.ID,
		Order:                 0,
		Title:                 "Quiz Step",
		StepType:              "quiz",
		ShowImmediateFeedback: true,
	}
	require.NoError(t, db.Create(&step).Error)

	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "student-1", CurrentStep: 0,
		Status: "active", StartedAt: time.Now(),
	}
	require.NoError(t, db.Create(&session).Error)
	require.NoError(t, db.Create(&models.ScenarioStepProgress{
		SessionID: session.ID, StepOrder: 0, Status: "active",
	}).Error)

	svc := services.NewScenarioSessionService(db, &mockFlagService{}, &mockVerificationService{})
	response, err := svc.GetCurrentStep(session.ID)

	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Equal(t, "quiz", response.StepType, "quiz step must expose StepType=\"quiz\"")
	assert.True(t, response.ShowImmediateFeedback,
		"CurrentStepResponse must expose show_immediate_feedback=true when the step opted in")
}

// TestGetCurrentStep_TerminalStep_OmitsQuestions — terminal/flag/info steps
// must not include a populated Questions array (keep payload tight + avoid
// leaking unrelated rows if a quiz happens to share the step).
func TestGetCurrentStep_TerminalStep_OmitsQuestions(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	sessionID, _ := setupSessionWithStepType(t, "terminal", nil)

	svc := services.NewScenarioSessionService(sharedTestDB, &mockFlagService{}, &mockVerificationService{})
	response, err := svc.GetCurrentStep(sessionID)

	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Empty(t, response.Questions,
		"terminal step must not populate Questions in CurrentStepResponse")
}
