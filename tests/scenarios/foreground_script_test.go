package scenarios_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

// A step's foreground script runs in the learner's own shell.
//
// This is the property that distinguishes it from the background script and the
// reason it cannot be an exec: a foreground script exists so the learner watches
// it happen, and so that what it does to the shell — cd, export, a function —
// is still true for them afterwards. Running it in a separate process would
// satisfy neither.
func TestForegroundScript_IsTypedIntoTheLearnersShell(t *testing.T) {
	db := setupTestDB(t)
	session := twoStepSession(t, db, "foreground-basic", models.ScenarioStep{
		ForegroundScript: "cd /opt && ls -la",
	})

	verifySvc := &bgTrackingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	_, err := sessionSvc.VerifyCurrentStep(session.ID)
	require.NoError(t, err)

	require.Len(t, verifySvc.consoleWrites, 1)
	assert.Equal(t, "cd /opt && ls -la", verifySvc.consoleWrites[0].text)
	// Addressed to the TERMINAL session, not the scenario session: the shell is
	// the thing being typed into, and only tt-backend knows where it lives.
	assert.Equal(t, "terminal-foreground-basic", verifySvc.consoleWrites[0].sessionID)
}

// A step with no foreground script must not touch the console at all. Sending
// an empty line would still submit a newline into the learner's shell, which
// they would see as a stray blank prompt on every advance.
func TestForegroundScript_AbsentScriptWritesNothing(t *testing.T) {
	db := setupTestDB(t)
	session := twoStepSession(t, db, "foreground-absent", models.ScenarioStep{
		BackgroundScript: "echo setup",
	})

	verifySvc := &bgTrackingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	_, err := sessionSvc.VerifyCurrentStep(session.ID)
	require.NoError(t, err)

	assert.Empty(t, verifySvc.consoleWrites)
}

// The foreground script runs after the background script, because it acts on
// the environment the background script builds. A demonstration of a file that
// does not exist yet is not a demonstration.
func TestForegroundScript_RunsAfterTheEnvironmentExists(t *testing.T) {
	db := setupTestDB(t)
	session := twoStepSession(t, db, "foreground-ordering", models.ScenarioStep{
		BackgroundScript: "mkdir -p /opt/lab",
		ForegroundScript: "ls /opt/lab",
	})

	verifySvc := &bgTrackingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	_, err := sessionSvc.VerifyCurrentStep(session.ID)
	require.NoError(t, err)

	require.Len(t, verifySvc.execCalls, 1, "the background script must have run")
	require.Len(t, verifySvc.consoleWrites, 1, "the foreground script must have run")
}

// No console attached is the ordinary case, not a failure: the learner may not
// have opened their terminal. The advance still succeeds — the level is already
// provisioned, and losing a legitimately earned step over a missed demonstration
// would be a far worse trade.
func TestForegroundScript_NoConsoleDoesNotFailTheAdvance(t *testing.T) {
	db := setupTestDB(t)
	session := twoStepSession(t, db, "foreground-no-console", models.ScenarioStep{
		ForegroundScript: "echo hello",
	})

	verifySvc := &bgTrackingVerificationService{consoleErr: services.ErrNoLiveConsole}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	result, err := sessionSvc.VerifyCurrentStep(session.ID)

	require.NoError(t, err)
	assert.True(t, result.Passed)
	assert.False(t, result.NextStepProvisioningFailed,
		"a missed demonstration is not a provisioning failure")
	assert.Equal(t, "active", sessionStatus(t, db, session.ID))
}

// Any other console error is equally non-fatal, for the same reason.
func TestForegroundScript_WriteErrorDoesNotFailTheAdvance(t *testing.T) {
	db := setupTestDB(t)
	session := twoStepSession(t, db, "foreground-write-error", models.ScenarioStep{
		ForegroundScript: "echo hello",
	})

	verifySvc := &bgTrackingVerificationService{consoleErr: errors.New("connection reset")}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	result, err := sessionSvc.VerifyCurrentStep(session.ID)

	require.NoError(t, err)
	assert.True(t, result.Passed)
	assert.False(t, result.NextStepProvisioningFailed)
	assert.Equal(t, "active", sessionStatus(t, db, session.ID))
}

// A background script that fails aborts the step's provisioning, so the
// foreground script must not run: its environment was never built, and typing a
// command that will error into the learner's shell tells them nothing useful.
func TestForegroundScript_SkippedWhenTheBackgroundScriptFailed(t *testing.T) {
	db := setupTestDB(t)
	session := twoStepSession(t, db, "foreground-after-failure", models.ScenarioStep{
		BackgroundScript: "false",
		ForegroundScript: "ls /opt/lab",
	})

	verifySvc := &bgTrackingVerificationService{execErr: errors.New("script exited 1")}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	_, err := sessionSvc.VerifyCurrentStep(session.ID)
	require.NoError(t, err)

	assert.Empty(t, verifySvc.consoleWrites,
		"nothing should be typed into a shell whose level was never provisioned")
}
