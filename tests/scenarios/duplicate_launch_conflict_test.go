package scenarios_test

import (
	"testing"

	"github.com/google/uuid"

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
func startScenarioWithLiveTerminal(t *testing.T, name string) (svc *services.ScenarioSessionService, scenario models.Scenario, userID string) {
	t.Helper()
	db := setupTestDB(t)

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
	// running, so the fixture has to carry a real terminal row.
	require.NoError(t, db.Create(&terminalModels.Terminal{
		SessionID: "live-terminal",
		UserID:    "student-1",
		State:     terminalModels.StateRunning,
	}).Error)

	svc = services.NewScenarioSessionService(db, &mockFlagService{}, &bgTrackingVerificationService{})
	_, err := svc.StartScenario("student-1", scenario.ID, "live-terminal")
	require.NoError(t, err)
	return svc, scenario, "student-1"
}

func TestStartScenarioWithLiveRunReturnsTypedConflict(t *testing.T) {
	svc, scenario, userID := startScenarioWithLiveTerminal(t, "dup-launch-conflict")

	_, err := svc.StartScenario(userID, scenario.ID, "")

	require.ErrorIs(t, err, services.ErrActiveSessionExists,
		"a second launch must be a conflict the caller can act on, not an opaque failure")
}

func TestResumableSessionsReportTheRunThatBlocksALaunch(t *testing.T) {
	svc, scenario, userID := startScenarioWithLiveTerminal(t, "dup-launch-listing")

	resumable, err := svc.GetResumableSessions(userID, []uuid.UUID{scenario.ID})

	require.NoError(t, err)
	require.NotNil(t, resumable[scenario.ID],
		"the listing must see the same run the launch path refuses for")
}
