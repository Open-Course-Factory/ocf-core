package scenarios_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/scenarios/models"
	scenarioController "soli/formations/src/scenarios/routes"
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
func waitForSessionStatus(t *testing.T, db interface {
	First(dest interface{}, conds ...interface{}) interface{ Error() error }
}, sessionID interface{}, targetStatuses []string, timeout time.Duration) string {
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
	session, err := sessionSvc.StartScenario("student-stop-1", scenario.ID, terminalID, "")
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

// TestAbandonSession_DestroysTerminal locks the contract that abandoning a
// scenario destroys the learner's container rather than stopping it. A stopped
// session keeps its slice of the plan's CPU/RAM budget — stop means "keep this
// machine for me" — so stopping on abandon drains the learner's allowance one
// abandoned run at a time until nothing will launch.
func TestAbandonSession_DestroysTerminal(t *testing.T) {
	db := setupTestDB(t)

	scenario := models.Scenario{
		Name:         "abandon-destroy-test",
		Title:        "Abandon Destroy Test",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-abandon-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	require.NoError(t, db.Create(&models.ScenarioStep{
		ScenarioID:  scenario.ID,
		Order:       0,
		Title:       "Step 1",
		TextContent: "First step",
	}).Error)

	terminalID := "terminal-abandon-1"
	session := models.ScenarioSession{
		ScenarioID:        scenario.ID,
		UserID:            "test-user-123",
		CurrentStep:       0,
		Status:            "active",
		StartedAt:         time.Now(),
		TerminalSessionID: &terminalID,
	}
	require.NoError(t, db.Create(&session).Error)

	ttMock := newMockTTService()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api/v1")
	api.Use(func(c *gin.Context) {
		c.Set("userId", "test-user-123")
		c.Set("userRoles", []string{"admin"})
		c.Next()
	})
	progressController := scenarioController.NewScenarioProgressControllerWithTerminalService(db, ttMock)
	api.POST("/scenario-sessions/:id/abandon", progressController.AbandonSession)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/scenario-sessions/"+session.ID.String()+"/abandon", nil)
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var dbSession models.ScenarioSession
	require.NoError(t, db.First(&dbSession, "id = ?", session.ID).Error)
	assert.Equal(t, "abandoned", dbSession.Status)

	assert.Equal(t, []string{terminalID}, ttMock.DeletedSessions(),
		"abandoning must destroy the learner's container, so its budget is released")
	assert.Empty(t, ttMock.StoppedSessions(),
		"a stopped container still holds plan budget — abandon must not merely stop it")
}

// panickingVerificationService panics on ExecInContainer to simulate an
// unexpected runtime fault inside runStep0Setup (e.g. a nil-deref from a
// malformed tt-backend response). VerifyStep and PushFile return zero values.
type panickingVerificationService struct{}

func (p *panickingVerificationService) VerifyStep(terminalSessionID string, step *models.ScenarioStep) (bool, string, error) {
	return false, "", nil
}

func (p *panickingVerificationService) PushFile(sessionID string, targetPath string, content string, mode string) error {
	return nil
}

// No console in these tests; the foreground path is exercised elsewhere.
func (p *panickingVerificationService) WriteToConsole(sessionID string, text string) error {
	return nil
}
func (p *panickingVerificationService) ExecInContainer(sessionID string, command []string, env map[string]string, timeout int) (int, string, string, error) {
	panic("simulated nil-deref inside ExecInContainer")
}

// TestRunStep0Setup_RecoversFromPanic_TransitionsToSetupFailed locks the
// contract that runStep0Setup MUST recover from any panic in its body so
// the ocf-core process does not crash. After recovery the session row must
// be marked status='setup_failed' with provisioning_phase=” and the linked
// terminal must be stopped via the configured TerminalStopFunc.
//
// Without `defer recover()` at the top of runStep0Setup, the goroutine
// spawned by StartScenario will crash the entire test binary when
// executeBackgroundScript panics — that is the RED state.
func TestRunStep0Setup_RecoversFromPanic_TransitionsToSetupFailed(t *testing.T) {
	db := setupTestDB(t)

	// Scenario with a non-empty setup script so runStep0Setup enters the
	// executeBackgroundScript branch and the panicking verification service
	// is invoked.
	scenario := models.Scenario{
		Name:         "panic-recovery-test",
		Title:        "Panic Recovery Test",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-panic-1",
		SetupScript:  "#!/bin/bash\necho setup",
	}
	require.NoError(t, db.Create(&scenario).Error)

	// One step is required so StartScenario spawns the runStep0Setup goroutine
	// (see scenarioSessionService.go: `if len(scenario.Steps) > 0`).
	step := models.ScenarioStep{
		ScenarioID:  scenario.ID,
		Order:       0,
		Title:       "Step 1",
		TextContent: "First step",
	}
	require.NoError(t, db.Create(&step).Error)

	flagSvc := &mockFlagService{}
	verifySvc := &panickingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, flagSvc, verifySvc)

	tracker := &terminalStopTracker{}
	sessionSvc.SetTerminalStopFunc(tracker.StopFunc())

	terminalID := "terminal-panic-recovery-1"
	session, err := sessionSvc.StartScenario("student-panic-1", scenario.ID, terminalID, "")
	require.NoError(t, err)
	require.Equal(t, "provisioning", session.Status,
		"session should be provisioning before the goroutine runs")

	// Poll the DB until the goroutine settles. With `defer recover()` in
	// place, the goroutine will catch the panic, update the row, and
	// invoke tryStopTerminal. Without it, the test binary will have
	// already crashed before reaching this line.
	deadline := time.Now().Add(5 * time.Second)
	var finalStatus, finalPhase string
	for time.Now().Before(deadline) {
		var dbSession models.ScenarioSession
		require.NoError(t, db.First(&dbSession, "id = ?", session.ID).Error)
		if dbSession.Status != "provisioning" {
			finalStatus = dbSession.Status
			finalPhase = dbSession.ProvisioningPhase
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	assert.Equal(t, "setup_failed", finalStatus,
		"runStep0Setup must mark the session as setup_failed after recovering from a panic")
	assert.Equal(t, "", finalPhase,
		"runStep0Setup must clear provisioning_phase after recovering from a panic")

	assert.GreaterOrEqual(t, tracker.CallCount(), 1,
		"runStep0Setup must call tryStopTerminal after recovering from a panic")
	assert.Contains(t, tracker.CalledWith(), terminalID,
		"tryStopTerminal must be invoked with the linked terminal session ID")
}
