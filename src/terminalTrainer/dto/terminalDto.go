package dto

import (
	"time"

	"github.com/google/uuid"
)

// Terminal DTOs for generic system
type CreateTerminalInput struct {
	SessionID            string    `binding:"required" json:"session_id"`
	UserID               string    `binding:"required" json:"user_id"`
	ExpiresAt            time.Time `binding:"required" json:"expires_at"`
	TerminalTrainerKeyID uuid.UUID `binding:"required" json:"terminal_trainer_key_id"`
}

type UpdateTerminalInput struct {
	Status string `json:"status,omitempty"`
}

type TerminalOutput struct {
	ID        uuid.UUID `json:"id"`
	SessionID string    `json:"session_id"`
	UserID    string    `json:"user_id"`
	Status    string    `json:"status"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// UserTerminalKey DTOs for generic system
type CreateUserTerminalKeyInput struct {
	UserID      string `binding:"required" json:"user_id"`
	KeyName     string `binding:"required" json:"key_name"`
	MaxSessions int    `json:"max_sessions"`
}

type UpdateUserTerminalKeyInput struct {
	KeyName     string `json:"key_name,omitempty"`
	IsActive    *bool  `json:"is_active,omitempty"`
	MaxSessions *int   `json:"max_sessions,omitempty"`
}

type UserTerminalKeyOutput struct {
	ID          uuid.UUID `json:"id"`
	UserID      string    `json:"user_id"`
	KeyName     string    `json:"key_name"`
	IsActive    bool      `json:"is_active"`
	MaxSessions int       `json:"max_sessions"`
	CreatedAt   time.Time `json:"created_at"`
}

// Terminal Service DTOs (pour les appels au Terminal Trainer)
type CreateTerminalSessionInput struct {
	Terms  string `binding:"required" json:"terms" form:"terms"`
	Expiry int    `json:"expiry,omitempty" form:"expiry"` // optionnel
}

type TerminalSessionResponse struct {
	SessionID  string    `json:"session_id"`
	ExpiresAt  time.Time `json:"expires_at"`
	ConsoleURL string    `json:"console_url"`
	Status     string    `json:"status"`
}

// Response du Terminal Trainer API (pour mapping)
type TerminalTrainerAPIKeyResponse struct {
	Success bool `json:"success"`
	Data    struct {
		ID                    int64  `json:"id"`
		KeyValue              string `json:"key_value"`
		Name                  string `json:"name"`
		IsAdmin               bool   `json:"is_admin"`
		IsActive              bool   `json:"is_active"`
		CreatedAt             int64  `json:"created_at"`
		UpdatedAt             int64  `json:"updated_at"`
		LastUsedAt            *int64 `json:"last_used_at"`
		MaxConcurrentSessions int    `json:"max_concurrent_sessions"`
	} `json:"data"`
	Message string `json:"message"`
}

type TerminalTrainerSessionResponse struct {
	SessionID string `json:"id"`
	Status    string `json:"status"`
	ExpiresAt int64  `json:"expires_at"` // timestamp Unix
	CreatedAt int64  `json:"created_at"`
}
