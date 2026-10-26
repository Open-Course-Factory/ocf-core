package models

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrAccountBlocked refuses a capability to an account an administrator (or an
// organization offboarding) has blocked. Its text is the public error code.
var ErrAccountBlocked = errors.New("user_blocked")

// BlockedAccount mirrors Casdoor's is_forbidden flag into the OCF database.
// Casdoor issues JWTs that ocf-core validates locally, so blocking sign-in
// there does nothing to a token already issued; the auth middleware checks
// this table on every request instead — a primary-key lookup, where asking
// Casdoor would be an HTTP call per request. The table, not an in-process
// cache, is the source of truth so every ocf-core replica sees a block at once.
//
// Written only by CasdoorUserClient.SetForbidden (src/auth/services), right
// after Casdoor persists the flag, and reconciled from Casdoor at startup.
type BlockedAccount struct {
	UserID    string    `gorm:"primaryKey" json:"user_id"`
	BlockedAt time.Time `gorm:"not null" json:"blocked_at"`
}

// SetAccountBlocked records or clears the block on userID. Idempotent both ways.
func SetAccountBlocked(db *gorm.DB, userID string, blocked bool) error {
	if !blocked {
		return db.Where("user_id = ?", userID).Delete(&BlockedAccount{}).Error
	}
	return db.Clauses(clause.OnConflict{DoNothing: true}).
		Create(&BlockedAccount{UserID: userID, BlockedAt: time.Now()}).Error
}

// IsAccountBlocked reports whether userID is blocked. The single definition of
// "blocked" every gate (auth middleware, terminal key creation) asks.
func IsAccountBlocked(db *gorm.DB, userID string) (bool, error) {
	var count int64
	err := db.Model(&BlockedAccount{}).Where("user_id = ?", userID).Count(&count).Error
	return count > 0, err
}
