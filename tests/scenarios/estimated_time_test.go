package scenarios_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	scenarioModels "soli/formations/src/scenarios/models"
)

// The estimate used to be prose, and production held three spellings of it at
// once — "180 minutes", "10m", and empty. These are those spellings, plus the
// hour the column's own comment advertised.
func TestParseLegacyEstimatedTime_ReadsEverySpellingThatWasStored(t *testing.T) {
	cases := map[string]int{
		"180 minutes": 180,
		"90 minutes":  90,
		"10 minutes":  10,
		"10m":         10,
		"30m":         30,
		"1h":          60,
		"2 hours":     120,
		"45":          45,
		" 15m ":       15,
	}

	for text, expected := range cases {
		assert.Equal(t, expected, scenarioModels.ParseLegacyEstimatedTime(text),
			"reading %q", text)
	}
}

// Zero, not a guess. An unreadable estimate already looked exactly like an
// unset one, and inventing a number would put a duration in front of a learner
// that nobody wrote.
func TestParseLegacyEstimatedTime_UnreadableIsZero(t *testing.T) {
	for _, text := range []string{"", "   ", "a while", "quelques instants"} {
		assert.Equal(t, 0, scenarioModels.ParseLegacyEstimatedTime(text), "reading %q", text)
	}
}

// The migration must carry the value across before it drops the column it read
// from — the whole point of running it is that nothing is lost.
func TestMigrateEstimatedTime_CarriesTheValueAcrossThenDropsTheColumn(t *testing.T) {
	db := freshTestDB(t)

	require.NoError(t, db.Exec(`ALTER TABLE scenarios ADD COLUMN estimated_time varchar(100)`).Error)

	scenario := scenarioModels.Scenario{
		Name: "legacy-estimate", Title: "Legacy", InstanceType: "ubuntu:22.04",
	}
	require.NoError(t, db.Create(&scenario).Error)
	require.NoError(t, db.Exec(
		`UPDATE scenarios SET estimated_time = ? WHERE id = ?`, "90 minutes", scenario.ID).Error)

	scenarioModels.MigrateEstimatedTimeToMinutes(db)

	var migrated scenarioModels.Scenario
	require.NoError(t, db.First(&migrated, "id = ?", scenario.ID).Error)
	assert.Equal(t, 90, migrated.EstimatedTimeMinutes)

	// The drop itself can only be asserted where the driver performs it.
	// GORM's SQLite migrator returns no error and leaves the column in place,
	// so asserting it here would be asserting the driver, not the migration —
	// production is postgres, where it is a real ALTER.
	if db.Dialector.Name() == "postgres" {
		assert.False(t, db.Migrator().HasColumn(&scenarioModels.Scenario{}, "estimated_time"),
			"the prose column should be gone once its value has landed")
	}
}

// Running twice must be safe: the column it looks for is already gone, so there
// is nothing to read and nothing to drop.
func TestMigrateEstimatedTime_IsSafeToRunAgain(t *testing.T) {
	db := freshTestDB(t)

	scenario := scenarioModels.Scenario{
		Name: "already-migrated", Title: "Done", InstanceType: "ubuntu:22.04",
		EstimatedTimeMinutes: 45,
	}
	require.NoError(t, db.Create(&scenario).Error)

	scenarioModels.MigrateEstimatedTimeToMinutes(db)

	var untouched scenarioModels.Scenario
	require.NoError(t, db.First(&untouched, "id = ?", scenario.ID).Error)
	assert.Equal(t, 45, untouched.EstimatedTimeMinutes)
}
