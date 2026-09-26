package scenarios_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

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
	db := freshTestDB(t)
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
	db := freshTestDB(t)
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
	db := freshTestDB(t)
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
	db := freshTestDB(t)
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
	db := freshTestDB(t)
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
	db := freshTestDB(t)
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

// The first step's foreground script at launch.
//
// A launch builds the level before anyone can have a console open, so the
// live-console path above always finds nobody attached and the demonstration
// would be lost. Instead the build leaves it pending on the run, and it is typed
// when the learner first opens their console — once, and only while the run is
// still on the step it belongs to. Only that attach types it: never the build,
// even when the learner's page is already open while the level is built.

// launchWithFirstStepForeground launches a two-step scenario whose first step
// has a background and a foreground script, waits for the build, and returns
// the running session. The mock accepts every console write, as if the
// learner's page were open throughout, so any write the build makes shows up.
func launchWithFirstStepForeground(t *testing.T, name string) (*models.ScenarioSession, *bgTrackingVerificationService, *services.ScenarioSessionService, *gorm.DB) {
	t.Helper()
	db := freshTestDB(t)

	scenario := models.Scenario{Name: name, Title: name, InstanceType: "ubuntu:22.04", CreatedByID: "creator-1"}
	require.NoError(t, db.Create(&scenario).Error)
	require.NoError(t, db.Create(&models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 0, Title: "Step 1",
		BackgroundScript: "mkdir -p /opt/lab",
		ForegroundScript: "cd /opt/lab && ls",
	}).Error)
	require.NoError(t, db.Create(&models.ScenarioStep{ScenarioID: scenario.ID, Order: 1, Title: "Step 2"}).Error)

	verifySvc := &bgTrackingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	session, err := sessionSvc.StartScenario("student-"+name, scenario.ID, "terminal-"+name, "")
	require.NoError(t, err)
	require.Equal(t, "active", waitForSetupDone(t, db, session.ID))

	return session, verifySvc, sessionSvc, db
}

func pendingForegroundOrder(t *testing.T, db *gorm.DB, sessionID any) *int {
	t.Helper()
	var session models.ScenarioSession
	require.NoError(t, db.First(&session, "id = ?", sessionID).Error)
	return session.PendingForegroundOrder
}

func TestForeground_PendingAfterProvisioning_TypedOnFirstLearnerAttach(t *testing.T) {
	session, verifySvc, sessionSvc, db := launchWithFirstStepForeground(t, "fg-pending-attach")

	assert.Empty(t, verifySvc.consoleWrites,
		"the build never types the foreground script: only the learner's own "+
			"first console attach does, even when a console is already open")
	pending := pendingForegroundOrder(t, db, session.ID)
	require.NotNil(t, pending, "the build must leave the first step's foreground pending on the run")
	assert.Equal(t, 0, *pending)

	sessionSvc.DeliverPendingForeground("terminal-fg-pending-attach")

	require.Len(t, verifySvc.consoleWrites, 1, "the learner's first attach types the pending script")
	assert.Equal(t, "cd /opt/lab && ls", verifySvc.consoleWrites[0].text)
	assert.Equal(t, "terminal-fg-pending-attach", verifySvc.consoleWrites[0].sessionID)
	assert.Nil(t, pendingForegroundOrder(t, db, session.ID),
		"a delivered foreground is no longer pending")
}

// Reopening the console — a page reload, a second tab — is not a new step. The
// demonstration has already played in that shell, and whatever it did to it (a
// cd, an export) is still true; typing it again is noise at best.
func TestForeground_SecondAttach_DoesNotRetype(t *testing.T) {
	_, verifySvc, sessionSvc, _ := launchWithFirstStepForeground(t, "fg-second-attach")

	sessionSvc.DeliverPendingForeground("terminal-fg-second-attach")
	sessionSvc.DeliverPendingForeground("terminal-fg-second-attach")

	assert.Len(t, verifySvc.consoleWrites, 1, "the pending script is typed once, on the first attach only")
}

// A learner can solve a step without ever opening the console (a flag submitted
// from elsewhere, a check that passes on the built world). The pending
// demonstration belongs to the step they have left, so it must never be typed
// into the shell of the step they are on now.
//
// Pinned: nothing is typed, on this attach or any later one. The column itself
// is left unasserted on purpose — the guarded clear (`WHERE current_step = ?`)
// leaves a stale order behind, and that is harmless because a run's current
// step never moves back to it; clearing it on advance would be an extra write
// for no observable difference.
func TestForeground_StepAdvancedMeanwhile_PendingDropped(t *testing.T) {
	session, verifySvc, sessionSvc, db := launchWithFirstStepForeground(t, "fg-advanced")

	result, err := sessionSvc.VerifyCurrentStep(session.ID)
	require.NoError(t, err)
	require.True(t, result.Passed)
	var advanced models.ScenarioSession
	require.NoError(t, db.First(&advanced, "id = ?", session.ID).Error)
	require.Equal(t, 1, advanced.CurrentStep, "precondition: the run moved on to the second step")
	require.Empty(t, verifySvc.consoleWrites, "precondition: the second step has no foreground of its own")

	sessionSvc.DeliverPendingForeground("terminal-fg-advanced")
	sessionSvc.DeliverPendingForeground("terminal-fg-advanced")

	assert.Empty(t, verifySvc.consoleWrites,
		"a foreground left pending by a step the learner has already left must never be typed")
}

// Reprovisioning a step whose foreground is still pending.
//
// The retry re-runs the step's setup and ends by typing its foreground live, so
// the pending copy the build left on the run is spent by it. Leaving it pending
// would type the demonstration a second time on the learner's next attach —
// and, while an async retry is still rebuilding the level, type it before the
// environment it demonstrates exists again.

// stepOnePendingForeground parks a run on a second step that has a background
// and a foreground script, with that foreground left pending as a build leaves
// it when no console was attached.
func stepOnePendingForeground(t *testing.T, db *gorm.DB, name string, async bool) *models.ScenarioSession {
	t.Helper()
	session := twoStepSession(t, db, name, models.ScenarioStep{
		BackgroundScript: "mkdir -p /opt/lab",
		BackgroundAsync:  async,
		ForegroundScript: "cd /opt/lab && ls",
	})
	require.NoError(t, db.Model(&models.ScenarioSession{}).Where("id = ?", session.ID).
		Updates(map[string]any{"current_step": 1, "pending_foreground_order": 1}).Error)
	return session
}

func TestForeground_SyncReprovision_TypesItOnce(t *testing.T) {
	db := freshTestDB(t)
	session := stepOnePendingForeground(t, db, "fg-reprovision-sync", false)
	verifySvc := &bgTrackingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	result, err := sessionSvc.ReprovisionCurrentStep(session.ID, false)
	require.NoError(t, err)
	require.Equal(t, "active", result.Status)
	require.Len(t, verifySvc.consoleWrites, 1, "precondition: the retry typed the foreground into the open console")

	sessionSvc.DeliverPendingForeground("terminal-fg-reprovision-sync")

	assert.Len(t, verifySvc.consoleWrites, 1,
		"the retry already typed the foreground; the next attach must not type it again")
	assert.Nil(t, pendingForegroundOrder(t, db, session.ID),
		"a foreground typed by the retry is no longer pending")
}

// blockingExecVerificationService holds every container exec until released,
// so a test can act while a step is still being provisioned.
type blockingExecVerificationService struct {
	bgTrackingVerificationService
	started chan struct{}
	release chan struct{}
}

func (m *blockingExecVerificationService) ExecInContainer(sessionID string, command []string, env map[string]string, timeout int) (int, string, string, error) {
	m.started <- struct{}{}
	<-m.release
	return m.bgTrackingVerificationService.ExecInContainer(sessionID, command, env, timeout)
}

func TestForeground_AsyncReprovision_AttachDuringRebuildTypesNothingEarly(t *testing.T) {
	db := freshTestDB(t)
	session := stepOnePendingForeground(t, db, "fg-reprovision-async", true)
	verifySvc := &blockingExecVerificationService{started: make(chan struct{}, 1), release: make(chan struct{})}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	result, err := sessionSvc.ReprovisionCurrentStep(session.ID, false)
	require.NoError(t, err)
	require.Equal(t, "provisioning", result.Status)
	<-verifySvc.started // the background script is running, not yet done

	sessionSvc.DeliverPendingForeground("terminal-fg-reprovision-async")

	assert.Empty(t, verifySvc.consoleWrites,
		"an attach while the step is being rebuilt must not type its foreground "+
			"before its background script has re-run")
	assert.Nil(t, pendingForegroundOrder(t, db, session.ID),
		"the retry takes over the pending foreground as it starts")

	close(verifySvc.release)
	require.Equal(t, "active", waitForSetupDone(t, db, session.ID))
	assert.Len(t, verifySvc.consoleWrites, 1, "the retry types the foreground once, after the rebuild")
	sessionSvc.DeliverPendingForeground("terminal-fg-reprovision-async")
	assert.Len(t, verifySvc.consoleWrites, 1, "and a later attach does not type it again")
}

// A live foreground with nobody attached.
//
// An advance or a retry types the step's foreground straight into the open
// console. When none is open, the demonstration is not lost: it is left pending
// for the learner's next attach, exactly as a build leaves it.

// openConsole models the learner opening their console after the live attempt
// found none: writes succeed from here on, and the log starts empty, since the
// attempt against a closed console typed nothing.
func openConsole(verifySvc *bgTrackingVerificationService) {
	verifySvc.consoleErr = nil
	verifySvc.consoleWrites = nil
}

func TestForeground_AdvanceWithNoConsole_LeftPendingForNextAttach(t *testing.T) {
	db := freshTestDB(t)
	session := twoStepSession(t, db, "fg-advance-no-console", models.ScenarioStep{
		ForegroundScript: "cd /opt && ls",
	})
	verifySvc := &bgTrackingVerificationService{consoleErr: services.ErrNoLiveConsole}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	result, err := sessionSvc.VerifyCurrentStep(session.ID)
	require.NoError(t, err)
	require.True(t, result.Passed)

	pending := pendingForegroundOrder(t, db, session.ID)
	require.NotNil(t, pending, "a foreground that found no console must wait for the learner's attach")
	assert.Equal(t, 1, *pending)

	openConsole(verifySvc)
	sessionSvc.DeliverPendingForeground("terminal-fg-advance-no-console")

	require.Len(t, verifySvc.consoleWrites, 1, "the learner's next attach types it once")
	assert.Equal(t, "cd /opt && ls", verifySvc.consoleWrites[0].text)
	assert.Nil(t, pendingForegroundOrder(t, db, session.ID))
}

// With a console open the advance types it there and then, and leaves nothing
// behind for the next attach to type a second time.
func TestForeground_AdvanceWithConsole_LeavesNothingPending(t *testing.T) {
	db := freshTestDB(t)
	session := twoStepSession(t, db, "fg-advance-console", models.ScenarioStep{
		ForegroundScript: "cd /opt && ls",
	})
	verifySvc := &bgTrackingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	_, err := sessionSvc.VerifyCurrentStep(session.ID)
	require.NoError(t, err)
	require.Len(t, verifySvc.consoleWrites, 1)
	assert.Nil(t, pendingForegroundOrder(t, db, session.ID))

	sessionSvc.DeliverPendingForeground("terminal-fg-advance-console")

	assert.Len(t, verifySvc.consoleWrites, 1, "a foreground typed live is not typed again on attach")
}

func TestForeground_SyncReprovisionWithNoConsole_LeftPending(t *testing.T) {
	db := freshTestDB(t)
	session := twoStepSession(t, db, "fg-reprovision-no-console", models.ScenarioStep{
		BackgroundScript: "mkdir -p /opt/lab",
		ForegroundScript: "cd /opt/lab && ls",
	})
	require.NoError(t, db.Model(&models.ScenarioSession{}).Where("id = ?", session.ID).
		Update("current_step", 1).Error)
	verifySvc := &bgTrackingVerificationService{consoleErr: services.ErrNoLiveConsole}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	result, err := sessionSvc.ReprovisionCurrentStep(session.ID, false)
	require.NoError(t, err)
	require.Equal(t, "active", result.Status)

	pending := pendingForegroundOrder(t, db, session.ID)
	require.NotNil(t, pending, "a retried foreground that found no console must wait for the learner's attach")
	assert.Equal(t, 1, *pending)

	openConsole(verifySvc)
	sessionSvc.DeliverPendingForeground("terminal-fg-reprovision-no-console")

	require.Len(t, verifySvc.consoleWrites, 1, "the learner's next attach types it once")
	assert.Nil(t, pendingForegroundOrder(t, db, session.ID))
}
