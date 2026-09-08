package terminalTrainer_tests

import (
	"testing"

	"soli/formations/src/initialization"
	"soli/formations/src/terminalTrainer/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func reinstateMaxSessionsColumn(t *testing.T, db *gorm.DB) {
	t.Helper()
	if db.Migrator().HasColumn(&models.UserTerminalKey{}, "max_sessions") {
		return
	}
	require.NoError(t, db.Exec("ALTER TABLE user_terminal_keys ADD COLUMN max_sessions INTEGER DEFAULT 5").Error)
}

func TestDropOrphanUserTerminalKeyColumns_RemovesMaxSessions(t *testing.T) {
	db := freshTestDB(t)
	reinstateMaxSessionsColumn(t, db)
	require.True(t, db.Migrator().HasColumn(&models.UserTerminalKey{}, "max_sessions"),
		"precondition: the orphan column exists, as it does in prod")

	initialization.DropOrphanUserTerminalKeyColumns(db)
	// A second run must be a no-op: the migration runs at every startup.
	initialization.DropOrphanUserTerminalKeyColumns(db)

	assert.False(t, db.Migrator().HasColumn(&models.UserTerminalKey{}, "max_sessions"),
		"the orphan column must be gone after the migration")

	key := models.UserTerminalKey{UserID: "user-1", APIKey: "key-1", KeyName: "k", IsActive: true}
	require.NoError(t, db.Create(&key).Error)
	var reloaded models.UserTerminalKey
	require.NoError(t, db.First(&reloaded, "id = ?", key.ID).Error)
	assert.Equal(t, "key-1", reloaded.APIKey)
	assert.True(t, reloaded.IsActive)
}
