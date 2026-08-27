package scenarios_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/stretchr/testify/require"

	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
	terminalModels "soli/formations/src/terminalTrainer/models"
)

// A learner who already has a live run of a scenario must be told to resume it,
// not handed a server error. These tests pin the two halves of that together:
// the launch path refuses with a typed error, and the listing reports the same
// run — so the card and the button cannot disagree about whether a launch is
// possible. They disagreed once, and the learner met a 500 on a card that
// still offered Launch.
func startScenarioWithLiveTerminal(t *testing.T, name string) (db *gorm.DB, svc *services.ScenarioSessionService, scenario models.Scenario, userID string) {
	t.Helper()
	db = setupTestDB(t)

	scenario = models.Scenario{
		Name:         name,
		Title:        name,
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)
	require.NoError(t, db.Create(&models.ScenarioStep{
		ScenarioID: scenario.ID,
		Order:      0,
		Title:      "Step 1",
	}).Error)

	// The rule turns on whether the terminal behind the session is still
	// alive, so the fixture has to carry a real terminal row — running AND
	// inside its TTL. An expires_at left at its zero value is a terminal that
	// died in year 1: it used to read as live here only because the rule
	// ignored expiry.
	require.NoError(t, db.Create(&terminalModels.Terminal{
		SessionID: "live-terminal",
		UserID:    "student-1",
		State:     terminalModels.StateRunning,
		ExpiresAt: time.Now().Add(time.Hour),
	}).Error)

	svc = services.NewScenarioSessionService(db, &mockFlagService{}, &bgTrackingVerificationService{})
	_, err := svc.StartScenario("student-1", scenario.ID, "live-terminal", "")
	require.NoError(t, err)
	return db, svc, scenario, "student-1"
}

func TestStartScenarioWithLiveRunReturnsTypedConflict(t *testing.T) {
	_, svc, scenario, userID := startScenarioWithLiveTerminal(t, "dup-launch-conflict")

	_, err := svc.StartScenario(userID, scenario.ID, "", "")

	require.ErrorIs(t, err, services.ErrActiveSessionExists,
		"a second launch must be a conflict the caller can act on, not an opaque failure")
}

func TestResumableSessionsReportTheRunThatBlocksALaunch(t *testing.T) {
	_, svc, scenario, userID := startScenarioWithLiveTerminal(t, "dup-launch-listing")

	resumable, err := svc.GetResumableSessions(userID, []uuid.UUID{scenario.ID})

	require.NoError(t, err)
	require.NotNil(t, resumable[scenario.ID],
		"the listing must see the same run the launch path refuses for")
}

// The other half of the same rule, and the one that actually reached learners:
// a terminal that reached its TTL keeps `state = "running"` because nothing
// tears it down, so a state-only check reported the run as live forever. The
// learner saw "a run is already in progress" for a container deleted hours
// earlier, and could neither resume it nor start another.
func startScenarioThenExpireItsTerminal(t *testing.T, name string) (*services.ScenarioSessionService, models.Scenario, string) {
	t.Helper()
	db, svc, scenario, userID := startScenarioWithLiveTerminal(t, name)

	require.NoError(t, db.Model(&terminalModels.Terminal{}).
		Where("session_id = ?", "live-terminal").
		Update("expires_at", time.Now().Add(-time.Hour)).Error)

	return svc, scenario, userID
}

func TestStartScenarioAfterTerminalExpiredIsAllowed(t *testing.T) {
	svc, scenario, userID := startScenarioThenExpireItsTerminal(t, "relaunch-after-expiry")

	_, err := svc.StartScenario(userID, scenario.ID, "", "")
	require.NoError(t, err, "a run whose terminal has expired must not block the next one")
}

func TestResumableSessionsOmitsRunOnExpiredTerminal(t *testing.T) {
	svc, scenario, userID := startScenarioThenExpireItsTerminal(t, "resumable-after-expiry")

	resumable, err := svc.GetResumableSessions(userID, []uuid.UUID{scenario.ID})
	require.NoError(t, err)
	require.Empty(t, resumable, "an expired terminal leaves nothing to resume")
}

func TestMySessionsReportsExpiredRunAsNotResumable(t *testing.T) {
	svc, _, userID := startScenarioThenExpireItsTerminal(t, "my-sessions-after-expiry")

	sessions, err := svc.GetMySessions(userID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.False(t, sessions[0].Resumable,
		"the launcher offers Resume from this flag; a dead terminal must not set it")
}
