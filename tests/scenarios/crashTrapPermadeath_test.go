// tests/scenarios/crashTrapPermadeath_test.go
//
// Permadeath: in a crash_traps scenario (linux-rogueLite and friends), a
// careless command runs `kill -9 -1` and the learner loses the run. The
// container survives `kill -9 -1` (PID 1 is spared by specification), so the
// only observable signal is the learner's shell dying by SIGKILL — tt-backend
// reports it as WebSocket close code 4137 (4000 + exit 137, 128+SIGKILL).
//
// These tests pin the server-side half: which sessions that signal ends, and —
// more importantly — which ones it must leave alone. Before this, the learner
// simply reconnected to the same container with every flag and step still
// theirs, and the mechanic did nothing at all.
//
// A crash-trap run may now be persistent (pausable), and a pause's force-kill
// can also end the shell with 137. So permadeath only fires when tt-backend
// confirms the container is still running — `kill -9 -1` leaves it up, a stop
// does not — and it deletes the container rather than stopping it, since a
// stopped persistent container would be resumable.
package scenarios_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

// seedRunOnTerminal creates a scenario (with or without crash traps) plus an
// active session attached to the given terminal, and returns the session.
func seedRunOnTerminal(t *testing.T, db *gorm.DB, name string, crashTraps bool, terminalSessionID string) *models.ScenarioSession {
	t.Helper()
	scenario := models.Scenario{
		Name:         name,
		Title:        name,
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-" + name,
		CrashTraps:   crashTraps,
	}
	require.NoError(t, db.Create(&scenario).Error)
	require.NoError(t, db.Create(&models.ScenarioStep{
		ScenarioID:  scenario.ID,
		Order:       0,
		Title:       "Step 1",
		TextContent: "First step",
	}).Error)

	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, &mockVerificationService{})
	session, err := sessionSvc.StartScenario("student-"+name, scenario.ID, terminalSessionID, "")
	require.NoError(t, err)
	require.Equal(t, "active", session.Status,
		"a scenario without a setup script starts active")
	return session
}

// instanceRunning returns a liveness callback with a fixed answer.
func instanceRunning(running bool, err error) services.TerminalInstanceRunningFunc {
	return func(string) (bool, error) { return running, err }
}

// wireCrashTrapTerminal gives a session service the three terminal callbacks
// permadeath uses, and returns the stop and delete trackers.
func wireCrashTrapTerminal(sessionSvc *services.ScenarioSessionService, liveness services.TerminalInstanceRunningFunc) (stopped, deleted *terminalStopTracker) {
	stopped, deleted = &terminalStopTracker{}, &terminalStopTracker{}
	sessionSvc.SetTerminalStopFunc(stopped.StopFunc())
	sessionSvc.SetTerminalDeleteFunc(deleted.DeleteFunc())
	sessionSvc.SetTerminalInstanceRunningFunc(liveness)
	return stopped, deleted
}

func sessionStatus(t *testing.T, db *gorm.DB, sessionID any) string {
	t.Helper()
	var stored models.ScenarioSession
	require.NoError(t, db.First(&stored, "id = ?", sessionID).Error)
	return stored.Status
}

func TestEndCrashTrapRun_AbandonsTheRunAndDeletesTheTerminal(t *testing.T) {
	db := freshTestDB(t)
	session := seedRunOnTerminal(t, db, "permadeath-armed", true, "terminal-permadeath-armed")

	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, &mockVerificationService{})
	stopped, deleted := wireCrashTrapTerminal(sessionSvc, instanceRunning(true, nil))

	sessionSvc.EndCrashTrapRun("terminal-permadeath-armed")

	assert.Equal(t, "abandoned", sessionStatus(t, db, session.ID),
		"a SIGKILLed shell inside a crash_traps scenario must end the run — "+
			"otherwise the learner reconnects with every flag still theirs")
	assert.Equal(t, []string{"terminal-permadeath-armed"}, deleted.CalledWith(),
		"the container must be deleted, so the learner cannot keep working in it")
	assert.Zero(t, stopped.CallCount(),
		"a stopped persistent container is resumable — permadeath must delete, never stop")
}

func TestEndCrashTrapRun_InstanceNotRunning_KeepsRunOpen(t *testing.T) {
	// A 137 while the container is stopping or stopped is a pause's
	// force-kill, not a crash trap: `kill -9 -1` spares PID 1, so a real trap
	// leaves the container running.
	db := freshTestDB(t)
	session := seedRunOnTerminal(t, db, "permadeath-paused", true, "terminal-permadeath-paused")

	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, &mockVerificationService{})
	stopped, deleted := wireCrashTrapTerminal(sessionSvc, instanceRunning(false, nil))

	sessionSvc.EndCrashTrapRun("terminal-permadeath-paused")

	assert.Equal(t, "active", sessionStatus(t, db, session.ID),
		"a killed shell on a container that is not running is a pause, and "+
			"pausing must not end the run")
	assert.Zero(t, deleted.CallCount(),
		"a paused container must never be deleted by the permadeath path")
	assert.Zero(t, stopped.CallCount())
}

func TestEndCrashTrapRun_LivenessUnknown_KeepsRunOpen(t *testing.T) {
	// Deleting is irreversible, so when tt-backend cannot say whether the
	// container is running, permadeath must not fire.
	db := freshTestDB(t)
	session := seedRunOnTerminal(t, db, "permadeath-unknown", true, "terminal-permadeath-unknown")

	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, &mockVerificationService{})
	stopped, deleted := wireCrashTrapTerminal(sessionSvc, instanceRunning(true, errors.New("tt-backend unreachable")))

	sessionSvc.EndCrashTrapRun("terminal-permadeath-unknown")

	assert.Equal(t, "active", sessionStatus(t, db, session.ID),
		"an unknown liveness must keep the run open — a false permadeath "+
			"would destroy the learner's work for good")
	assert.Zero(t, deleted.CallCount(),
		"nothing may be deleted when liveness is unknown")
	assert.Zero(t, stopped.CallCount())
}

func TestEndCrashTrapRun_NoLivenessCheckWired_KeepsRunOpen(t *testing.T) {
	// An unwired liveness callback is the same as an unknown answer.
	db := freshTestDB(t)
	session := seedRunOnTerminal(t, db, "permadeath-unwired", true, "terminal-permadeath-unwired")

	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, &mockVerificationService{})
	stopped, deleted := wireCrashTrapTerminal(sessionSvc, nil)

	sessionSvc.EndCrashTrapRun("terminal-permadeath-unwired")

	assert.Equal(t, "active", sessionStatus(t, db, session.ID),
		"without a liveness check permadeath must not fire")
	assert.Zero(t, deleted.CallCount())
	assert.Zero(t, stopped.CallCount())
}

func TestEndCrashTrapRun_LeavesOrdinaryScenarioUntouched(t *testing.T) {
	db := freshTestDB(t)
	session := seedRunOnTerminal(t, db, "permadeath-disarmed", false, "terminal-permadeath-disarmed")

	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, &mockVerificationService{})
	stopped, deleted := wireCrashTrapTerminal(sessionSvc, instanceRunning(true, nil))

	sessionSvc.EndCrashTrapRun("terminal-permadeath-disarmed")

	assert.Equal(t, "active", sessionStatus(t, db, session.ID),
		"only crash_traps scenarios arm permadeath; elsewhere a killed shell is "+
			"an accident the learner recovers from by reconnecting")
	assert.Zero(t, stopped.CallCount()+deleted.CallCount(),
		"an ordinary scenario's container must keep running")
}

func TestEndCrashTrapRun_LeavesPlainTerminalUntouched(t *testing.T) {
	db := freshTestDB(t)
	// A terminal with no scenario session at all — the ordinary "open a
	// terminal from the dashboard" case.
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, &mockVerificationService{})
	stopped, deleted := wireCrashTrapTerminal(sessionSvc, instanceRunning(true, nil))

	require.NotPanics(t, func() {
		sessionSvc.EndCrashTrapRun("terminal-with-no-scenario")
	}, "a plain terminal must be a silent no-op, not a crash")
	assert.Zero(t, stopped.CallCount()+deleted.CallCount(),
		"a plain terminal must not be stopped or deleted by the permadeath path")

	require.NotPanics(t, func() {
		sessionSvc.EndCrashTrapRun("")
	}, "an empty terminal id must be a silent no-op")
}

func TestEndCrashTrapRun_LeavesFinishedRunUntouched(t *testing.T) {
	db := freshTestDB(t)
	session := seedRunOnTerminal(t, db, "permadeath-finished", true, "terminal-permadeath-finished")
	require.NoError(t, db.Model(&models.ScenarioSession{}).
		Where("id = ?", session.ID).Update("status", "completed").Error)

	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, &mockVerificationService{})
	stopped, deleted := wireCrashTrapTerminal(sessionSvc, instanceRunning(true, nil))

	sessionSvc.EndCrashTrapRun("terminal-permadeath-finished")

	assert.Equal(t, "completed", sessionStatus(t, db, session.ID),
		"a run that already finished must not be rewritten to abandoned — the "+
			"learner's grade would silently disappear")
	assert.Zero(t, stopped.CallCount()+deleted.CallCount(),
		"nothing to stop or delete for a run that already ended")
}

// TestEndCrashTrapRun_RefusesLearnerActionsAfterPermadeath proves the run is
// really over rather than merely relabelled: the launcher's Resume affordance
// and every learner action key on status == "active".
func TestEndCrashTrapRun_RefusesLearnerActionsAfterPermadeath(t *testing.T) {
	db := freshTestDB(t)
	session := seedRunOnTerminal(t, db, "permadeath-resume", true, "terminal-permadeath-resume")

	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, &mockVerificationService{})
	wireCrashTrapTerminal(sessionSvc, instanceRunning(true, nil))
	sessionSvc.EndCrashTrapRun("terminal-permadeath-resume")
	require.Equal(t, "abandoned", sessionStatus(t, db, session.ID))

	_, err := sessionSvc.RevealHint(session.ID, 0, 1)
	require.Error(t, err,
		"a dead run must refuse learner actions, not just carry a different label")
	assert.Contains(t, err.Error(), "not active",
		"the refusal must come from the active-session gate every learner action "+
			"shares, not from some incidental failure. Got %v", err)
}

// TestFindSessionByTerminal_ReturnsTheMostRecentRun pins the newest-first
// ordering the by-terminal endpoint has always had, now that permadeath shares
// the lookup with it: the two must never disagree about which run a terminal
// belongs to. StartScenario binds a terminal to one run permanently, so the
// second row is seeded directly — the ordering is a defensive guarantee for
// rows other paths (preview, import) may leave behind.
func TestFindSessionByTerminal_ReturnsTheMostRecentRun(t *testing.T) {
	db := freshTestDB(t)
	older := seedRunOnTerminal(t, db, "by-terminal-older", true, "terminal-reused")
	reusedTerminal := "terminal-reused"
	newer := models.ScenarioSession{
		BaseModel: entityManagementModels.BaseModel{
			Model: gorm.Model{CreatedAt: older.CreatedAt.Add(time.Hour)},
		},
		ScenarioID:        older.ScenarioID,
		UserID:            "student-by-terminal-newer",
		TerminalSessionID: &reusedTerminal,
		Status:            "active",
		StartedAt:         older.StartedAt.Add(time.Hour),
	}
	require.NoError(t, db.Create(&newer).Error)

	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, &mockVerificationService{})
	found, err := sessionSvc.FindSessionByTerminal("terminal-reused")

	require.NoError(t, err)
	assert.Equal(t, newer.ID, found.ID,
		"the newest run must win, exactly as GET /scenario-sessions/by-terminal "+
			"has always resolved it")
}
