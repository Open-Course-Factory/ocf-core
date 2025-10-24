package models

import "time"

// TokenBlacklist stores invalidated JWT tokens
type TokenBlacklist struct {
	ID        uint      `gorm:"primaryKey"`
	TokenJTI  string    `gorm:"uniqueIndex;not null" json:"token_jti"`
	UserID    string    `gorm:"index;not null" json:"user_id"`
	ExpiresAt time.Time `gorm:"not null;index" json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
	Reason    string    `json:"reason"` // e.g., "password_change", "logout"
}
