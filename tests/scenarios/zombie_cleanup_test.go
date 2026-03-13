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
//
// These tests are behind a build tag because CleanupZombieScenarioSessions
// does not exist yet. Remove the build tag once the function is implemented.

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
		Status: "expired", ExpiresAt: time.Now().Add(-2 * time.Hour),
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
		Status: "stopped", ExpiresAt: time.Now().Add(-1 * time.Hour),
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
		Status: "active", ExpiresAt: time.Now().Add(1 * time.Hour),
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
		Status: "expired", ExpiresAt: time.Now().Add(-2 * time.Hour),
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
