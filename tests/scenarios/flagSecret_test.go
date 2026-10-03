// tests/scenarios/flagSecret_test.go
//
// Only seed, import and duplicate generated a flag secret. A scenario created
// in the editor and given flags later had none, so its flags were an HMAC
// under an empty key — computable by the learner from their own session id.
package scenarios_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

// startRunWithRealFlags starts a run through the real flag service and returns
// the flags stored for it.
func startRunWithRealFlags(t *testing.T, db *gorm.DB, scenarioID uuid.UUID) []models.ScenarioFlag {
	t.Helper()
	sessionSvc := services.NewScenarioSessionService(db, services.NewFlagService(), &mockVerificationService{})
	session, err := sessionSvc.StartScenario("learner-"+uuid.NewString(), scenarioID, "terminal-"+uuid.NewString(), "")
	require.NoError(t, err)

	var flags []models.ScenarioFlag
	require.NoError(t, db.Where("session_id = ?", session.ID).Find(&flags).Error)
	return flags
}

func TestFlagSecret_ScenarioWithoutSecret_GetsOneStoredBeforeItsFlags(t *testing.T) {
	db := freshTestDB(t)
	scenario := &models.Scenario{
		Name: "no-secret", Title: "No Secret", InstanceType: "ubuntu:22.04",
		CreatedByID: "editor-author", FlagsEnabled: true,
	}
	require.NoError(t, db.Create(scenario).Error)
	require.Empty(t, scenario.FlagSecret)
	require.NoError(t, db.Create(&models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 0, Title: "find it", StepType: "flag", HasFlag: true,
	}).Error)

	flags := startRunWithRealFlags(t, db, scenario.ID)
	require.Len(t, flags, 1)
	assert.Regexp(t, `^FLAG\{[0-9a-f]{16}\}$`, flags[0].ExpectedFlag)

	stored := reloadScenario(t, db, scenario.ID)
	assert.Len(t, stored.FlagSecret, 64, "an unkeyed HMAC would let the learner compute the flag")

	// The secret is kept, so a later run is keyed the same way.
	require.Len(t, startRunWithRealFlags(t, db, scenario.ID), 1)
	assert.Equal(t, stored.FlagSecret, reloadScenario(t, db, scenario.ID).FlagSecret)
}

func TestFlagSecret_ScenarioWithoutFlagSteps_GetsNoSecret(t *testing.T) {
	db := freshTestDB(t)
	scenario := &models.Scenario{
		Name: "no-flag-steps", Title: "No Flag Steps", InstanceType: "ubuntu:22.04",
		CreatedByID: "editor-author", FlagsEnabled: true,
	}
	require.NoError(t, db.Create(scenario).Error)
	require.NoError(t, db.Create(&models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 0, Title: "verify it", StepType: "terminal",
	}).Error)

	assert.Empty(t, startRunWithRealFlags(t, db, scenario.ID))
	assert.Empty(t, reloadScenario(t, db, scenario.ID).FlagSecret)
}
