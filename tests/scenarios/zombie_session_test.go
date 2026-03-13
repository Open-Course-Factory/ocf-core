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

// --- Fix 1: Defensive check in StartScenario ---

func TestStartScenario_ZombieSession_ExpiredTerminal_AutoAbandons(t *testing.T) {
	db := setupTestDB(t)

	// Create a scenario with one step
	scenario := models.Scenario{
		Name:         "zombie-expired",
		Title:        "Zombie Expired Terminal",
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

	// Create a UserTerminalKey (FK required by Terminal)
	utk := terminalModels.UserTerminalKey{
		UserID:   "student-zombie-1",
		APIKey:   "key-expired",
		KeyName:  "test-key",
		IsActive: true,
	}
	require.NoError(t, db.Create(&utk).Error)

	// Create an EXPIRED terminal with session_id = "terminal-old"
	oldTerminal := terminalModels.Terminal{
		SessionID:         "terminal-old",
		UserID:            "student-zombie-1",
		Status:            "expired",
		ExpiresAt:         time.Now().Add(-1 * time.Hour),
		InstanceType:      "ubuntu:22.04",
		UserTerminalKeyID: utk.ID,
	}
	require.NoError(t, db.Create(&oldTerminal).Error)

	flagSvc := &mockFlagService{}
	verifySvc := &mockVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, flagSvc, verifySvc)

	// Start a scenario session on the old (now expired) terminal — this should succeed
	session1, err := sessionSvc.StartScenario("student-zombie-1", scenario.ID, "terminal-old")
	require.NoError(t, err)
	require.NotNil(t, session1)
	assert.Equal(t, "active", session1.Status)

	// Create a new ACTIVE terminal
	utk2 := terminalModels.UserTerminalKey{
		UserID:   "student-zombie-1",
		APIKey:   "key-new",
		KeyName:  "test-key-new",
		IsActive: true,
	}
	require.NoError(t, db.Create(&utk2).Error)

	newTerminal := terminalModels.Terminal{
		SessionID:         "terminal-new",
		UserID:            "student-zombie-1",
		Status:            "active",
		ExpiresAt:         time.Now().Add(1 * time.Hour),
		InstanceType:      "ubuntu:22.04",
		UserTerminalKeyID: utk2.ID,
	}
	require.NoError(t, db.Create(&newTerminal).Error)

	// Try to start the SAME scenario again on the new terminal.
	// The old session's terminal is expired, so it should be auto-abandoned.
	session2, err := sessionSvc.StartScenario("student-zombie-1", scenario.ID, "terminal-new")
	require.NoError(t, err, "should auto-abandon zombie session with expired terminal")
	require.NotNil(t, session2)
	assert.Equal(t, "active", session2.Status)
	assert.NotEqual(t, session1.ID, session2.ID, "should be a new session")

	// Verify old session was marked abandoned
	var oldSession models.ScenarioSession
	require.NoError(t, db.First(&oldSession, "id = ?", session1.ID).Error)
	assert.Equal(t, "abandoned", oldSession.Status, "old zombie session should be auto-abandoned")
}

func TestStartScenario_ZombieSession_StoppedTerminal_AutoAbandons(t *testing.T) {
	db := setupTestDB(t)

	scenario := models.Scenario{
		Name:         "zombie-stopped",
		Title:        "Zombie Stopped Terminal",
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
		UserID:   "student-zombie-2",
		APIKey:   "key-stopped",
		KeyName:  "test-key",
		IsActive: true,
	}
	require.NoError(t, db.Create(&utk).Error)

	// Create a STOPPED terminal
	stoppedTerminal := terminalModels.Terminal{
		SessionID:         "terminal-stopped",
		UserID:            "student-zombie-2",
		Status:            "stopped",
		ExpiresAt:         time.Now().Add(-30 * time.Minute),
		InstanceType:      "ubuntu:22.04",
		UserTerminalKeyID: utk.ID,
	}
	require.NoError(t, db.Create(&stoppedTerminal).Error)

	flagSvc := &mockFlagService{}
	verifySvc := &mockVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, flagSvc, verifySvc)

	// Start a session on the stopped terminal
	session1, err := sessionSvc.StartScenario("student-zombie-2", scenario.ID, "terminal-stopped")
	require.NoError(t, err)
	require.NotNil(t, session1)

	// Create a new active terminal
	utk2 := terminalModels.UserTerminalKey{
		UserID:   "student-zombie-2",
		APIKey:   "key-new-2",
		KeyName:  "test-key-new-2",
		IsActive: true,
	}
	require.NoError(t, db.Create(&utk2).Error)

	newTerminal := terminalModels.Terminal{
		SessionID:         "terminal-new-2",
		UserID:            "student-zombie-2",
		Status:            "active",
		ExpiresAt:         time.Now().Add(1 * time.Hour),
		InstanceType:      "ubuntu:22.04",
		UserTerminalKeyID: utk2.ID,
	}
	require.NoError(t, db.Create(&newTerminal).Error)

	// Try to start the same scenario again — should auto-abandon the zombie
	session2, err := sessionSvc.StartScenario("student-zombie-2", scenario.ID, "terminal-new-2")
	require.NoError(t, err, "should auto-abandon zombie session with stopped terminal")
	require.NotNil(t, session2)
	assert.NotEqual(t, session1.ID, session2.ID)

	var oldSession models.ScenarioSession
	require.NoError(t, db.First(&oldSession, "id = ?", session1.ID).Error)
	assert.Equal(t, "abandoned", oldSession.Status)
}

func TestStartScenario_ZombieSession_DeletedTerminal_AutoAbandons(t *testing.T) {
	db := setupTestDB(t)

	scenario := models.Scenario{
		Name:         "zombie-deleted",
		Title:        "Zombie Deleted Terminal",
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

	flagSvc := &mockFlagService{}
	verifySvc := &mockVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, flagSvc, verifySvc)

	// Directly create a scenario session with a terminal_session_id that doesn't
	// match any Terminal record (simulating a deleted/soft-deleted terminal).
	orphanTerminalID := "terminal-ghost"
	zombieSession := models.ScenarioSession{
		ScenarioID:        scenario.ID,
		UserID:            "student-zombie-3",
		TerminalSessionID: &orphanTerminalID,
		CurrentStep:       0,
		Status:            "active",
		StartedAt:         time.Now().Add(-2 * time.Hour),
	}
	require.NoError(t, db.Create(&zombieSession).Error)

	// Create step progress so the session looks valid
	sp := models.ScenarioStepProgress{
		SessionID: zombieSession.ID,
		StepOrder: 0,
		Status:    "active",
	}
	require.NoError(t, db.Create(&sp).Error)

	// Now try to start a new session for the same user+scenario
	utk := terminalModels.UserTerminalKey{
		UserID:   "student-zombie-3",
		APIKey:   "key-ghost",
		KeyName:  "test-key",
		IsActive: true,
	}
	require.NoError(t, db.Create(&utk).Error)

	newTerminal := terminalModels.Terminal{
		SessionID:         "terminal-real",
		UserID:            "student-zombie-3",
		Status:            "active",
		ExpiresAt:         time.Now().Add(1 * time.Hour),
		InstanceType:      "ubuntu:22.04",
		UserTerminalKeyID: utk.ID,
	}
	require.NoError(t, db.Create(&newTerminal).Error)

	session2, err := sessionSvc.StartScenario("student-zombie-3", scenario.ID, "terminal-real")
	require.NoError(t, err, "should auto-abandon zombie session with deleted terminal")
	require.NotNil(t, session2)

	// Verify the zombie was abandoned
	var oldSession models.ScenarioSession
	require.NoError(t, db.First(&oldSession, "id = ?", zombieSession.ID).Error)
	assert.Equal(t, "abandoned", oldSession.Status)
}

func TestStartScenario_ActiveTerminal_StillBlocks(t *testing.T) {
	db := setupTestDB(t)

	scenario := models.Scenario{
		Name:         "active-blocks",
		Title:        "Active Terminal Blocks Duplicate",
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
		UserID:   "student-active-1",
		APIKey:   "key-active",
		KeyName:  "test-key",
		IsActive: true,
	}
	require.NoError(t, db.Create(&utk).Error)

	// Create an ACTIVE terminal
	activeTerminal := terminalModels.Terminal{
		SessionID:         "terminal-1",
		UserID:            "student-active-1",
		Status:            "active",
		ExpiresAt:         time.Now().Add(1 * time.Hour),
		InstanceType:      "ubuntu:22.04",
		UserTerminalKeyID: utk.ID,
	}
	require.NoError(t, db.Create(&activeTerminal).Error)

	flagSvc := &mockFlagService{}
	verifySvc := &mockVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, flagSvc, verifySvc)

	// Start a session on the active terminal — should succeed
	session1, err := sessionSvc.StartScenario("student-active-1", scenario.ID, "terminal-1")
	require.NoError(t, err)
	require.NotNil(t, session1)

	// Try to start the same scenario again on a different terminal.
	// The existing session's terminal is still active, so this is a legit duplicate.
	utk2 := terminalModels.UserTerminalKey{
		UserID:   "student-active-1",
		APIKey:   "key-active-2",
		KeyName:  "test-key-2",
		IsActive: true,
	}
	require.NoError(t, db.Create(&utk2).Error)

	otherTerminal := terminalModels.Terminal{
		SessionID:         "terminal-2",
		UserID:            "student-active-1",
		Status:            "active",
		ExpiresAt:         time.Now().Add(1 * time.Hour),
		InstanceType:      "ubuntu:22.04",
		UserTerminalKeyID: utk2.ID,
	}
	require.NoError(t, db.Create(&otherTerminal).Error)

	session2, err := sessionSvc.StartScenario("student-active-1", scenario.ID, "terminal-2")
	assert.Error(t, err, "should block when existing session has an active terminal")
	assert.Nil(t, session2)
	assert.Contains(t, err.Error(), "active session already exists")

	// Verify original session is untouched
	var originalSession models.ScenarioSession
	require.NoError(t, db.First(&originalSession, "id = ?", session1.ID).Error)
	assert.Equal(t, "active", originalSession.Status, "original session should remain active")
}
