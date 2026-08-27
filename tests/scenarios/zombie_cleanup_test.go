package scenarios_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
	terminalModels "soli/formations/src/terminalTrainer/models"
)

// --- Fix 2: Cron cleanup job ---

func TestCleanupZombieScenarioSessions_AbandonsStaleSessions(t *testing.T) {
	db := setupTestDB(t)

	scenario := models.Scenario{
		Name:         "cleanup-test",
		Title:        "Cleanup Test",
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
		CurrentStep: 0, Status: "active", StartedAt: time.Now().Add(-3 * time.Hour),
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
		CurrentStep: 0, Status: "active", StartedAt: time.Now().Add(-2 * time.Hour),
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
		CurrentStep: 0, Status: "active", StartedAt: time.Now().Add(-30 * time.Minute),
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
	db := setupTestDB(t)

	scenario := models.Scenario{
		Name:         "cleanup-inprogress",
		Title:        "Cleanup In Progress Test",
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
		CurrentStep: 0, Status: "in_progress", StartedAt: time.Now().Add(-1 * time.Hour),
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
	db := setupTestDB(t)

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
		CurrentStep: 0, Status: "completed",
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
	db := setupTestDB(t)

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
	db := setupTestDB(t)

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
	db := setupTestDB(t)

	scenario := models.Scenario{
		Name:         "cleanup-expired-running",
		Title:        "Cleanup Expired Running",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
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
