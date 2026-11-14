package test_tools

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// CreateSessionBlocksTable creates session_blocks the way the tests know it,
// written by hand so a test package can seed and count blocks without
// depending on the model. Keep it aligned with terminalTrainer/models.SessionBlock:
// one row per (user, scope type, scope id); scope_id is NULL for a global block,
// so a partial index keeps one global block per user, as
// models.MigrateSessionBlockGlobalIndex does in production.
func CreateSessionBlocksTable(db *gorm.DB) error {
	if err := db.Exec(`CREATE TABLE IF NOT EXISTS session_blocks (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		scope_type TEXT NOT NULL,
		scope_id TEXT,
		reason TEXT,
		blocked_by TEXT,
		created_at DATETIME
	)`).Error; err != nil {
		return err
	}
	if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_session_blocks_user_scope
		ON session_blocks (user_id, scope_type, scope_id)`).Error; err != nil {
		return err
	}
	return db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_session_blocks_user_global
		ON session_blocks (user_id) WHERE scope_type = 'global'`).Error
}

// SeedSessionBlock records a block on userID. scopeType is "group", "org" or
// "global"; scopeID is nil for "global".
func SeedSessionBlock(db *gorm.DB, userID, scopeType string, scopeID *uuid.UUID, reason string) error {
	var scope any
	if scopeID != nil {
		scope = scopeID.String()
	}
	return db.Exec(
		`INSERT INTO session_blocks (id, user_id, scope_type, scope_id, reason, blocked_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		uuid.NewString(), userID, scopeType, scope, reason, "test-blocker", time.Now(),
	).Error
}

// CountSessionBlocks returns how many blocks userID has, whatever their scope.
func CountSessionBlocks(db *gorm.DB, userID string) int64 {
	var n int64
	db.Table("session_blocks").Where("user_id = ?", userID).Count(&n)
	return n
}
