package models

import (
	"time"

	"gorm.io/gorm"
)

// PasswordResetToken stores password reset tokens for users
type PasswordResetToken struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	UserID    string    `gorm:"type:varchar(255);not null;index" json:"user_id"`
	Token     string    `gorm:"type:varchar(255);not null;uniqueIndex" json:"-"` // Never expose token in JSON
	ExpiresAt time.Time `gorm:"not null;index" json:"expires_at"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
}

// TableName specifies the table name for PasswordResetToken
func (PasswordResetToken) TableName() string {
	return "password_reset_tokens"
}

// IsExpired checks if the token has expired
func (t *PasswordResetToken) IsExpired() bool {
	return time.Now().After(t.ExpiresAt)
}

// IsUsed checks if the token has been used
func (t *PasswordResetToken) IsUsed() bool {
	return t.UsedAt != nil
}

// IsValid checks if the token is still valid (not expired and not used)
func (t *PasswordResetToken) IsValid() bool {
	return !t.IsExpired() && !t.IsUsed()
}
