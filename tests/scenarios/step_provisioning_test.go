package scenarios_test

import (
	"encoding/json"
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

func TestProvisionNextStep_ShortTimeout_RunsInlineAndReportsNothing(t *testing.T) {
	db := setupTestDB(t)
	session := twoStepSession(t, db, "sync-short", models.ScenarioStep{
		BackgroundScript:         "echo quick",
		BackgroundTimeoutSeconds: 5, // under the 15s async threshold
	})

	verifySvc := &bgTrackingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	result, err := sessionSvc.VerifyCurrentStep(session.ID)
	require.NoError(t, err)

	// next_step_provisioning means "async started, go poll". Setup that
	// finished inline has nothing for the client to wait on.
	assert.False(t, result.NextStepProvisioning)
	assert.Zero(t, result.ProvisioningTimeoutSeconds)
	assert.False(t, result.NextStepProvisioningFailed)

	// No wait: the synchronous branch has already run the script by now, and
	// the session never left "active".
	require.Len(t, verifySvc.execCalls, 1)
	assert.Equal(t, 5, verifySvc.execCalls[0].timeout)
	assert.Equal(t, "active", sessionStatus(t, db, session.ID))
}

func TestProvisionNextStep_SyncFailure_ReportsFailureAndKeepsTheAdvance(t *testing.T) {
	db := setupTestDB(t)
	session := twoStepSession(t, db, "sync-failure", models.ScenarioStep{
		BackgroundScript:         "echo will fail",
		BackgroundTimeoutSeconds: 5,
	})

	verifySvc := &bgTrackingVerificationService{execErr: fmt.Errorf("container unreachable")}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	result, err := sessionSvc.VerifyCurrentStep(session.ID)
	require.NoError(t, err, "the advance stands — the step is already completed and the flag burned")
	require.NotNil(t, result.NextStep)

	assert.True(t, result.NextStepProvisioningFailed,
		"a silent inline failure would leave the learner on a half-built level reading it as an impossible puzzle")
	assert.False(t, result.NextStepProvisioning, "nothing is running in the background to poll for")

	// The session stays usable: reprovision-step is the retry, not a restart.
	assert.Equal(t, "active", sessionStatus(t, db, session.ID))
}

func TestProvisionNextStep_SyncFailure_LeaksNoScriptOutput(t *testing.T) {
	db := setupTestDB(t)
	session := twoStepSession(t, db, "sync-failure-quiet", models.ScenarioStep{
		BackgroundScript:         "echo will fail",
		BackgroundTimeoutSeconds: 5,
	})

	// Background scripts hold flags and puzzle internals, so no part of the
	// failure may reach the learner's browser — only the boolean.
	secret := "flag{leaked-through-stderr}"
	verifySvc := &bgTrackingVerificationService{execErr: fmt.Errorf("script failed: %s", secret)}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	result, err := sessionSvc.VerifyCurrentStep(session.ID)
	require.NoError(t, err)

	payload, marshalErr := json.Marshal(result)
	require.NoError(t, marshalErr)
	assert.NotContains(t, string(payload), secret)
	assert.NotContains(t, string(payload), "script failed")
}

func TestProvisionNextStep_FlagDeployFailure_ReportsFailure(t *testing.T) {
	db := setupTestDB(t)
	session := twoStepSession(t, db, "flag-push-failure", models.ScenarioStep{
		HasFlag:  true,
		FlagPath: "/tmp/the_flag",
	})
	require.NoError(t, db.Create(&models.ScenarioFlag{
		SessionID: session.ID, StepOrder: 1, ExpectedFlag: "flag{never-lands}",
	}).Error)

	verifySvc := &bgTrackingVerificationService{pushFileErr: fmt.Errorf("container filesystem full")}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	result, err := sessionSvc.VerifyCurrentStep(session.ID)
	require.NoError(t, err)
	assert.True(t, result.NextStepProvisioningFailed,
		"a flag that never landed leaves the step unsolvable, exactly like a failed script")
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
	assert.Equal(t, 5, result.ProvisioningTimeoutSeconds,
		"the client derives its poll ceiling from the step's own timeout, not a constant")

	assert.Equal(t, "active", waitForSetupDone(t, db, session.ID))
	require.Len(t, verifySvc.execCalls, 1)
}

func TestProvisionNextStep_AsyncOnDefaultTimeout_ReportsThatTimeout(t *testing.T) {
	db := setupTestDB(t)
	session := twoStepSession(t, db, "async-default-timeout", models.ScenarioStep{
		BackgroundScript: "echo default budget",
	})

	verifySvc := &bgTrackingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	result, err := sessionSvc.VerifyCurrentStep(session.ID)
	require.NoError(t, err)
	assert.True(t, result.NextStepProvisioning)
	assert.Equal(t, 60, result.ProvisioningTimeoutSeconds)

	assert.Equal(t, "active", waitForSetupDone(t, db, session.ID))
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

func TestProvisionNextStep_AbandonedSession_NeverTouchesTheContainer(t *testing.T) {
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
	require.ErrorIs(t, err, services.ErrSessionNotActive)

	assert.Equal(t, "abandoned", sessionStatus(t, db, session.ID))
	assert.Len(t, verifySvc.execCalls, 0)
}

func TestCurrentStepProvisioningTimeout_OnlyWhileProvisioning(t *testing.T) {
	db := setupTestDB(t)
	session := twoStepSession(t, db, "info-timeout", models.ScenarioStep{
		BackgroundScript:         "echo long setup",
		BackgroundTimeoutSeconds: 120,
	})
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, &bgTrackingVerificationService{})

	// A client that reloads mid-provisioning never saw the advance response, so
	// session info has to carry the ceiling too.
	require.NoError(t, db.Model(&models.ScenarioSession{}).
		Where("id = ?", session.ID).
		Updates(map[string]any{"current_step": 1, "status": "provisioning"}).Error)

	var provisioning models.ScenarioSession
	require.NoError(t, db.First(&provisioning, "id = ?", session.ID).Error)
	assert.Equal(t, 120, sessionSvc.CurrentStepProvisioningTimeout(&provisioning))

	require.NoError(t, db.Model(&models.ScenarioSession{}).
		Where("id = ?", session.ID).Update("status", "active").Error)

	// Fresh struct: reloading into the previous one would keep its stale status.
	var active models.ScenarioSession
	require.NoError(t, db.First(&active, "id = ?", session.ID).Error)
	assert.Zero(t, sessionSvc.CurrentStepProvisioningTimeout(&active),
		"an active session is not waiting on anything")
}

// -----------------------------------------------------------------------------
// Session-status guard on the advance endpoints
//
// Step setup can now run mid-scenario, so a second browser tab submitting while
// the container is being rebuilt would race the provisioning. Every advance
// entry point refuses anything but an active session.
// -----------------------------------------------------------------------------

func TestAdvanceEndpoints_RejectSubmissionsWhileProvisioning(t *testing.T) {
	db := setupTestDB(t)

	// One provisioning session reused across the endpoints — none of them may
	// get as far as touching the container.
	session := twoStepSession(t, db, "guard-provisioning", models.ScenarioStep{
		HasFlag: true,
	})
	require.NoError(t, db.Create(&models.ScenarioFlag{
		SessionID: session.ID, StepOrder: 0, ExpectedFlag: "flag{guarded}",
	}).Error)
	require.NoError(t, db.Model(&models.ScenarioSession{}).
		Where("id = ?", session.ID).
		Updates(map[string]any{"status": "provisioning", "provisioning_phase": "step_setup"}).Error)

	verifySvc := &bgTrackingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{validateRes: true}, verifySvc)

	t.Run("verify", func(t *testing.T) {
		_, err := sessionSvc.VerifyCurrentStep(session.ID)
		require.ErrorIs(t, err, services.ErrSessionNotActive)
		assert.Contains(t, err.Error(), "still being prepared")
	})

	t.Run("submit flag", func(t *testing.T) {
		_, err := sessionSvc.SubmitFlag(session.ID, "flag{guarded}")
		require.ErrorIs(t, err, services.ErrSessionNotActive)
	})

	t.Run("submit quiz", func(t *testing.T) {
		_, err := sessionSvc.SubmitQuiz(session.ID, dto.SubmitQuizInput{
			Answers: map[uuid.UUID]string{uuid.New(): "whatever"},
		})
		require.ErrorIs(t, err, services.ErrSessionNotActive)
	})

	t.Run("reveal hint", func(t *testing.T) {
		_, err := sessionSvc.RevealHint(session.ID, 0, 1)
		require.ErrorIs(t, err, services.ErrSessionNotActive)
	})

	assert.Len(t, verifySvc.execCalls, 0, "no submission may reach the container while it is being rebuilt")
	assert.Len(t, verifySvc.pushFileCalls, 0)

	// The flag was not consumed — the learner can still submit it once setup lands.
	var flag models.ScenarioFlag
	require.NoError(t, db.First(&flag, "session_id = ?", session.ID).Error)
	assert.Equal(t, 0, flag.FlagAttempts)
}

func TestAdvanceEndpoints_AcceptSubmissionsOnceProvisioningClears(t *testing.T) {
	db := setupTestDB(t)
	session := twoStepSession(t, db, "guard-cleared", models.ScenarioStep{
		TextContent: "nothing to provision",
	})
	require.NoError(t, db.Model(&models.ScenarioSession{}).
		Where("id = ?", session.ID).
		Updates(map[string]any{"status": "provisioning", "provisioning_phase": "step_setup"}).Error)

	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, &bgTrackingVerificationService{})

	_, err := sessionSvc.VerifyCurrentStep(session.ID)
	require.ErrorIs(t, err, services.ErrSessionNotActive)

	// The async goroutine's success transition, replayed.
	require.NoError(t, db.Model(&models.ScenarioSession{}).
		Where("id = ?", session.ID).
		Updates(map[string]any{"status": "active", "provisioning_phase": ""}).Error)

	result, err := sessionSvc.VerifyCurrentStep(session.ID)
	require.NoError(t, err)
	assert.True(t, result.Passed)
	require.NotNil(t, result.NextStep)
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

func TestVerifyStep_FlagOnlyStep_DeploysInlineAndReportsNothing(t *testing.T) {
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

	// Placing a flag is one push with no script — it never goes async, so the
	// step is playable the moment the response lands.
	assert.False(t, result.NextStepProvisioning)
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
