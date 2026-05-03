package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ImpersonationSession records a platform-administrator impersonating another
// user. A row is created when an admin starts impersonation and updated when
// the session ends (manually or via idle expiration).
type ImpersonationSession struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	ImpersonatorID string     `gorm:"index;not null" json:"impersonator_id"`
	TargetID       string     `gorm:"index;not null" json:"target_id"`
	StartedAt      time.Time  `gorm:"not null" json:"started_at"`
	LastActivityAt time.Time  `gorm:"not null;index" json:"last_activity_at"`
	EndedAt        *time.Time `gorm:"index" json:"ended_at,omitempty"`
	EndReason      string     `json:"end_reason,omitempty"`
	ActorIP        string     `json:"actor_ip,omitempty"`
	ActorUserAgent string     `json:"actor_user_agent,omitempty"`
}

// BeforeCreate generates a UUID if one was not provided and seeds the
// StartedAt / LastActivityAt timestamps so callers do not have to set them
// explicitly. Existing values are preserved (useful for deterministic tests
// and for backfill rows created with explicit timestamps).
func (s *ImpersonationSession) BeforeCreate(tx *gorm.DB) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}

	now := time.Now()
	if s.StartedAt.IsZero() {
		s.StartedAt = now
	}
	if s.LastActivityAt.IsZero() {
		s.LastActivityAt = now
	}

	return nil
}
