package models

import (
	"time"

	"gorm.io/gorm"
)

type UserSettings struct {
	gorm.Model
	UserID string `gorm:"uniqueIndex;not null" json:"user_id"`

	// Navigation
	DefaultLandingPage string `gorm:"default:'/dashboard'" json:"default_landing_page"` // e.g., /dashboard, /courses, /terminals

	// Localization
	PreferredLanguage string `gorm:"default:'en'" json:"preferred_language"` // en, fr, es, etc.
	Timezone          string `gorm:"default:'UTC'" json:"timezone"`

	// UI Preferences
	Theme       string `gorm:"default:'light'" json:"theme"` // light, dark, auto
	CompactMode bool   `gorm:"default:false" json:"compact_mode"`

	// Notifications
	EmailNotifications   bool `gorm:"default:true" json:"email_notifications"`
	DesktopNotifications bool `gorm:"default:false" json:"desktop_notifications"`

	// Security (metadata only, not actual password)
	PasswordLastChanged *time.Time `json:"password_last_changed,omitempty"`
	TwoFactorEnabled    bool       `gorm:"default:false" json:"two_factor_enabled"`
}
