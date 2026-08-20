// tests/scenarios/dropOrphanScenarioColumns_test.go
//
// Pins the guarded migration that removes scenarios.gsh_enabled, the column left
// behind when the in-terminal gsh helper was dropped and its Go field deleted.
// AutoMigrate adds columns and never drops them, so without this the column
// survives in prod forever.
//
// ISOLATION: these tests DROP a column, so they open their own in-memory DB
// rather than the package's shared one — dropping a column from sharedTestDB
// would corrupt every sibling test that touches scenarios.
//
// MECHANISM: GORM's Migrator().DropColumn is a silent no-op on
// gorm.io/driver/sqlite (returns nil, the column survives), so the migration
// issues a raw ALTER guarded by HasColumn. That is what makes it both effective
// on SQLite and idempotent everywhere; TestDropOrphanScenarioColumns_Idempotent
// is what would catch a regression back to DropColumn.
package scenarios

import (
	"fmt"
	"testing"

	"soli/formations/src/initialization"
	"soli/formations/src/scenarios/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// isolatedScenarioDB opens a private in-memory DB carrying only the scenarios
// table, which is all this migration touches.
func isolatedScenarioDB(t *testing.T, name string) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_busy_timeout=5000", name)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	require.NoError(t, db.AutoMigrate(&models.Scenario{}))
	return db
}

// reinstateGshColumn puts the column back the way prod has it: physically
// present, mapped by no Go field.
func reinstateGshColumn(t *testing.T, db *gorm.DB) {
	t.Helper()
	if db.Migrator().HasColumn(&models.Scenario{}, "gsh_enabled") {
		return
	}
	require.NoError(t, db.Exec("ALTER TABLE scenarios ADD COLUMN gsh_enabled BOOLEAN DEFAULT false").Error)
}

func TestDropOrphanScenarioColumns_RemovesGshEnabled(t *testing.T) {
	db := isolatedScenarioDB(t, "drop_gsh_removes")
	reinstateGshColumn(t, db)
	require.True(t, db.Migrator().HasColumn(&models.Scenario{}, "gsh_enabled"),
		"precondition: the orphan column exists, as it does in prod")

	initialization.DropOrphanScenarioColumns(db)

	assert.False(t, db.Migrator().HasColumn(&models.Scenario{}, "gsh_enabled"),
		"the orphan column must be gone after the migration")
}

func TestDropOrphanScenarioColumns_Idempotent(t *testing.T) {
	db := isolatedScenarioDB(t, "drop_gsh_idempotent")
	reinstateGshColumn(t, db)

	// Three passes: the column is there for the first and absent for the rest,
	// which is what every startup after the first one looks like.
	for i := 0; i < 3; i++ {
		initialization.DropOrphanScenarioColumns(db)
	}

	assert.False(t, db.Migrator().HasColumn(&models.Scenario{}, "gsh_enabled"))
}

// A database that never had the column — a fresh install — must survive the
// migration untouched rather than erroring on a DROP of something absent.
func TestDropOrphanScenarioColumns_FreshDatabaseUnaffected(t *testing.T) {
	db := isolatedScenarioDB(t, "drop_gsh_fresh")
	require.False(t, db.Migrator().HasColumn(&models.Scenario{}, "gsh_enabled"),
		"precondition: a model-built table has no gsh_enabled")

	initialization.DropOrphanScenarioColumns(db)

	assert.True(t, db.Migrator().HasColumn(&models.Scenario{}, "name"),
		"the rest of the table must be intact")
}

// The scenarios table keeps working for ordinary writes after the drop — the
// check that catches a DROP that took the wrong column with it.
func TestDropOrphanScenarioColumns_ScenarioStillWritable(t *testing.T) {
	db := isolatedScenarioDB(t, "drop_gsh_writable")
	reinstateGshColumn(t, db)
	initialization.DropOrphanScenarioColumns(db)

	scenario := models.Scenario{
		Name:         "post-migration-write",
		Title:        "Post migration write",
		InstanceType: "s",
		FlagsEnabled: true,
	}
	require.NoError(t, db.Create(&scenario).Error)

	var reloaded models.Scenario
	require.NoError(t, db.First(&reloaded, "id = ?", scenario.ID).Error)
	assert.Equal(t, "post-migration-write", reloaded.Name)
	assert.True(t, reloaded.FlagsEnabled)
}
