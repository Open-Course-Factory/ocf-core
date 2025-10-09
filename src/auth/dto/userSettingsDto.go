package dto

import "time"

// UserSettingsOutput represents the full user settings response
type UserSettingsOutput struct {
	ID                   uint       `json:"id"`
	UserID               string     `json:"user_id"`
	DefaultLandingPage   string     `json:"default_landing_page"`
	PreferredLanguage    string     `json:"preferred_language"`
	Timezone             string     `json:"timezone"`
	Theme                string     `json:"theme"`
	CompactMode          bool       `json:"compact_mode"`
	EmailNotifications   bool       `json:"email_notifications"`
	DesktopNotifications bool       `json:"desktop_notifications"`
	PasswordLastChanged  *time.Time `json:"password_last_changed,omitempty"`
	TwoFactorEnabled     bool       `json:"two_factor_enabled"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

// UserSettingsInput for creating new settings (typically done automatically)
type UserSettingsInput struct {
	UserID               string  `json:"user_id" binding:"required"`
	DefaultLandingPage   *string `json:"default_landing_page"`
	PreferredLanguage    *string `json:"preferred_language"`
	Timezone             *string `json:"timezone"`
	Theme                *string `json:"theme"`
	CompactMode          *bool   `json:"compact_mode"`
	EmailNotifications   *bool   `json:"email_notifications"`
	DesktopNotifications *bool   `json:"desktop_notifications"`
}

// EditUserSettingsInput for partial updates (all fields optional)
type EditUserSettingsInput struct {
	DefaultLandingPage   *string `json:"default_landing_page"`
	PreferredLanguage    *string `json:"preferred_language"`
	Timezone             *string `json:"timezone"`
	Theme                *string `json:"theme"`
	CompactMode          *bool   `json:"compact_mode"`
	EmailNotifications   *bool   `json:"email_notifications"`
	DesktopNotifications *bool   `json:"desktop_notifications"`
}

// ChangePasswordInput for password change endpoint
type ChangePasswordInput struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required,min=8"`
	ConfirmPassword string `json:"confirm_password" binding:"required"`
}
