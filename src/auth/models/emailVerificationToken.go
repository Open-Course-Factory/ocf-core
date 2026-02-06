package models

import (
	"os"
	"strconv"
	"time"

	"gorm.io/gorm"
)

type EmailVerificationToken struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	UserID    string     `gorm:"type:varchar(255);not null;index" json:"user_id"`
	Email     string     `gorm:"type:varchar(255);not null;index" json:"email"`
	Token     string     `gorm:"type:varchar(255);not null;uniqueIndex" json:"-"` // Never expose
	ExpiresAt time.Time  `gorm:"not null;index" json:"expires_at"`                // 48 hours default
	UsedAt    *time.Time `json:"used_at,omitempty"`

	// Rate limiting
	ResendCount int        `gorm:"default:0" json:"-"`
	LastResent  *time.Time `json:"-"`
}

func (EmailVerificationToken) TableName() string {
	return "email_verification_tokens"
}

func (t *EmailVerificationToken) IsExpired() bool {
	return time.Now().After(t.ExpiresAt)
}

func (t *EmailVerificationToken) IsUsed() bool {
	return t.UsedAt != nil
}

func (t *EmailVerificationToken) IsValid() bool {
	return !t.IsExpired() && !t.IsUsed()
}

func (t *EmailVerificationToken) CanResend() bool {
	maxResends := 5
	if envMax := os.Getenv("EMAIL_VERIFICATION_MAX_RESENDS"); envMax != "" {
		if parsed, err := strconv.Atoi(envMax); err == nil && parsed > 0 {
			maxResends = parsed
		}
	}
	if t.ResendCount >= maxResends {
		return false
	}
	if t.LastResent == nil {
		return true
	}
	cooldown := 2 * time.Minute
	if envCooldown := os.Getenv("EMAIL_VERIFICATION_RESEND_COOLDOWN_MINUTES"); envCooldown != "" {
		if parsed, err := strconv.Atoi(envCooldown); err == nil && parsed > 0 {
			cooldown = time.Duration(parsed) * time.Minute
		}
	}
	return time.Since(*t.LastResent) > cooldown
}
