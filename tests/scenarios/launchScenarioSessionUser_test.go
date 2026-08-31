// tests/scenarios/launchScenarioSessionUser_test.go
//
// Pins the uid a scenario's console runs as, on the wire.
//
// A scenario whose subject is file permissions cannot be played as root: root
// holds CAP_DAC_OVERRIDE, so it reads a mode-000 file and walks into a mode-000
// directory. The three chmod missions in GameShell taught nothing for exactly
// that reason, and one of them passed without the learner running chmod at all.
//
// So Scenario.SessionUser reaches tt-backend as session_user, and:
//
//  1. a scenario that names a uid sends it;
//  2. a scenario that names none sends no key at all, leaving the
//     distribution's own uid — which is every scenario that exists today, and
//     the arm that proves this change is inert for them;
//  3. a scenario that names 0 sends 0. Root is a choice, and a scenario that
//     says "root" must be distinguishable from one that says nothing, or the
//     field cannot be trusted in either direction.
//
// Witness: assertions read session_user from the JSON body the controller
// POSTs to the fake tt-backend's /1.0/sessions, not from a mock call count.
package scenarios_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"soli/formations/src/scenarios/models"
)

// seedSessionUserScenario is seedPersistenceScenario's sibling: same minimal
// launchable scenario, differing only in the uid it asks its console to run as.
func seedSessionUserScenario(t *testing.T, db *gorm.DB, userID string, sessionUser *int) *models.Scenario {
	t.Helper()
	scenario := &models.Scenario{
		Name:         "session-user-test-" + uuid.New().String(),
		Title:        "Session User Test",
		InstanceType: "M",
		OsType:       "deb",
		IsPublic:     true,
		CreatedByID:  userID,
		SessionUser:  sessionUser,
	}
	require.NoError(t, db.Create(scenario).Error)
	require.NoError(t, db.Create(&models.ScenarioStep{
		ScenarioID: scenario.ID,
		Order:      0,
		Title:      "Step 1",
		StepType:   "terminal",
	}).Error)
	return scenario
}

// launchWithSessionUser runs one launch and hands back the body tt-backend saw.
func launchWithSessionUser(t *testing.T, sessionUser *int) map[string]any {
	t.Helper()
	db := freshTestDB(t)
	userID := "launch-session-user-" + uuid.New().String()

	seedPersistencePlan(t, db, userID, false)
	seedPersistenceUserKey(t, db, userID)
	scenario := seedSessionUserScenario(t, db, userID, sessionUser)

	ttSrv, rec := newPersistenceTTBackend(t)
	configureTTServerForPersistence(t, ttSrv.URL)

	router := setupPersistenceRouter(t, db, userID)
	w := launchScenarioForTest(t, router, scenario.ID)

	require.Equal(t, http.StatusOK, w.Code,
		"launch must succeed; got %d. Body: %s", w.Code, w.Body.String())
	require.Equal(t, 1, rec.calls,
		"tt-backend /1.0/sessions must be reached exactly once")
	return rec.gotBody
}

func TestLaunchScenario_SendsTheScenariosSessionUser(t *testing.T) {
	learner := 1000
	body := launchWithSessionUser(t, &learner)

	assert.EqualValues(t, 1000, body["session_user"],
		"a scenario that names a uid must send it, or its console attaches as "+
			"root and every permission mission it teaches is decorative; body=%v", body)
}

func TestLaunchScenario_OmitsSessionUserWhenTheScenarioNamesNone(t *testing.T) {
	body := launchWithSessionUser(t, nil)

	_, present := body["session_user"]
	assert.False(t, present,
		"a scenario that names no uid must send no key, leaving the "+
			"distribution's own — this is every scenario that exists today; body=%v", body)
}

// Root is a choice, not the absence of one.
func TestLaunchScenario_SendsAnExplicitRoot(t *testing.T) {
	root := 0
	body := launchWithSessionUser(t, &root)

	value, present := body["session_user"]
	require.True(t, present,
		"an explicit 0 must reach the wire; dropping it makes \"run as root\" "+
			"indistinguishable from \"do not override\"; body=%v", body)
	assert.EqualValues(t, 0, value, "body=%v", body)
}
