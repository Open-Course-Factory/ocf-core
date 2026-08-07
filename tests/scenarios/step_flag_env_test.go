package scenarios_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

// Challenge flags reach a step's background script through the process
// environment, never through argv or a file. tt-backend forwards the map to
// Incus, so values land in /proc/PID/environ (readable by the process owner
// alone); argv is world-readable inside the container and a file is worse.
//
// The contract these tests pin: a step's script sees its OWN flag and nothing
// else, and a scenario without flags sees no variable at all.

const flagEnvVar = "OCF_FLAG_CURRENT"

// flaggedTwoStepSession builds a flags-enabled scenario whose step 1 carries a
// background script, with a distinct flag generated per step.
func flaggedTwoStepSession(t *testing.T, db *gorm.DB, name string, nextStep models.ScenarioStep) *models.ScenarioSession {
	t.Helper()

	scenario := models.Scenario{
		Name:         name,
		Title:        name,
		InstanceType: "ubuntu:22.04",
		FlagsEnabled: true,
		FlagSecret:   "secret",
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	require.NoError(t, db.Create(&models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 0, Title: "Step 1", HasFlag: true,
	}).Error)
	nextStep.ScenarioID = scenario.ID
	nextStep.Order = 1
	nextStep.Title = "Step 2"
	nextStep.HasFlag = true
	require.NoError(t, db.Create(&nextStep).Error)

	terminalID := "terminal-" + name
	session := models.ScenarioSession{
		ScenarioID:        scenario.ID,
		UserID:            "student-" + name,
		CurrentStep:       0,
		Status:            "active",
		StartedAt:         db.NowFunc(),
		TerminalSessionID: &terminalID,
	}
	require.NoError(t, db.Create(&session).Error)

	require.NoError(t, db.Create(&models.ScenarioStepProgress{SessionID: session.ID, StepOrder: 0, Status: "active"}).Error)
	require.NoError(t, db.Create(&models.ScenarioStepProgress{SessionID: session.ID, StepOrder: 1, Status: "locked"}).Error)

	// One distinct flag per step, so a test can tell "the right flag" from
	// "a flag".
	require.NoError(t, db.Create(&models.ScenarioFlag{
		SessionID: session.ID, StepOrder: 0, ExpectedFlag: "flag{step-zero}",
	}).Error)
	require.NoError(t, db.Create(&models.ScenarioFlag{
		SessionID: session.ID, StepOrder: 1, ExpectedFlag: "flag{step-one}",
	}).Error)

	return &session
}

func TestBackgroundScript_FlagsEnabled_ReceivesOnlyItsOwnFlag(t *testing.T) {
	db := setupTestDB(t)
	session := flaggedTwoStepSession(t, db, "flag-env-own", models.ScenarioStep{
		BackgroundScript:         "echo setting up level one",
		BackgroundTimeoutSeconds: 5, // inline, so the exec has happened on return
	})

	verifySvc := &bgTrackingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{validateRes: true}, verifySvc)

	_, err := sessionSvc.SubmitFlag(session.ID, "flag{step-zero}")
	require.NoError(t, err)

	require.Len(t, verifySvc.execCalls, 1)
	assert.Equal(t, map[string]string{flagEnvVar: "flag{step-one}"}, verifySvc.execCalls[0].env,
		"the step's script gets its own flag, and only that")
}

func TestBackgroundScript_NeverCarriesAnotherStepsFlag(t *testing.T) {
	db := setupTestDB(t)
	session := flaggedTwoStepSession(t, db, "flag-env-isolation", models.ScenarioStep{
		BackgroundScript:         "echo setting up level one",
		BackgroundTimeoutSeconds: 5,
	})

	verifySvc := &bgTrackingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{validateRes: true}, verifySvc)

	_, err := sessionSvc.SubmitFlag(session.ID, "flag{step-zero}")
	require.NoError(t, err)

	// Levels 7 and 9 hold NOPASSWD sudo by design, so a learner can read any
	// process environment. Whatever they find there must not shortcut the rest
	// of the run.
	require.Len(t, verifySvc.execCalls, 1)
	for key, value := range verifySvc.execCalls[0].env {
		assert.NotEqual(t, "flag{step-zero}", value,
			"key %q leaked another step's flag into this step's environment", key)
	}
	assert.Len(t, verifySvc.execCalls[0].env, 1, "exactly one variable, no bulk flag set")
}

func TestBackgroundScript_FlagsDisabled_SendsNoEnvAtAll(t *testing.T) {
	db := setupTestDB(t)
	// twoStepSession builds a scenario with FlagsEnabled false.
	session := twoStepSession(t, db, "flag-env-absent", models.ScenarioStep{
		BackgroundScript:         "echo no flags here",
		BackgroundTimeoutSeconds: 5,
	})

	verifySvc := &bgTrackingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	_, err := sessionSvc.VerifyCurrentStep(session.ID)
	require.NoError(t, err)

	require.Len(t, verifySvc.execCalls, 1)
	assert.Nil(t, verifySvc.execCalls[0].env,
		"a scenario without flags must send no env — an empty OCF_FLAG_CURRENT would be a visible, confusing difference")
}

func TestBackgroundScript_FlagsEnabledButStepHasNoFlag_SendsNoEnv(t *testing.T) {
	db := setupTestDB(t)

	scenario := models.Scenario{
		Name:         "flag-env-stepless",
		Title:        "Flag env, flagless step",
		InstanceType: "ubuntu:22.04",
		FlagsEnabled: true,
		FlagSecret:   "secret",
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)
	require.NoError(t, db.Create(&models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 0, Title: "Step 1", HasFlag: true,
	}).Error)
	require.NoError(t, db.Create(&models.ScenarioStep{
		ScenarioID:               scenario.ID,
		Order:                    1,
		Title:                    "Step 2",
		BackgroundScript:         "echo no flag on this step",
		BackgroundTimeoutSeconds: 5,
	}).Error)

	terminalID := "terminal-flag-env-stepless"
	session := models.ScenarioSession{
		ScenarioID:        scenario.ID,
		UserID:            "student-stepless",
		CurrentStep:       0,
		Status:            "active",
		StartedAt:         db.NowFunc(),
		TerminalSessionID: &terminalID,
	}
	require.NoError(t, db.Create(&session).Error)
	require.NoError(t, db.Create(&models.ScenarioStepProgress{SessionID: session.ID, StepOrder: 0, Status: "active"}).Error)
	require.NoError(t, db.Create(&models.ScenarioStepProgress{SessionID: session.ID, StepOrder: 1, Status: "locked"}).Error)
	require.NoError(t, db.Create(&models.ScenarioFlag{
		SessionID: session.ID, StepOrder: 0, ExpectedFlag: "flag{step-zero}",
	}).Error)

	verifySvc := &bgTrackingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{validateRes: true}, verifySvc)

	_, err := sessionSvc.SubmitFlag(session.ID, "flag{step-zero}")
	require.NoError(t, err)

	require.Len(t, verifySvc.execCalls, 1)
	assert.Nil(t, verifySvc.execCalls[0].env,
		"a step with no flag of its own gets nothing, not the previous step's")
}

func TestBackgroundScript_LargeScript_CarriesFlagOnRunButNotOnCleanup(t *testing.T) {
	db := setupTestDB(t)

	// Over 4000 bytes, so the script is pushed to a temp file and executed from
	// disk — a second code path, plus a cleanup exec that must stay bare.
	large := "#!/bin/bash\n" + strings.Repeat("echo padding to exceed the inline exec limit\n", 100)
	require.Greater(t, len(large), 4000)

	session := flaggedTwoStepSession(t, db, "flag-env-large", models.ScenarioStep{
		BackgroundScript:         large,
		BackgroundTimeoutSeconds: 5,
	})

	verifySvc := &bgTrackingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{validateRes: true}, verifySvc)

	_, err := sessionSvc.SubmitFlag(session.ID, "flag{step-zero}")
	require.NoError(t, err)

	require.Len(t, verifySvc.execCalls, 2, "run then cleanup")
	assert.Equal(t, map[string]string{flagEnvVar: "flag{step-one}"}, verifySvc.execCalls[0].env)
	assert.Nil(t, verifySvc.execCalls[1].env, "the cleanup rm has no business holding a flag")

	// The flag travels in the environment, never inside the script written to
	// disk. (The step's own flag *file* is a separate, deliberate push — see
	// the crash_traps test below for the scenarios that suppress it.)
	scriptPush := findPushTo(t, verifySvc, "/tmp/.ocf_bg_1.sh")
	assert.NotContains(t, scriptPush.content, "flag{step-one}")
}

// findPushTo returns the single PushFile call aimed at targetPath.
func findPushTo(t *testing.T, svc *bgTrackingVerificationService, targetPath string) pushFileCall {
	t.Helper()
	for _, call := range svc.pushFileCalls {
		if call.targetPath == targetPath {
			return call
		}
	}
	t.Fatalf("no PushFile call targeting %s", targetPath)
	return pushFileCall{}
}

// crash_traps scenarios are the ones the env channel exists for: their flags
// must exist nowhere on disk, so the step script gets the value through the
// environment and no flag file is ever written.
func TestBackgroundScript_CrashTraps_PassesFlagInEnvAndWritesNoFlagFile(t *testing.T) {
	db := setupTestDB(t)
	session := flaggedTwoStepSession(t, db, "flag-env-crash-traps", models.ScenarioStep{
		BackgroundScript:         "echo level one uses the env",
		BackgroundTimeoutSeconds: 5,
	})
	require.NoError(t, db.Model(&models.Scenario{}).
		Where("id = ?", session.ScenarioID).Update("crash_traps", true).Error)

	verifySvc := &bgTrackingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{validateRes: true}, verifySvc)

	_, err := sessionSvc.SubmitFlag(session.ID, "flag{step-zero}")
	require.NoError(t, err)

	require.Len(t, verifySvc.execCalls, 1)
	assert.Equal(t, map[string]string{flagEnvVar: "flag{step-one}"}, verifySvc.execCalls[0].env)

	for _, call := range verifySvc.pushFileCalls {
		assert.NotContains(t, call.content, "flag{step-one}",
			"crash_traps keeps flags off the filesystem entirely; %s carried one", call.targetPath)
	}
}

func TestBackgroundScript_FlagNeverAppearsInArgv(t *testing.T) {
	db := setupTestDB(t)
	session := flaggedTwoStepSession(t, db, "flag-env-not-argv", models.ScenarioStep{
		BackgroundScript:         "echo level one uses $OCF_FLAG_CURRENT",
		BackgroundTimeoutSeconds: 5,
	})

	verifySvc := &bgTrackingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{validateRes: true}, verifySvc)

	_, err := sessionSvc.SubmitFlag(session.ID, "flag{step-zero}")
	require.NoError(t, err)

	// /proc/<pid>/cmdline is world-readable inside the container, so a flag in
	// argv is a flag every level can read.
	require.Len(t, verifySvc.execCalls, 1)
	for _, arg := range verifySvc.execCalls[0].command {
		assert.NotContains(t, arg, "flag{step-one}")
	}
}

func TestReprovisionStep_CarriesTheCurrentStepsFlag(t *testing.T) {
	db := setupTestDB(t)
	session := flaggedTwoStepSession(t, db, "flag-env-reprovision", models.ScenarioStep{
		BackgroundScript:         "echo rebuilding level one",
		BackgroundTimeoutSeconds: 5,
	})
	require.NoError(t, db.Model(&models.ScenarioSession{}).
		Where("id = ?", session.ID).Update("current_step", 1).Error)

	verifySvc := &bgTrackingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	_, err := sessionSvc.ReprovisionCurrentStep(session.ID, false)
	require.NoError(t, err)

	// A retry must rebuild the level exactly as the advance did, flag included,
	// or the reprovisioned level comes back subtly different from the original.
	require.Len(t, verifySvc.execCalls, 1)
	assert.Equal(t, map[string]string{flagEnvVar: "flag{step-one}"}, verifySvc.execCalls[0].env)
}
