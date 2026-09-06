package terminalTrainer_tests

import (
	"fmt"
	"testing"

	"soli/formations/src/initialization"
	"soli/formations/src/terminalTrainer/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func isolatedUserTerminalKeyDB(t *testing.T, name string) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_busy_timeout=5000", name)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	require.NoError(t, db.AutoMigrate(&models.UserTerminalKey{}))
	return db
}

func reinstateMaxSessionsColumn(t *testing.T, db *gorm.DB) {
	t.Helper()
	if db.Migrator().HasColumn(&models.UserTerminalKey{}, "max_sessions") {
		return
	}
	require.NoError(t, db.Exec("ALTER TABLE user_terminal_keys ADD COLUMN max_sessions INTEGER DEFAULT 5").Error)
}

func TestDropOrphanUserTerminalKeyColumns_RemovesMaxSessions(t *testing.T) {
	db := isolatedUserTerminalKeyDB(t, "drop_max_sessions_removes")
	reinstateMaxSessionsColumn(t, db)
	require.True(t, db.Migrator().HasColumn(&models.UserTerminalKey{}, "max_sessions"),
		"precondition: the orphan column exists, as it does in prod")

	initialization.DropOrphanUserTerminalKeyColumns(db)

	assert.False(t, db.Migrator().HasColumn(&models.UserTerminalKey{}, "max_sessions"),
		"the orphan column must be gone after the migration")
}

func TestDropOrphanUserTerminalKeyColumns_Idempotent(t *testing.T) {
	db := isolatedUserTerminalKeyDB(t, "drop_max_sessions_idempotent")
	reinstateMaxSessionsColumn(t, db)

	for i := 0; i < 3; i++ {
		initialization.DropOrphanUserTerminalKeyColumns(db)
	}

	assert.False(t, db.Migrator().HasColumn(&models.UserTerminalKey{}, "max_sessions"))
}

func TestDropOrphanUserTerminalKeyColumns_KeyStillWritable(t *testing.T) {
	db := isolatedUserTerminalKeyDB(t, "drop_max_sessions_writable")
	reinstateMaxSessionsColumn(t, db)
	initialization.DropOrphanUserTerminalKeyColumns(db)

	key := models.UserTerminalKey{UserID: "user-1", APIKey: "key-1", KeyName: "k", IsActive: true}
	require.NoError(t, db.Create(&key).Error)

	var reloaded models.UserTerminalKey
	require.NoError(t, db.First(&reloaded, "id = ?", key.ID).Error)
	assert.Equal(t, "key-1", reloaded.APIKey)
	assert.True(t, reloaded.IsActive)
}

func TestUserTerminalKeyModel_HasNoMaxSessionsColumn(t *testing.T) {
	db := isolatedUserTerminalKeyDB(t, "model_no_max_sessions")

	assert.False(t, db.Migrator().HasColumn(&models.UserTerminalKey{}, "max_sessions"),
		"a model-built table must not carry the decorative max_sessions column")
}
