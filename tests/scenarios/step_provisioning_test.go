package scenarios_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

// Per-step provisioning engine (B1): background-script timeouts, the
// sync-versus-async rule, next_step_provisioning on the advance responses,
// the reprovision endpoint and the stuck-provisioning reaper.
//
// The engine's timeout resolution is unexported, so every assertion here reads
// it back off the timeout the container exec actually received.

// twoStepSession builds a scenario whose step 0 auto-passes verification and
// whose step 1 carries the provisioning under test, plus a live session parked
// on step 0. It returns the session so callers can advance it.
func twoStepSession(t *testing.T, db *gorm.DB, name string, nextStep models.ScenarioStep) *models.ScenarioSession {
	t.Helper()

	scenario := models.Scenario{
		Name:         name,
		Title:        name,
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	step0 := models.ScenarioStep{ScenarioID: scenario.ID, Order: 0, Title: "Step 1"}
	require.NoError(t, db.Create(&step0).Error)

	nextStep.ScenarioID = scenario.ID
	nextStep.Order = 1
	if nextStep.Title == "" {
		nextStep.Title = "Step 2"
	}
	require.NoError(t, db.Create(&nextStep).Error)

	terminalID := "terminal-" + name
	session := models.ScenarioSession{
		ScenarioID:        scenario.ID,
		UserID:            "student-" + name,
		CurrentStep:       0,
		Status:            "active",
		StartedAt:         time.Now(),
		TerminalSessionID: &terminalID,
	}
	require.NoError(t, db.Create(&session).Error)

	require.NoError(t, db.Create(&models.ScenarioStepProgress{SessionID: session.ID, StepOrder: 0, Status: "active"}).Error)
	require.NoError(t, db.Create(&models.ScenarioStepProgress{SessionID: session.ID, StepOrder: 1, Status: "locked"}).Error)

	return &session
}

func sessionStatus(t *testing.T, db *gorm.DB, sessionID any) string {
	t.Helper()
	var s models.ScenarioSession
	require.NoError(t, db.First(&s, "id = ?", sessionID).Error)
	return s.Status
}

// -----------------------------------------------------------------------------
// Timeout resolution
// -----------------------------------------------------------------------------

func TestBackgroundScript_LaterStep_UsesRaisedDefaultTimeout(t *testing.T) {
	db := setupTestDB(t)
	session := twoStepSession(t, db, "timeout-default", models.ScenarioStep{
		BackgroundScript: "echo provisioning",
	})

	verifySvc := &bgTrackingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	_, err := sessionSvc.VerifyCurrentStep(session.ID)
	require.NoError(t, err)
	require.Equal(t, "active", waitForSetupDone(t, db, session.ID))

	require.Len(t, verifySvc.execCalls, 1)
	assert.Equal(t, 60, verifySvc.execCalls[0].timeout,
		"steps past step 0 get the raised 60s default, not the old 30s")
}

func TestBackgroundScript_PerStepTimeout_OverridesDefault(t *testing.T) {
	db := setupTestDB(t)
	session := twoStepSession(t, db, "timeout-override", models.ScenarioStep{
		BackgroundScript:         "echo slow provisioning",
		BackgroundTimeoutSeconds: 120,
	})

	verifySvc := &bgTrackingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	_, err := sessionSvc.VerifyCurrentStep(session.ID)
	require.NoError(t, err)
	require.Equal(t, "active", waitForSetupDone(t, db, session.ID))

	require.Len(t, verifySvc.execCalls, 1)
	assert.Equal(t, 120, verifySvc.execCalls[0].timeout)
}

func TestBackgroundScript_Step0_PerStepTimeoutOverridesInitialSetupBudget(t *testing.T) {
	db := setupTestDB(t)

	scenario := models.Scenario{
		Name:         "timeout-step0-override",
		Title:        "Step 0 timeout override",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)
	require.NoError(t, db.Create(&models.ScenarioStep{
		ScenarioID:               scenario.ID,
		Order:                    0,
		Title:                    "Step 1",
		BackgroundScript:         "echo setup",
		BackgroundTimeoutSeconds: 45,
	}).Error)

	verifySvc := &bgTrackingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	session, err := sessionSvc.StartScenario("student-1", scenario.ID, "terminal-step0-override")
	require.NoError(t, err)
	require.Equal(t, "active", waitForSetupDone(t, db, session.ID))

	require.Len(t, verifySvc.execCalls, 1)
	assert.Equal(t, 45, verifySvc.execCalls[0].timeout,
		"an explicit per-step timeout wins even over the 300s initial-setup budget")
}

// -----------------------------------------------------------------------------
// Sync versus async
// -----------------------------------------------------------------------------

func TestProvisionNextStep_ShortTimeout_RunsBeforeTheResponseReturns(t *testing.T) {
	db := setupTestDB(t)
	session := twoStepSession(t, db, "sync-short", models.ScenarioStep{
		BackgroundScript:         "echo quick",
		BackgroundTimeoutSeconds: 5, // under the 15s async threshold
	})

	verifySvc := &bgTrackingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	result, err := sessionSvc.VerifyCurrentStep(session.ID)
	require.NoError(t, err)
	assert.True(t, result.NextStepProvisioning)

	// No wait: a synchronous branch has already run the script by now, and the
	// session never left "active".
	require.Len(t, verifySvc.execCalls, 1)
	assert.Equal(t, 5, verifySvc.execCalls[0].timeout)
	assert.Equal(t, "active", sessionStatus(t, db, session.ID))
}

func TestProvisionNextStep_BackgroundAsyncFlag_RunsInBackgroundDespiteShortTimeout(t *testing.T) {
	db := setupTestDB(t)
	session := twoStepSession(t, db, "async-flag", models.ScenarioStep{
		BackgroundScript:         "echo quick but async",
		BackgroundTimeoutSeconds: 5,
		BackgroundAsync:          true,
	})

	verifySvc := &bgTrackingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	result, err := sessionSvc.VerifyCurrentStep(session.ID)
	require.NoError(t, err)
	assert.True(t, result.NextStepProvisioning)

	assert.Equal(t, "active", waitForSetupDone(t, db, session.ID))
	require.Len(t, verifySvc.execCalls, 1)
}

func TestProvisionNextStep_AsyncFailure_MarksSetupFailedAndKeepsTheTerminal(t *testing.T) {
	db := setupTestDB(t)
	session := twoStepSession(t, db, "async-failure", models.ScenarioStep{
		BackgroundScript: "echo will fail",
		BackgroundAsync:  true,
	})

	verifySvc := &bgTrackingVerificationService{execErr: fmt.Errorf("container unreachable")}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	stopped := 0
	sessionSvc.SetTerminalStopFunc(func(string) error {
		stopped++
		return nil
	})

	result, err := sessionSvc.VerifyCurrentStep(session.ID)
	require.NoError(t, err, "a provisioning failure never fails the advance — the step is already completed")
	require.NotNil(t, result.NextStep)

	assert.Equal(t, "setup_failed", waitForSetupDone(t, db, session.ID))
	assert.Equal(t, 0, stopped,
		"a mid-scenario failure must leave the learner's shell alive so they can reprovision")

	// The advance itself stands: step 0 stays completed and the session sits on step 1.
	var progress models.ScenarioStepProgress
	require.NoError(t, db.First(&progress, "session_id = ? AND step_order = 0", session.ID).Error)
	assert.Equal(t, "completed", progress.Status)
}

func TestProvisionNextStep_AbandonedMidAdvance_DoesNotResurrectTheSession(t *testing.T) {
	db := setupTestDB(t)
	session := twoStepSession(t, db, "async-abandoned", models.ScenarioStep{
		BackgroundScript: "echo provisioning",
		BackgroundAsync:  true,
	})
	require.NoError(t, db.Model(&models.ScenarioSession{}).
		Where("id = ?", session.ID).Update("status", "abandoned").Error)

	verifySvc := &bgTrackingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	_, err := sessionSvc.VerifyCurrentStep(session.ID)
	require.NoError(t, err)

	assert.Equal(t, "abandoned", sessionStatus(t, db, session.ID))
	assert.Len(t, verifySvc.execCalls, 0, "no goroutine is spawned for a session that is gone")
}

// -----------------------------------------------------------------------------
// next_step_provisioning on the three advance responses
// -----------------------------------------------------------------------------

func TestVerifyStep_NextStepProvisioning_FalseWhenNextStepNeedsNothing(t *testing.T) {
	db := setupTestDB(t)
	session := twoStepSession(t, db, "no-provisioning", models.ScenarioStep{
		TextContent: "just read this",
	})

	verifySvc := &bgTrackingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	result, err := sessionSvc.VerifyCurrentStep(session.ID)
	require.NoError(t, err)
	assert.False(t, result.NextStepProvisioning)
	assert.Len(t, verifySvc.execCalls, 0)
	assert.Len(t, verifySvc.pushFileCalls, 0)
}

func TestVerifyStep_NextStepProvisioning_TrueForFlagOnlyStep(t *testing.T) {
	db := setupTestDB(t)
	session := twoStepSession(t, db, "flag-only", models.ScenarioStep{
		HasFlag:  true,
		FlagPath: "/tmp/the_flag",
	})
	require.NoError(t, db.Create(&models.ScenarioFlag{
		SessionID:    session.ID,
		StepOrder:    1,
		ExpectedFlag: "flag{next}",
	}).Error)

	verifySvc := &bgTrackingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	result, err := sessionSvc.VerifyCurrentStep(session.ID)
	require.NoError(t, err)
	assert.True(t, result.NextStepProvisioning, "a flag to place is provisioning work too")

	// Deploying a flag has no script, so it stays on the request path.
	assert.Equal(t, "active", sessionStatus(t, db, session.ID))
	require.Len(t, verifySvc.pushFileCalls, 1)
	assert.Equal(t, "/tmp/the_flag", verifySvc.pushFileCalls[0].targetPath)
	assert.Equal(t, "flag{next}\n", verifySvc.pushFileCalls[0].content)
}

func TestSubmitQuiz_NextStepProvisioning_ReportsTheNextStepsScript(t *testing.T) {
	db := setupTestDB(t)

	scenario := models.Scenario{
		Name:         "quiz-provisioning",
		Title:        "Quiz provisioning",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	quizStep := models.ScenarioStep{ScenarioID: scenario.ID, Order: 0, Title: "Quiz", StepType: "quiz"}
	require.NoError(t, db.Create(&quizStep).Error)
	require.NoError(t, db.Create(&models.ScenarioStep{
		ScenarioID:       scenario.ID,
		Order:            1,
		Title:            "After the quiz",
		BackgroundScript: "echo post-quiz setup",
	}).Error)

	question := models.ScenarioStepQuestion{
		StepID:        quizStep.ID,
		Order:         1,
		QuestionText:  "2 + 2 ?",
		QuestionType:  "single_choice",
		CorrectAnswer: "4",
	}
	require.NoError(t, db.Create(&question).Error)

	terminalID := "terminal-quiz-provisioning"
	session := models.ScenarioSession{
		ScenarioID:        scenario.ID,
		UserID:            "student-quiz",
		CurrentStep:       0,
		Status:            "active",
		StartedAt:         time.Now(),
		TerminalSessionID: &terminalID,
	}
	require.NoError(t, db.Create(&session).Error)
	require.NoError(t, db.Create(&models.ScenarioStepProgress{SessionID: session.ID, StepOrder: 0, Status: "active"}).Error)
	require.NoError(t, db.Create(&models.ScenarioStepProgress{SessionID: session.ID, StepOrder: 1, Status: "locked"}).Error)

	verifySvc := &bgTrackingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	result, err := sessionSvc.SubmitQuiz(session.ID, dto.SubmitQuizInput{
		Answers: map[uuid.UUID]string{question.ID: "4"},
	})
	require.NoError(t, err)
	assert.True(t, result.NextStepProvisioning)
	assert.Equal(t, "active", waitForSetupDone(t, db, session.ID))
	require.Len(t, verifySvc.execCalls, 1)
}

func TestGetCurrentStep_WhileProvisioning_ReportsTheRealStepOrder(t *testing.T) {
	db := setupTestDB(t)
	session := twoStepSession(t, db, "current-step-provisioning", models.ScenarioStep{
		BackgroundScript: "echo long setup",
		BackgroundAsync:  true,
	})

	// Freeze the session mid-provisioning on step 1, the way an async advance
	// leaves it: a hardcoded step 0 here would send the panel back to the start.
	require.NoError(t, db.Model(&models.ScenarioSession{}).
		Where("id = ?", session.ID).
		Updates(map[string]any{"current_step": 1, "status": "provisioning", "provisioning_phase": "step_setup"}).Error)

	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, &bgTrackingVerificationService{})

	step, err := sessionSvc.GetCurrentStep(session.ID)
	require.NoError(t, err)
	assert.Equal(t, "provisioning", step.Status)
	assert.Equal(t, 1, step.StepOrder)
	assert.Equal(t, 2, step.TotalSteps)
}
