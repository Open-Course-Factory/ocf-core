package scenarios_test

// Red-phase tests for step_type-aware verification (#283).
//
// These tests assert the EXPECTED branching behavior of
// ScenarioSessionService.VerifyCurrentStep once the dev agent extends it to
// dispatch on step_type:
//   - terminal → existing path (call verification service)
//   - flag     → reject and direct caller to /submit-flag
//   - info     → auto-complete and advance
//   - quiz     → reject and direct caller to /submit-quiz
//
// The existing terminal path must keep working unchanged. The new branches
// will fail in the current implementation because VerifyCurrentStep does not
// look at step.StepType — it only branches on step.HasFlag.

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

// TestVerifyStep_TerminalStep_CallsVerifyScript — sanity: when step_type is
// "terminal" the existing path still calls the verification service and
// advances on success. This guards against the dev agent breaking the happy
// path while adding branching.
func TestVerifyStep_TerminalStep_CallsVerifyScript(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db := freshTestDB(t)

	scenario := models.Scenario{
		Name: "verify-terminal", Title: "Verify Terminal", InstanceType: "ubuntu:22.04", CreatedByID: "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	step0 := models.ScenarioStep{
		ScenarioID:   scenario.ID,
		Order:        0,
		Title:        "Terminal step",
		StepType:     "terminal",
		VerifyScript: "#!/bin/bash\ntrue",
	}
	step1 := models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 1, Title: "Next", StepType: "terminal",
	}
	require.NoError(t, db.Create(&step0).Error)
	require.NoError(t, db.Create(&step1).Error)

	terminalID := "terminal-verify-terminal"
	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "student-1", CurrentStep: 0,
		Status: "active", StartedAt: time.Now(), TerminalSessionID: &terminalID,
	}
	require.NoError(t, db.Create(&session).Error)
	require.NoError(t, db.Create(&models.ScenarioStepProgress{
		SessionID: session.ID, StepOrder: 0, Status: "active",
	}).Error)
	require.NoError(t, db.Create(&models.ScenarioStepProgress{
		SessionID: session.ID, StepOrder: 1, Status: "locked",
	}).Error)

	verifySvc := &mockVerificationService{passed: true, output: "OK"}
	svc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	result, err := svc.VerifyCurrentStep(session.ID)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Passed, "terminal step must use verify script path")
	require.NotNil(t, result.NextStep)
	assert.Equal(t, 1, *result.NextStep)
}

// TestVerifyStep_FlagStep_RejectsVerifyAndPointsToSubmitFlag — calling verify
// on a step_type=flag step must error out and tell the caller to use
// /submit-flag instead. Today this happens implicitly via step.HasFlag, but
// once step_type is the source of truth the dispatch must key off it.
func TestVerifyStep_FlagStep_RejectsVerifyAndPointsToSubmitFlag(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db := freshTestDB(t)

	scenario := models.Scenario{
		Name: "verify-flag", Title: "Verify Flag Step", InstanceType: "ubuntu:22.04",
		FlagsEnabled: true, FlagSecret: "secret", CreatedByID: "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	step0 := models.ScenarioStep{
		ScenarioID: scenario.ID,
		Order:      0,
		Title:      "Flag step",
		StepType:   "flag",
		HasFlag:    true,
	}
	require.NoError(t, db.Create(&step0).Error)

	terminalID := "terminal-verify-flag"
	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "student-1", CurrentStep: 0,
		Status: "active", StartedAt: time.Now(), TerminalSessionID: &terminalID,
	}
	require.NoError(t, db.Create(&session).Error)
	require.NoError(t, db.Create(&models.ScenarioStepProgress{
		SessionID: session.ID, StepOrder: 0, Status: "active",
	}).Error)

	svc := services.NewScenarioSessionService(db, &mockFlagService{}, &mockVerificationService{})
	result, err := svc.VerifyCurrentStep(session.ID)

	require.Error(t, err, "flag step must reject /verify")
	assert.Nil(t, result)
	msg := strings.ToLower(err.Error())
	assert.True(t,
		strings.Contains(msg, "submit-flag") || strings.Contains(msg, "submit_flag") || strings.Contains(msg, "flag submission"),
		"verify-on-flag error should direct caller to submit-flag, got: %q", err.Error())
}

// TestVerifyStep_InfoStep_AutoCompletesAndAdvances — calling verify on an info
// step must auto-mark it completed and advance to the next step. Info steps
// have no verification script — clicking "next" is the verify equivalent.
func TestVerifyStep_InfoStep_AutoCompletesAndAdvances(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db := freshTestDB(t)

	scenario := models.Scenario{
		Name: "verify-info", Title: "Verify Info Step", InstanceType: "ubuntu:22.04", CreatedByID: "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	step0 := models.ScenarioStep{
		ScenarioID:  scenario.ID,
		Order:       0,
		Title:       "Info step",
		StepType:    "info",
		TextContent: "Read this and click next.",
	}
	step1 := models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 1, Title: "Next", StepType: "terminal",
	}
	require.NoError(t, db.Create(&step0).Error)
	require.NoError(t, db.Create(&step1).Error)

	terminalID := "terminal-verify-info"
	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "student-1", CurrentStep: 0,
		Status: "active", StartedAt: time.Now(), TerminalSessionID: &terminalID,
	}
	require.NoError(t, db.Create(&session).Error)
	require.NoError(t, db.Create(&models.ScenarioStepProgress{
		SessionID: session.ID, StepOrder: 0, Status: "active",
	}).Error)
	require.NoError(t, db.Create(&models.ScenarioStepProgress{
		SessionID: session.ID, StepOrder: 1, Status: "locked",
	}).Error)

	svc := services.NewScenarioSessionService(db, &mockFlagService{}, &mockVerificationService{})
	result, err := svc.VerifyCurrentStep(session.ID)

	require.NoError(t, err, "info step must auto-pass on /verify")
	require.NotNil(t, result)
	assert.True(t, result.Passed, "info step must report passed=true")
	require.NotNil(t, result.NextStep)
	assert.Equal(t, 1, *result.NextStep, "info step must advance to step+1")

	// Step 0 must now be completed; step 1 must be active.
	var sp0, sp1 models.ScenarioStepProgress
	require.NoError(t, db.First(&sp0, "session_id = ? AND step_order = ?", session.ID, 0).Error)
	require.NoError(t, db.First(&sp1, "session_id = ? AND step_order = ?", session.ID, 1).Error)
	assert.Equal(t, "completed", sp0.Status, "info step must be marked completed after verify")
	assert.Equal(t, "active", sp1.Status, "next step must be unlocked after info verify")
}

// TestVerifyStep_QuizStep_RejectsVerifyAndPointsToSubmitQuiz — calling verify
// on a step_type=quiz step must error out and direct the caller to /submit-quiz.
func TestVerifyStep_QuizStep_RejectsVerifyAndPointsToSubmitQuiz(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db := freshTestDB(t)

	scenario := models.Scenario{
		Name: "verify-quiz", Title: "Verify Quiz Step", InstanceType: "ubuntu:22.04", CreatedByID: "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	step0 := models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 0, Title: "Quiz step", StepType: "quiz",
	}
	require.NoError(t, db.Create(&step0).Error)
	q := models.ScenarioStepQuestion{
		StepID: step0.ID, Order: 1, QuestionText: "Q?",
		QuestionType: "free_text", CorrectAnswer: "answer",
	}
	require.NoError(t, db.Create(&q).Error)

	terminalID := "terminal-verify-quiz"
	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "student-1", CurrentStep: 0,
		Status: "active", StartedAt: time.Now(), TerminalSessionID: &terminalID,
	}
	require.NoError(t, db.Create(&session).Error)
	require.NoError(t, db.Create(&models.ScenarioStepProgress{
		SessionID: session.ID, StepOrder: 0, Status: "active",
	}).Error)

	svc := services.NewScenarioSessionService(db, &mockFlagService{}, &mockVerificationService{})
	result, err := svc.VerifyCurrentStep(session.ID)

	require.Error(t, err, "quiz step must reject /verify")
	assert.Nil(t, result)
	msg := strings.ToLower(err.Error())
	assert.True(t,
		strings.Contains(msg, "submit-quiz") || strings.Contains(msg, "submit_quiz") || strings.Contains(msg, "quiz submission"),
		"verify-on-quiz error should direct caller to submit-quiz, got: %q", err.Error())
}
