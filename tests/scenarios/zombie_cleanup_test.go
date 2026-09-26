package scenarios_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
	terminalModels "soli/formations/src/terminalTrainer/models"
	terminalServices "soli/formations/src/terminalTrainer/services"
)

// --- Fix 2: Cron cleanup job ---

func TestCleanupZombieScenarioSessions_AbandonsStaleSessions(t *testing.T) {
	db := freshTestDB(t)

	scenario := models.Scenario{
		Name:         "cleanup-test",
		Title:        "Cleanup Test",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
		// Crash traps: the sweep abandons only runs that cannot be rebuilt
		// (TestCleanupZombieScenarioSessions_SparesRebuildableRun).
		CrashTraps: true,
	}
	require.NoError(t, db.Create(&scenario).Error)

	step := models.ScenarioStep{
		ScenarioID: scenario.ID,
		Order:      0,
		Title:      "Step 1",
	}
	require.NoError(t, db.Create(&step).Error)

	// --- Session 1: linked to an EXPIRED terminal ---
	utk1 := terminalModels.UserTerminalKey{
		UserID: "student-cleanup-1", APIKey: "key-c1", KeyName: "k1", IsActive: true,
	}
	require.NoError(t, db.Create(&utk1).Error)

	expiredTerminal := terminalModels.Terminal{
		SessionID: "terminal-expired-cleanup", UserID: "student-cleanup-1",
		State: "deleted", ExpiresAt: time.Now().Add(-2 * time.Hour),
		InstanceType: "ubuntu:22.04", UserTerminalKeyID: utk1.ID,
	}
	require.NoError(t, db.Create(&expiredTerminal).Error)

	expiredTerminalID := "terminal-expired-cleanup"
	sessionExpired := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "student-cleanup-1",
		TerminalSessionID: &expiredTerminalID,
		CurrentStep:       0, Status: "active", StartedAt: time.Now().Add(-3 * time.Hour),
	}
	require.NoError(t, db.Create(&sessionExpired).Error)

	// --- Session 2: linked to a STOPPED terminal ---
	utk2 := terminalModels.UserTerminalKey{
		UserID: "student-cleanup-2", APIKey: "key-c2", KeyName: "k2", IsActive: true,
	}
	require.NoError(t, db.Create(&utk2).Error)

	stoppedTerminal := terminalModels.Terminal{
		SessionID: "terminal-stopped-cleanup", UserID: "student-cleanup-2",
		State: "stopped", ExpiresAt: time.Now().Add(-1 * time.Hour),
		InstanceType: "ubuntu:22.04", UserTerminalKeyID: utk2.ID,
	}
	require.NoError(t, db.Create(&stoppedTerminal).Error)

	stoppedTerminalID := "terminal-stopped-cleanup"
	sessionStopped := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "student-cleanup-2",
		TerminalSessionID: &stoppedTerminalID,
		CurrentStep:       0, Status: "active", StartedAt: time.Now().Add(-2 * time.Hour),
	}
	require.NoError(t, db.Create(&sessionStopped).Error)

	// --- Session 3: linked to an ACTIVE terminal (should NOT be touched) ---
	utk3 := terminalModels.UserTerminalKey{
		UserID: "student-cleanup-3", APIKey: "key-c3", KeyName: "k3", IsActive: true,
	}
	require.NoError(t, db.Create(&utk3).Error)

	activeTerminal := terminalModels.Terminal{
		SessionID: "terminal-active-cleanup", UserID: "student-cleanup-3",
		State: "running", ExpiresAt: time.Now().Add(1 * time.Hour),
		InstanceType: "ubuntu:22.04", UserTerminalKeyID: utk3.ID,
	}
	require.NoError(t, db.Create(&activeTerminal).Error)

	activeTerminalID := "terminal-active-cleanup"
	sessionActive := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "student-cleanup-3",
		TerminalSessionID: &activeTerminalID,
		CurrentStep:       0, Status: "active", StartedAt: time.Now().Add(-30 * time.Minute),
	}
	require.NoError(t, db.Create(&sessionActive).Error)

	// Run cleanup
	count, err := services.CleanupZombieScenarioSessions(db)
	require.NoError(t, err)
	assert.Equal(t, int64(2), count, "should abandon exactly 2 zombie sessions")

	// Verify expired-terminal session was abandoned
	var s1 models.ScenarioSession
	require.NoError(t, db.First(&s1, "id = ?", sessionExpired.ID).Error)
	assert.Equal(t, "abandoned", s1.Status)

	// Verify stopped-terminal session was abandoned
	var s2 models.ScenarioSession
	require.NoError(t, db.First(&s2, "id = ?", sessionStopped.ID).Error)
	assert.Equal(t, "abandoned", s2.Status)

	// Verify active-terminal session was NOT touched
	var s3 models.ScenarioSession
	require.NoError(t, db.First(&s3, "id = ?", sessionActive.ID).Error)
	assert.Equal(t, "active", s3.Status)
}

func TestCleanupZombieScenarioSessions_HandlesInProgressStatus(t *testing.T) {
	db := freshTestDB(t)

	scenario := models.Scenario{
		Name:         "cleanup-inprogress",
		Title:        "Cleanup In Progress Test",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
		// Crash traps: the sweep abandons only runs that cannot be rebuilt
		// (TestCleanupZombieScenarioSessions_SparesRebuildableRun).
		CrashTraps: true,
	}
	require.NoError(t, db.Create(&scenario).Error)

	step := models.ScenarioStep{
		ScenarioID: scenario.ID,
		Order:      0,
		Title:      "Step 1",
	}
	require.NoError(t, db.Create(&step).Error)

	utk := terminalModels.UserTerminalKey{
		UserID: "student-cleanup-ip", APIKey: "key-ip", KeyName: "k-ip", IsActive: true,
	}
	require.NoError(t, db.Create(&utk).Error)

	expiredTerminal := terminalModels.Terminal{
		SessionID: "terminal-ip-expired", UserID: "student-cleanup-ip",
		State: "deleted", ExpiresAt: time.Now().Add(-2 * time.Hour),
		InstanceType: "ubuntu:22.04", UserTerminalKeyID: utk.ID,
	}
	require.NoError(t, db.Create(&expiredTerminal).Error)

	termID := "terminal-ip-expired"
	sessionIP := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "student-cleanup-ip",
		TerminalSessionID: &termID,
		CurrentStep:       0, Status: "in_progress", StartedAt: time.Now().Add(-1 * time.Hour),
	}
	require.NoError(t, db.Create(&sessionIP).Error)

	count, err := services.CleanupZombieScenarioSessions(db)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count, "should abandon in_progress session with expired terminal")

	var s models.ScenarioSession
	require.NoError(t, db.First(&s, "id = ?", sessionIP.ID).Error)
	assert.Equal(t, "abandoned", s.Status)
}

func TestCleanupZombieScenarioSessions_IgnoresCompletedSessions(t *testing.T) {
	db := freshTestDB(t)

	scenario := models.Scenario{
		Name:         "cleanup-completed",
		Title:        "Cleanup Completed Test",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	step := models.ScenarioStep{
		ScenarioID: scenario.ID,
		Order:      0,
		Title:      "Step 1",
	}
	require.NoError(t, db.Create(&step).Error)

	// Create a COMPLETED session linked to an expired terminal
	utk := terminalModels.UserTerminalKey{
		UserID: "student-cleanup-done", APIKey: "key-done", KeyName: "k-done", IsActive: true,
	}
	require.NoError(t, db.Create(&utk).Error)

	expiredTerminal := terminalModels.Terminal{
		SessionID: "terminal-completed-expired", UserID: "student-cleanup-done",
		State: "deleted", ExpiresAt: time.Now().Add(-2 * time.Hour),
		InstanceType: "ubuntu:22.04", UserTerminalKeyID: utk.ID,
	}
	require.NoError(t, db.Create(&expiredTerminal).Error)

	completedAt := time.Now().Add(-1 * time.Hour)
	grade := 100.0
	termID := "terminal-completed-expired"
	completedSession := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "student-cleanup-done",
		TerminalSessionID: &termID,
		CurrentStep:       0, Status: "completed",
		StartedAt: time.Now().Add(-3 * time.Hour), CompletedAt: &completedAt, Grade: &grade,
	}
	require.NoError(t, db.Create(&completedSession).Error)

	// Run cleanup
	count, err := services.CleanupZombieScenarioSessions(db)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count, "should not touch completed sessions")

	// Verify session is still completed
	var s models.ScenarioSession
	require.NoError(t, db.First(&s, "id = ?", completedSession.ID).Error)
	assert.Equal(t, "completed", s.Status, "completed session must stay completed")
	assert.NotNil(t, s.Grade)
}

func TestCleanupStuckProvisioningSessions_ReleasesStalledSessions(t *testing.T) {
	db := freshTestDB(t)

	scenario := models.Scenario{
		Name:         "reaper-scenario",
		Title:        "Reaper scenario",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	newSession := func(user string, status string, age time.Duration) *models.ScenarioSession {
		s := models.ScenarioSession{
			ScenarioID:        scenario.ID,
			UserID:            user,
			CurrentStep:       0,
			Status:            status,
			ProvisioningPhase: "step_setup",
			StartedAt:         time.Now(),
		}
		require.NoError(t, db.Create(&s).Error)
		require.NoError(t, db.Model(&models.ScenarioSession{}).
			Where("id = ?", s.ID).
			Update("updated_at", time.Now().Add(-age)).Error)
		return &s
	}

	stalled := newSession("student-stalled", "provisioning", 15*time.Minute)
	recent := newSession("student-recent", "provisioning", 2*time.Minute)
	active := newSession("student-active", "active", 15*time.Minute)

	count, err := services.CleanupStuckProvisioningSessions(db)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)

	assert.Equal(t, "setup_failed", sessionStatus(t, db, stalled.ID),
		"a session whose setup goroutine died must be released — the unique partial index covers 'provisioning' and would otherwise block every restart")
	assert.Equal(t, "provisioning", sessionStatus(t, db, recent.ID),
		"a setup still inside its budget is not a zombie")
	assert.Equal(t, "active", sessionStatus(t, db, active.ID))

	var reaped models.ScenarioSession
	require.NoError(t, db.First(&reaped, "id = ?", stalled.ID).Error)
	assert.Equal(t, "", reaped.ProvisioningPhase)
}

// The reaper's patience and what a step may declare are one rule, not two.
//
// A step can ask for up to MaxBackgroundTimeoutSeconds. If the reaper's cutoff
// were chosen independently and landed below that, a step running inside its
// own declared budget would be written off mid-run — and worse, silently: the
// goroutine's success is written with a WHERE status = 'provisioning' guard,
// which the reaper has already cleared, so the update matches nothing and the
// session stays setup_failed having actually succeeded.
//
// This pins the ordering rather than the numbers, so raising either constant
// alone fails here instead of in a learner's session.
func TestCleanupStuckProvisioningSessions_SparesAStepInsideItsDeclaredBudget(t *testing.T) {
	db := freshTestDB(t)

	scenario := models.Scenario{
		Name:         "budget-scenario",
		Title:        "Budget scenario",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	// A session that has been provisioning for exactly the longest budget a
	// step is allowed to declare. It is at the edge of legitimate, not past it.
	atCeiling := models.ScenarioSession{
		ScenarioID:        scenario.ID,
		UserID:            "student-at-ceiling",
		Status:            "provisioning",
		ProvisioningPhase: "step_setup",
		StartedAt:         time.Now(),
	}
	require.NoError(t, db.Create(&atCeiling).Error)
	require.NoError(t, db.Model(&models.ScenarioSession{}).
		Where("id = ?", atCeiling.ID).
		Update("updated_at",
			time.Now().Add(-time.Duration(services.MaxBackgroundTimeoutSeconds)*time.Second)).Error)

	count, err := services.CleanupStuckProvisioningSessions(db)
	require.NoError(t, err)

	assert.Equal(t, int64(0), count)
	assert.Equal(t, "provisioning", sessionStatus(t, db, atCeiling.ID),
		"a step still inside the budget it was allowed to declare is not stuck")
}

// TestCleanupZombieScenarioSessions_AbandonsRunOnExpiredButRunningTerminal is
// the case the state-only rule missed, and the one learners actually hit: a
// terminal that simply reached its TTL. Nothing moves the state column then, so
// the row still reads "running" while its container is long gone — one such
// session stayed "active" for 21 hours and left the learner staring at a Resume
// button into nothing, with no way to start the scenario again.
func TestCleanupZombieScenarioSessions_AbandonsRunOnExpiredButRunningTerminal(t *testing.T) {
	db := freshTestDB(t)

	scenario := models.Scenario{
		Name:         "cleanup-expired-running",
		Title:        "Cleanup Expired Running",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
		// Crash traps: the sweep abandons only runs that cannot be rebuilt
		// (TestCleanupZombieScenarioSessions_SparesRebuildableRun).
		CrashTraps: true,
	}
	require.NoError(t, db.Create(&scenario).Error)

	utk := terminalModels.UserTerminalKey{
		UserID: "student-expired-running", APIKey: "key-er", KeyName: "k-er", IsActive: true,
	}
	require.NoError(t, db.Create(&utk).Error)

	terminal := terminalModels.Terminal{
		SessionID: "terminal-expired-running", UserID: "student-expired-running",
		State: terminalModels.StateRunning, ExpiresAt: time.Now().Add(-21 * time.Hour),
		InstanceType: "ubuntu:22.04", UserTerminalKeyID: utk.ID,
	}
	require.NoError(t, db.Create(&terminal).Error)

	terminalID := "terminal-expired-running"
	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "student-expired-running",
		TerminalSessionID: &terminalID,
		CurrentStep:       0, Status: "active", StartedAt: time.Now().Add(-22 * time.Hour),
	}
	require.NoError(t, db.Create(&session).Error)

	count, err := services.CleanupZombieScenarioSessions(db)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)

	var reloaded models.ScenarioSession
	require.NoError(t, db.First(&reloaded, "id = ?", session.ID).Error)
	assert.Equal(t, "abandoned", reloaded.Status,
		"a run whose terminal is past its TTL is not a run the learner can return to")
}

// seedOpenRunWithTerminal creates an open run of a fresh one-step scenario bound to
// terminalSessionID, plus the terminal row itself unless state is empty (a run
// whose terminal row has vanished).
func seedOpenRunWithTerminal(t *testing.T, db *gorm.DB, userID, terminalSessionID string, state terminalModels.TerminalState, expires time.Duration, persistence string, status string) (models.ScenarioSession, *terminalModels.Terminal) {
	t.Helper()

	scenario := models.Scenario{
		Name:         "run-" + terminalSessionID,
		Title:        "Run " + terminalSessionID,
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)
	require.NoError(t, db.Create(&models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 0, Title: "Step 1",
	}).Error)

	var terminal *terminalModels.Terminal
	if state != "" {
		terminal = &terminalModels.Terminal{
			SessionID:       terminalSessionID,
			UserID:          userID,
			State:           state,
			PersistenceMode: persistence,
			ExpiresAt:       time.Now().Add(expires),
			InstanceType:    "ubuntu:22.04",
		}
		require.NoError(t, db.Create(terminal).Error)
	}

	terminalID := terminalSessionID
	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: userID,
		TerminalSessionID: &terminalID,
		CurrentStep:       0, Status: status, StartedAt: time.Now().Add(-2 * time.Hour),
	}
	require.NoError(t, db.Create(&session).Error)
	return session, terminal
}

// runSeeder is the signature shared by seedOpenRunWithTerminal and its
// crash-trap and preview variants.
type runSeeder func(t *testing.T, db *gorm.DB, userID, terminalSessionID string, state terminalModels.TerminalState, expires time.Duration, persistence string, status string) (models.ScenarioSession, *terminalModels.Terminal)

// seedCrashTrapRunWithTerminal is seedOpenRunWithTerminal for a crash-trap
// scenario: the kind of run that dies with its container.
func seedCrashTrapRunWithTerminal(t *testing.T, db *gorm.DB, userID, terminalSessionID string, state terminalModels.TerminalState, expires time.Duration, persistence string, status string) (models.ScenarioSession, *terminalModels.Terminal) {
	t.Helper()
	session, terminal := seedOpenRunWithTerminal(t, db, userID, terminalSessionID, state, expires, persistence, status)
	require.NoError(t, db.Model(&models.Scenario{}).
		Where("id = ?", session.ScenarioID).Update("crash_traps", true).Error)
	return session, terminal
}

// seedPreviewRunWithTerminal is seedOpenRunWithTerminal for an author's
// preview run, which is never rebuilt either.
func seedPreviewRunWithTerminal(t *testing.T, db *gorm.DB, userID, terminalSessionID string, state terminalModels.TerminalState, expires time.Duration, persistence string, status string) (models.ScenarioSession, *terminalModels.Terminal) {
	t.Helper()
	session, terminal := seedOpenRunWithTerminal(t, db, userID, terminalSessionID, state, expires, persistence, status)
	require.NoError(t, db.Model(&models.ScenarioSession{}).
		Where("id = ?", session.ID).Update("is_preview", true).Error)
	session.IsPreview = true
	return session, terminal
}

// A normal run whose container is gone keeps its progress: the learner
// resumes it by rebuilding the environment at the current step. The sweep
// abandoning it every five minutes would throw that progress away.
func TestCleanupZombieScenarioSessions_SparesRebuildableRun(t *testing.T) {
	db := freshTestDB(t)

	cases := []struct {
		name        string
		state       terminalModels.TerminalState
		expires     time.Duration
		persistence string
		status      string
	}{
		{"deleted", terminalModels.StateDeleted, -2 * time.Hour, "", "active"},
		{"stopped-expired", terminalModels.StateStopped, -time.Hour, "", "active"},
		{"running-expired-ephemeral", terminalModels.StateRunning, -21 * time.Hour, "ephemeral", "in_progress"},
		{"missing-row", "", 0, "", "active"},
	}
	runs := map[string]models.ScenarioSession{}
	for _, tc := range cases {
		run, _ := seedOpenRunWithTerminal(t, db, "student-rebuild-"+tc.name, "terminal-rebuild-"+tc.name,
			tc.state, tc.expires, tc.persistence, tc.status)
		runs[tc.name] = run
	}

	count, err := services.CleanupZombieScenarioSessions(db)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
	for _, tc := range cases {
		assert.Equal(t, tc.status, sessionStatus(t, db, runs[tc.name].ID),
			"a normal run whose container is gone (%s) is rebuildable and must stay open", tc.name)
	}
}

func TestCleanupZombieScenarioSessions_AbandonsGoneCrashTrapRun(t *testing.T) {
	db := freshTestDB(t)

	gone, _ := seedCrashTrapRunWithTerminal(t, db, "student-crash-gone", "terminal-crash-gone",
		terminalModels.StateDeleted, -time.Hour, "", "in_progress")
	paused, _ := seedCrashTrapRunWithTerminal(t, db, "student-crash-paused", "terminal-crash-paused",
		terminalModels.StateStopped, time.Hour, "persistent", "active")

	count, err := services.CleanupZombieScenarioSessions(db)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
	assert.Equal(t, "abandoned", sessionStatus(t, db, gone.ID),
		"a crash-trap run dies with its container: nothing is left to resume")
	assert.Equal(t, "active", sessionStatus(t, db, paused.ID),
		"a paused crash-trap run still holds its container")
}

func TestCleanupZombieScenarioSessions_AbandonsGonePreviewRun(t *testing.T) {
	db := freshTestDB(t)

	gone, _ := seedPreviewRunWithTerminal(t, db, "author-preview-gone", "terminal-preview-gone",
		terminalModels.StateRunning, -time.Hour, "ephemeral", "active")
	live, _ := seedPreviewRunWithTerminal(t, db, "author-preview-live", "terminal-preview-live",
		terminalModels.StateRunning, time.Hour, "ephemeral", "active")

	count, err := services.CleanupZombieScenarioSessions(db)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
	assert.Equal(t, "abandoned", sessionStatus(t, db, gone.ID),
		"a preview whose container is gone is not rebuilt; the author previews again")
	assert.Equal(t, "active", sessionStatus(t, db, live.ID))
}

// The pre-sweep sync exists so the sweep can tell a reaped container from a
// kept one. The sweep now only abandons crash-trap and preview runs, so
// syncing the owner of a normal run would cost a tt-backend round trip every
// five minutes for a decision nobody makes.
func TestOwnersToSyncBeforeSweep_OnlyCrashTrapAndPreviewOwners(t *testing.T) {
	db := freshTestDB(t)

	for _, owner := range []string{"owner-normal", "owner-crash", "owner-preview"} {
		seedPersistenceUserKey(t, db, owner)
	}
	seedOpenRunWithTerminal(t, db, "owner-normal", "terminal-sync-normal",
		terminalModels.StateRunning, -30*time.Minute, "persistent", "active")
	seedCrashTrapRunWithTerminal(t, db, "owner-crash", "terminal-sync-crash",
		terminalModels.StateRunning, -30*time.Minute, "persistent", "active")
	seedPreviewRunWithTerminal(t, db, "owner-preview", "terminal-sync-preview",
		terminalModels.StateRunning, -30*time.Minute, "persistent", "in_progress")

	owners, err := services.OwnersToSyncBeforeSweep(db)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"owner-crash", "owner-preview"}, owners,
		"only owners of runs the sweep could abandon are worth a sync")
}

// A rebuild runs in a goroutine. A process restart mid-replay leaves the row
// in provisioning/replay forever, and the stuck-provisioning reaper would then
// make it setup_failed — which the launch path abandons, so the learner loses
// the progress the rebuild was meant to keep. ReleaseStalledReplays, which the
// cron runs first, hands such a run back to the resume rule instead: open,
// no phase, and with its half-built terminal returned for deletion, so the
// next resume rebuilds again.
func TestReleaseStalledReplays_ReturnsRunToRebuildable(t *testing.T) {
	db := freshTestDB(t)

	seedReplay := func(name, phase string, age time.Duration) models.ScenarioSession {
		run, _ := seedOpenRunWithTerminal(t, db, "student-"+name, "terminal-"+name,
			terminalModels.StateRunning, time.Hour, "ephemeral", "provisioning")
		require.NoError(t, db.Model(&models.ScenarioSession{}).Where("id = ?", run.ID).
			Updates(map[string]any{"provisioning_phase": phase}).Error)
		require.NoError(t, db.Model(&models.ScenarioSession{}).Where("id = ?", run.ID).
			Update("updated_at", time.Now().Add(-age)).Error)
		return run
	}
	stalled := seedReplay("replay-stalled", "replay", 15*time.Minute)
	recent := seedReplay("replay-recent", "replay", 2*time.Minute)
	stalledLaunch := seedReplay("launch-stalled", "step_setup", 15*time.Minute)

	released, err := services.ReleaseStalledReplays(db)
	require.NoError(t, err)
	assert.Equal(t, []string{"terminal-replay-stalled"}, released,
		"the stalled replay's terminal is returned so the cron can delete it")

	var reloaded models.ScenarioSession
	require.NoError(t, db.First(&reloaded, "id = ?", stalled.ID).Error)
	assert.Equal(t, "active", reloaded.Status, "a stalled replay goes back to an open run, not setup_failed")
	assert.Equal(t, "", reloaded.ProvisioningPhase)

	assert.Equal(t, "provisioning", sessionStatus(t, db, recent.ID),
		"a replay still inside its budget is not stalled")
	assert.Equal(t, "provisioning", sessionStatus(t, db, stalledLaunch.ID),
		"a stalled launch has no progress to keep; it is the stuck-provisioning reaper's")

	// The reaper that follows in the same pass leaves the released run alone.
	_, err = services.CleanupStuckProvisioningSessions(db)
	require.NoError(t, err)
	assert.Equal(t, "active", sessionStatus(t, db, stalled.ID))

	// Once the cron has deleted the half-built terminal, the run is
	// rebuildable again.
	deleted := &terminalModels.Terminal{SessionID: "terminal-replay-stalled", State: terminalModels.StateDeleted}
	require.NoError(t, db.First(&reloaded, "id = ?", stalled.ID).Error)
	assert.Equal(t, services.ResumeModeRebuild, services.RunResumeMode(&reloaded, deleted, false))
}

// A paused terminal is stopped with its container kept until the reap
// deadline (expires_at moved forward by the stop). The run behind it is the
// one the learner will resume — abandoning it every five minutes made Pause
// end the scenario.
func TestCleanupZombieScenarioSessions_SparesPausedRun(t *testing.T) {
	db := freshTestDB(t)

	session, _ := seedOpenRunWithTerminal(t, db, "student-paused", "terminal-paused",
		terminalModels.StateStopped, time.Hour, "persistent", "active")

	count, err := services.CleanupZombieScenarioSessions(db)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
	assert.Equal(t, "active", sessionStatus(t, db, session.ID),
		"a paused run still holds its container and must stay open")
}

// tt-backend auto-stops a persistent terminal at its TTL, but nothing moves the
// local row off "running" until the next user-triggered sync. The container is
// kept (it is persistent), so the run is paused, not dead — the cron must not
// get there before the sync does.
func TestCleanupZombieScenarioSessions_SparesTTLStoppedPersistentRun(t *testing.T) {
	db := freshTestDB(t)

	session, _ := seedOpenRunWithTerminal(t, db, "student-ttl-persistent", "terminal-ttl-persistent",
		terminalModels.StateRunning, -30*time.Minute, "persistent", "in_progress")

	count, err := services.CleanupZombieScenarioSessions(db)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
	assert.Equal(t, "in_progress", sessionStatus(t, db, session.ID),
		"a persistent terminal auto-stopped at its TTL still holds its container; its run must stay open")
}

// TestZombieCleanupAgreesWithRunResumeMode pins the cron to the resume rule:
// for every open run it sweeps, it abandons exactly the ones RunResumeMode
// says cannot be resumed. They are two forms of one rule (SQL and Go), so this
// runs both over the same rows and fails if they ever disagree.
func TestZombieCleanupAgreesWithRunResumeMode(t *testing.T) {
	db := freshTestDB(t)

	type terminalCase struct {
		name        string
		state       terminalModels.TerminalState // "" = terminal row missing
		expires     time.Duration
		persistence string
	}
	terminalCases := []terminalCase{
		{"running-future-ephemeral", terminalModels.StateRunning, time.Hour, "ephemeral"},
		{"running-future-persistent", terminalModels.StateRunning, time.Hour, "persistent"},
		{"running-past-ephemeral", terminalModels.StateRunning, -time.Hour, "ephemeral"},
		{"running-past-persistent", terminalModels.StateRunning, -time.Hour, "persistent"},
		{"stopped-future-ephemeral", terminalModels.StateStopped, time.Hour, "ephemeral"},
		{"stopped-future-persistent", terminalModels.StateStopped, time.Hour, "persistent"},
		{"stopped-past-ephemeral", terminalModels.StateStopped, -time.Hour, "ephemeral"},
		{"stopped-past-persistent", terminalModels.StateStopped, -time.Hour, "persistent"},
		{"deleted-future-persistent", terminalModels.StateDeleted, time.Hour, "persistent"},
		{"revoked-future-persistent", terminalModels.StateRevoked, time.Hour, "persistent"},
		{"starting-future-persistent", terminalModels.StateStarting, time.Hour, "persistent"},
		{"missing-row", "", 0, ""},
	}
	statuses := []string{"active", "in_progress"}
	// The run's own axes: whether the scenario has crash traps, and whether
	// the run is a preview. Either one makes a gone container the end of it.
	type runKind struct {
		name       string
		crashTraps bool
		preview    bool
	}
	runKinds := []runKind{
		{"normal", false, false},
		{"crash", true, false},
		{"preview", false, true},
		{"crash-preview", true, true},
	}

	type seeded struct {
		session    models.ScenarioSession
		terminal   *terminalModels.Terminal
		crashTraps bool
	}
	var all []seeded
	for _, kind := range runKinds {
		for _, tc := range terminalCases {
			for _, status := range statuses {
				key := kind.name + "-" + tc.name + "-" + status
				session, terminal := seedOpenRunWithTerminal(t, db, "student-agree-"+key, "terminal-agree-"+key,
					tc.state, tc.expires, tc.persistence, status)
				if kind.crashTraps {
					require.NoError(t, db.Model(&models.Scenario{}).
						Where("id = ?", session.ScenarioID).Update("crash_traps", true).Error)
				}
				if kind.preview {
					require.NoError(t, db.Model(&models.ScenarioSession{}).
						Where("id = ?", session.ID).Update("is_preview", true).Error)
					session.IsPreview = true
				}
				all = append(all, seeded{session, terminal, kind.crashTraps})
			}
		}
	}

	// Judge every run before the cron rewrites any status.
	wantAbandoned := make(map[string]bool, len(all))
	paused, rebuild := 0, 0
	for i := range all {
		mode := services.RunResumeMode(&all[i].session, all[i].terminal, all[i].crashTraps)
		wantAbandoned[all[i].session.ID.String()] = mode == services.ResumeModeNone
		switch mode {
		case services.ResumeModePaused:
			paused++
		case services.ResumeModeRebuild:
			rebuild++
		}
	}
	// stopped-future ×2 persistence + running-past-persistent, each × 2 statuses,
	// × 4 run kinds. Without paused rows the matrix would only re-check the live rule.
	require.Equal(t, 24, paused, "RunResumeMode must report the paused rows of the matrix as paused")
	// The 7 gone-container terminal cases × 2 statuses, for normal runs only.
	// Without rebuild rows the matrix would not pin the sweep's new filter.
	require.Equal(t, 14, rebuild, "RunResumeMode must report the normal runs on gone containers as rebuild")

	_, err := services.CleanupZombieScenarioSessions(db)
	require.NoError(t, err)

	for i := range all {
		id := all[i].session.ID.String()
		abandoned := sessionStatus(t, db, all[i].session.ID) == "abandoned"
		assert.Equal(t, wantAbandoned[id], abandoned,
			"cron and RunResumeMode disagree about the run on %s", *all[i].session.TerminalSessionID)
	}
}

// A persistent terminal tt-backend auto-stopped at its TTL still reads
// "running" locally, and ContainerHeldScope spares its run because the
// container is normally kept. But tt-backend eventually reaps that container,
// and nothing tells ocf-core unless the learner comes back and triggers a
// sync — so the run stayed open forever, holding the one-run slot.
//
// The cron therefore asks OwnersToSyncBeforeSweep for the owners of exactly
// those terminals, syncs each through the one sync there is
// (SyncUserSessions), then sweeps. syncThenSweep does the same.
func runZombieCleanupAfterSync(t *testing.T, reportTargetsAs string) (*gorm.DB, map[string]models.ScenarioSession) {
	t.Helper()
	db := freshTestDB(t)

	// tt-backend's answer for the two target terminals: "gone" leaves them out
	// of the listing (reaped); "stopped" lists them as stopped with a fresh
	// idle deadline (the container is still kept).
	targets := []string{"terminal-ttl-active", "terminal-ttl-in-progress"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/sessions") {
			http.Error(w, "unexpected: "+r.Method+" "+r.URL.Path, http.StatusNotFound)
			return
		}
		sessions := []map[string]any{}
		if reportTargetsAs == "stopped" {
			for _, id := range targets {
				sessions = append(sessions, map[string]any{
					"id": id, "session_id": id, "name": id,
					"status":           1,
					"state":            "stopped",
					"persistence_mode": "persistent",
					"expires_at":       time.Now().Add(-30 * time.Minute).Unix(),
					"idle_until":       time.Now().Add(time.Hour).Unix(),
					"created_at":       time.Now().Add(-2 * time.Hour).Unix(),
				})
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sessions": sessions, "count": len(sessions), "include_expired": true, "limit": 1000,
		})
	}))
	t.Cleanup(srv.Close)
	configureTTServerForPersistence(t, srv.URL)

	runs := map[string]models.ScenarioSession{}
	seed := func(seedRun runSeeder, userID, terminalID string, state terminalModels.TerminalState, expires time.Duration, persistence, status string) {
		seedPersistenceUserKey(t, db, userID)
		run, _ := seedRun(t, db, userID, terminalID, state, expires, persistence, status)
		runs[terminalID] = run
	}
	// Targets: open crash-trap runs on running + persistent + past-expiry
	// terminals — the sweep abandons only runs that cannot be rebuilt.
	seed(seedCrashTrapRunWithTerminal, "owner-ttl-active", "terminal-ttl-active", terminalModels.StateRunning, -30*time.Minute, "persistent", "active")
	seed(seedCrashTrapRunWithTerminal, "owner-ttl-in-progress", "terminal-ttl-in-progress", terminalModels.StateRunning, -30*time.Minute, "persistent", "in_progress")
	// Not targets: nothing about these runs is stale in a way a sync resolves.
	seed(seedCrashTrapRunWithTerminal, "owner-paused", "terminal-paused", terminalModels.StateStopped, time.Hour, "persistent", "active")
	seed(seedCrashTrapRunWithTerminal, "owner-live", "terminal-live", terminalModels.StateRunning, time.Hour, "persistent", "in_progress")
	seed(seedCrashTrapRunWithTerminal, "owner-finished", "terminal-finished", terminalModels.StateRunning, -30*time.Minute, "persistent", "completed")
	seed(seedCrashTrapRunWithTerminal, "owner-ephemeral-dead", "terminal-ephemeral-dead", terminalModels.StateRunning, -30*time.Minute, "ephemeral", "active")
	// A normal run on the same kind of stale terminal: rebuildable whichever
	// way the sync goes, so its owner is not synced.
	seed(seedOpenRunWithTerminal, "owner-ttl-normal", "terminal-ttl-normal", terminalModels.StateRunning, -30*time.Minute, "persistent", "active")

	svc := terminalServices.NewTerminalTrainerService(db)
	synced := syncThenSweep(t, db, func(userID string) {
		_, syncErr := svc.SyncUserSessions(userID)
		assert.NoError(t, syncErr, "sync of %s", userID)
	})

	assert.ElementsMatch(t, []string{"owner-ttl-active", "owner-ttl-in-progress"}, synced,
		"only owners of running+persistent+past-expiry terminals behind active/in_progress crash-trap or preview runs are synced, each once")
	return db, runs
}

// syncThenSweep is the cron's pass: the owners OwnersToSyncBeforeSweep names
// are synced one by one, then the sweep runs. It returns those owners.
func syncThenSweep(t *testing.T, db *gorm.DB, sync func(userID string)) []string {
	t.Helper()
	owners, err := services.OwnersToSyncBeforeSweep(db)
	require.NoError(t, err)
	for _, userID := range owners {
		sync(userID)
	}
	_, err = services.CleanupZombieScenarioSessions(db)
	require.NoError(t, err)
	return owners
}

func TestCleanupZombieScenarioSessions_SyncsStaleTTLStoppedTerminalsFirst(t *testing.T) {
	db, runs := runZombieCleanupAfterSync(t, "gone")

	for _, terminalID := range []string{"terminal-ttl-active", "terminal-ttl-in-progress"} {
		var terminal terminalModels.Terminal
		require.NoError(t, db.Where("session_id = ?", terminalID).First(&terminal).Error)
		assert.Equal(t, terminalModels.StateDeleted, terminal.State,
			"tt-backend no longer lists %s: the sync must mark it deleted", terminalID)
		assert.Equal(t, "abandoned", sessionStatus(t, db, runs[terminalID].ID),
			"the run on reaped %s must be abandoned in the same sweep, not left open until the learner returns", terminalID)
	}
	assert.Equal(t, "active", sessionStatus(t, db, runs["terminal-paused"].ID))
	assert.Equal(t, "in_progress", sessionStatus(t, db, runs["terminal-live"].ID))
	assert.Equal(t, "completed", sessionStatus(t, db, runs["terminal-finished"].ID))
	assert.Equal(t, "active", sessionStatus(t, db, runs["terminal-ttl-normal"].ID),
		"a normal run is rebuildable whether or not tt-backend still keeps its container")
}

func TestCleanupZombieScenarioSessions_SyncKeepsRunWhoseContainerIsStillKept(t *testing.T) {
	db, runs := runZombieCleanupAfterSync(t, "stopped")

	for _, terminalID := range []string{"terminal-ttl-active", "terminal-ttl-in-progress"} {
		var terminal terminalModels.Terminal
		require.NoError(t, db.Where("session_id = ?", terminalID).First(&terminal).Error)
		assert.Equal(t, terminalModels.StateStopped, terminal.State,
			"tt-backend reports %s stopped with its container kept: the sync must record the pause", terminalID)
		assert.Equal(t, runs[terminalID].Status, sessionStatus(t, db, runs[terminalID].ID),
			"a run whose container tt-backend still keeps is paused, not dead")
	}
}

// A sync that could not reach tt-backend knows nothing, so it must change
// nothing. When it treated the failed listing as an empty one, every terminal
// of every synced owner was marked deleted — the stale one AND the live one —
// and the sweep that followed abandoned both runs: one outage ended every
// learner's scenario.
func TestCleanupZombieScenarioSessions_TTBackendDown_AbandonsNothing(t *testing.T) {
	db := freshTestDB(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "tt-backend is down", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	configureTTServerForPersistence(t, srv.URL)

	owner := "owner-during-outage"
	seedPersistenceUserKey(t, db, owner)
	staleRun, _ := seedCrashTrapRunWithTerminal(t, db, owner, "terminal-outage-stale",
		terminalModels.StateRunning, -30*time.Minute, "persistent", "active")
	liveRun, _ := seedCrashTrapRunWithTerminal(t, db, owner, "terminal-outage-live",
		terminalModels.StateRunning, time.Hour, "persistent", "active")

	svc := terminalServices.NewTerminalTrainerService(db)
	synced := syncThenSweep(t, db, func(userID string) {
		_, _ = svc.SyncUserSessions(userID) // the cron logs a failed sync and carries on
	})
	require.Equal(t, []string{owner}, synced, "the stale terminal's owner is synced — the outage is what this test is about")

	for _, id := range []string{"terminal-outage-stale", "terminal-outage-live"} {
		var terminal terminalModels.Terminal
		require.NoError(t, db.Where("session_id = ?", id).First(&terminal).Error)
		assert.Equal(t, terminalModels.StateRunning, terminal.State,
			"tt-backend could not be asked about %s; its row must not change", id)
	}
	assert.Equal(t, "active", sessionStatus(t, db, staleRun.ID), "the stale run must wait for a sync that succeeds")
	assert.Equal(t, "active", sessionStatus(t, db, liveRun.ID), "the live run must survive a tt-backend outage")
}

// SyncUserSessions refuses an owner without an active terminal key, so asking
// for them only produces an error per sweep, every five minutes, forever. Such
// owners are not synced; their runs are left as the sweep found them.
func TestCleanupZombieScenarioSessions_SkipsOwnersWithoutActiveKey(t *testing.T) {
	db := freshTestDB(t)

	seedPersistenceUserKey(t, db, "owner-with-key")
	seedCrashTrapRunWithTerminal(t, db, "owner-with-key", "terminal-with-key",
		terminalModels.StateRunning, -30*time.Minute, "persistent", "active")

	noKeyRun, _ := seedCrashTrapRunWithTerminal(t, db, "owner-no-key", "terminal-no-key",
		terminalModels.StateRunning, -30*time.Minute, "persistent", "active")

	seedPersistenceUserKey(t, db, "owner-inactive-key")
	// is_active carries gorm:"default:true", so false is written by an update.
	require.NoError(t, db.Model(&terminalModels.UserTerminalKey{}).
		Where("user_id = ?", "owner-inactive-key").Update("is_active", false).Error)
	inactiveKeyRun, _ := seedCrashTrapRunWithTerminal(t, db, "owner-inactive-key", "terminal-inactive-key",
		terminalModels.StateRunning, -30*time.Minute, "persistent", "in_progress")

	synced := syncThenSweep(t, db, func(string) {})

	assert.Equal(t, []string{"owner-with-key"}, synced,
		"only owners with an active terminal key can be synced")
	assert.Equal(t, "active", sessionStatus(t, db, noKeyRun.ID))
	assert.Equal(t, "in_progress", sessionStatus(t, db, inactiveKeyRun.ID))
}

// A soft-deleted run is not a run: the sweep's own UPDATE never sees it, so it
// must not make its owner eligible for a sync either.
func TestCleanupZombieScenarioSessions_SkipsOwnersOfSoftDeletedRuns(t *testing.T) {
	db := freshTestDB(t)

	seedPersistenceUserKey(t, db, "owner-open-run")
	seedCrashTrapRunWithTerminal(t, db, "owner-open-run", "terminal-open-run",
		terminalModels.StateRunning, -30*time.Minute, "persistent", "active")

	seedPersistenceUserKey(t, db, "owner-deleted-run")
	deletedRun, _ := seedCrashTrapRunWithTerminal(t, db, "owner-deleted-run", "terminal-deleted-run",
		terminalModels.StateRunning, -30*time.Minute, "persistent", "active")
	require.NoError(t, db.Delete(&deletedRun).Error)

	owners, err := services.OwnersToSyncBeforeSweep(db)
	require.NoError(t, err)
	assert.Equal(t, []string{"owner-open-run"}, owners,
		"a soft-deleted run must not make its owner eligible for a sync")
}
