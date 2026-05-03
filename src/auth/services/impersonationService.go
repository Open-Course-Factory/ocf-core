package services

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	authModels "soli/formations/src/auth/models"
)

// ImpersonationIdleTimeout is the maximum duration an impersonation session
// can remain idle before ExpireStale closes it. Touch resets the idle clock.
const ImpersonationIdleTimeout = 30 * time.Minute

// Sentinel errors returned by ImpersonationService. Callers should compare
// using errors.Is.
var (
	// ErrAlreadyImpersonating indicates the impersonator already has an
	// active session. Only one concurrent impersonation per admin is allowed.
	ErrAlreadyImpersonating = errors.New("impersonation: session already active for this user")

	// ErrSelfImpersonation indicates an attempt to impersonate oneself.
	ErrSelfImpersonation = errors.New("impersonation: cannot impersonate self")

	// ErrNoActiveSession indicates no active session was found for the
	// requested impersonator (or the session referenced by ID is already
	// ended).
	ErrNoActiveSession = errors.New("impersonation: no active session")
)

// ImpersonationService manages the lifecycle of platform-admin impersonation
// sessions: starting, stopping, idle-expiry, and activity tracking.
type ImpersonationService interface {
	StartSession(impersonatorID, targetID, ip, userAgent string) (*authModels.ImpersonationSession, error)
	StopSession(impersonatorID, reason string) error
	GetActiveSession(impersonatorID string) (*authModels.ImpersonationSession, error)
	Touch(sessionID uuid.UUID) error
	ExpireStale(idleDuration time.Duration) (int, error)
}

type impersonationService struct {
	db *gorm.DB
}

// NewImpersonationService builds an ImpersonationService backed by the given
// database handle.
func NewImpersonationService(db *gorm.DB) ImpersonationService {
	return &impersonationService{db: db}
}

// StartSession opens a new impersonation session for the given impersonator
// targeting another user. Returns ErrSelfImpersonation if the impersonator
// targets themselves, or ErrAlreadyImpersonating if a session is already
// active for the impersonator.
func (s *impersonationService) StartSession(impersonatorID, targetID, ip, userAgent string) (*authModels.ImpersonationSession, error) {
	if impersonatorID == targetID {
		return nil, ErrSelfImpersonation
	}

	// Reject if an active session already exists for this impersonator.
	if _, err := s.GetActiveSession(impersonatorID); err == nil {
		return nil, ErrAlreadyImpersonating
	} else if !errors.Is(err, ErrNoActiveSession) {
		return nil, err
	}

	session := &authModels.ImpersonationSession{
		ImpersonatorID: impersonatorID,
		TargetID:       targetID,
		ActorIP:        ip,
		ActorUserAgent: userAgent,
	}

	if err := s.db.Create(session).Error; err != nil {
		return nil, err
	}
	return session, nil
}

// StopSession ends the active session for the given impersonator with the
// supplied reason. Returns ErrNoActiveSession if no active session exists.
func (s *impersonationService) StopSession(impersonatorID, reason string) error {
	session, err := s.GetActiveSession(impersonatorID)
	if err != nil {
		return err
	}

	now := time.Now()
	updates := map[string]any{
		"ended_at":   &now,
		"end_reason": reason,
	}
	result := s.db.Model(&authModels.ImpersonationSession{}).
		Where("id = ?", session.ID).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

// GetActiveSession returns the active (not-yet-ended) session for the given
// impersonator, or ErrNoActiveSession if none exists.
func (s *impersonationService) GetActiveSession(impersonatorID string) (*authModels.ImpersonationSession, error) {
	var session authModels.ImpersonationSession
	err := s.db.
		Where("impersonator_id = ? AND ended_at IS NULL", impersonatorID).
		First(&session).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNoActiveSession
		}
		return nil, err
	}
	return &session, nil
}

// Touch updates LastActivityAt on the active session identified by sessionID.
// Returns ErrNoActiveSession if the session does not exist or has already
// ended.
func (s *impersonationService) Touch(sessionID uuid.UUID) error {
	now := time.Now()
	result := s.db.Model(&authModels.ImpersonationSession{}).
		Where("id = ? AND ended_at IS NULL", sessionID).
		Update("last_activity_at", now)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNoActiveSession
	}
	return nil
}

// ExpireStale closes all active sessions whose LastActivityAt is older than
// the given idle duration, marking them as ended with reason "expired".
// Returns the number of sessions closed.
func (s *impersonationService) ExpireStale(idleDuration time.Duration) (int, error) {
	now := time.Now()
	cutoff := now.Add(-idleDuration)

	result := s.db.Model(&authModels.ImpersonationSession{}).
		Where("ended_at IS NULL AND last_activity_at < ?", cutoff).
		Updates(map[string]any{
			"ended_at":   &now,
			"end_reason": "expired",
		})
	if result.Error != nil {
		return 0, result.Error
	}
	return int(result.RowsAffected), nil
}
