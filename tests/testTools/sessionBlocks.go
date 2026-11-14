package test_tools

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"soli/formations/src/terminalTrainer/models"
)

// CreateSessionBlocksTable creates session_blocks with the production schema:
// the model's AutoMigrate, then the partial index that keeps one global block
// per user.
func CreateSessionBlocksTable(db *gorm.DB) error {
	if err := db.AutoMigrate(&models.SessionBlock{}); err != nil {
		return err
	}
	return models.MigrateSessionBlockGlobalIndex(db)
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
