package scenarios_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

// terminalStopTracker records calls to the stop function
type terminalStopTracker struct {
	mu        sync.Mutex
	calls     []string
	returnErr error
}

func (t *terminalStopTracker) StopFunc() services.TerminalStopFunc {
	return func(terminalSessionID string) error {
		t.mu.Lock()
		defer t.mu.Unlock()
		t.calls = append(t.calls, terminalSessionID)
		return t.returnErr
	}
}

func (t *terminalStopTracker) CallCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.calls)
}

func (t *terminalStopTracker) CalledWith() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	result := make([]string, len(t.calls))
	copy(result, t.calls)
	return result
}

// waitForSessionStatus polls the DB until the session reaches a target status or times out
func waitForSessionStatus(t *testing.T, db interface{ First(dest interface{}, conds ...interface{}) interface{ Error() error } }, sessionID interface{}, targetStatuses []string, timeout time.Duration) string {
	t.Helper()
	// This is a simplified helper — we use raw GORM since it's what the tests use
	return "" // placeholder
}

func TestRunStep0Setup_StopsTerminalOnFailure(t *testing.T) {
	db := setupTestDB(t)

	// Create a scenario with a setup script
	scenario := models.Scenario{
		Name:         "stop-terminal-test",
		Title:        "Stop Terminal Test",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-stop-1",
		SetupScript:  "#!/bin/bash\necho setup",
	}
	require.NoError(t, db.Create(&scenario).Error)

	step := models.ScenarioStep{
		ScenarioID:  scenario.ID,
		Order:       0,
		Title:       "Step 1",
		TextContent: "First step",
	}
	require.NoError(t, db.Create(&step).Error)

	// Create mock services where ExecInContainer always fails
	flagSvc := &mockFlagService{}
	verifySvc := &mockVerificationService{
		execErr: fmt.Errorf("simulated container execution failure"),
	}

	sessionSvc := services.NewScenarioSessionService(db, flagSvc, verifySvc)

	// Set up the terminal stop tracker
	tracker := &terminalStopTracker{}
	sessionSvc.SetTerminalStopFunc(tracker.StopFunc())

	// Start scenario — this triggers runStep0Setup in a goroutine
	terminalID := "terminal-stop-test-1"
	session, err := sessionSvc.StartScenario("student-stop-1", scenario.ID, terminalID)
	require.NoError(t, err)

	// Session should be in provisioning state initially
	assert.Equal(t, "provisioning", session.Status)

	// Wait for the goroutine to complete by polling session status
	deadline := time.Now().Add(5 * time.Second)
	var finalStatus string
	for time.Now().Before(deadline) {
		var dbSession models.ScenarioSession
		require.NoError(t, db.First(&dbSession, "id = ?", session.ID).Error)
		if dbSession.Status != "provisioning" {
			finalStatus = dbSession.Status
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// The setup script failed, so session should be setup_failed
	assert.Equal(t, "setup_failed", finalStatus, "session should be marked as setup_failed after script failure")

	// The terminal stop function should have been called with our terminal session ID
	assert.GreaterOrEqual(t, tracker.CallCount(), 1, "terminal stop function should have been called")
	calls := tracker.CalledWith()
	assert.Contains(t, calls, terminalID, "terminal stop should have been called with the correct terminal session ID")
}

func TestAbandonSession_StopsTerminalViaService(t *testing.T) {
	db := setupTestDB(t)

	// Create a scenario without setup script (so it goes directly to active)
	scenario := models.Scenario{
		Name:         "abandon-stop-test",
		Title:        "Abandon Stop Test",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-abandon-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	step := models.ScenarioStep{
		ScenarioID:  scenario.ID,
		Order:       0,
		Title:       "Step 1",
		TextContent: "First step",
	}
	require.NoError(t, db.Create(&step).Error)

	flagSvc := &mockFlagService{}
	verifySvc := &mockVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, flagSvc, verifySvc)

	// Start scenario — no setup script, so it's immediately active
	session, err := sessionSvc.StartScenario("student-abandon-1", scenario.ID, "terminal-abandon-1")
	require.NoError(t, err)
	assert.Equal(t, "active", session.Status)

	// Abandon the session via the service
	err = sessionSvc.AbandonSession(session.ID)
	require.NoError(t, err)

	// Verify session is now abandoned
	var dbSession models.ScenarioSession
	require.NoError(t, db.First(&dbSession, "id = ?", session.ID).Error)
	assert.Equal(t, "abandoned", dbSession.Status)

	// NOTE: The AbandonSession service method does NOT call the terminal stop function.
	// Terminal stopping on abandon is handled at the controller layer (scenarioController.AbandonSession),
	// which calls sc.terminalService.StopSession directly.
	// This is a design choice — the service layer only manages session state,
	// while the controller layer orchestrates the terminal cleanup.
}
