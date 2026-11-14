package models

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ErrSessionStartBlocked refuses a session start to a participant blocked in
// the context the session would run in. Its text is the public error code.
var ErrSessionStartBlocked = errors.New("session_start_blocked")

// SessionStartBlockedError is the refusal with what the participant is told:
// the scope of the block that applies and the reason given for it.
type SessionStartBlockedError struct {
	Scope  string
	Reason string
}

func (e *SessionStartBlockedError) Error() string { return ErrSessionStartBlocked.Error() }

// Is makes errors.Is(err, ErrSessionStartBlocked) hold for every refusal.
func (e *SessionStartBlockedError) Is(target error) bool { return target == ErrSessionStartBlocked }

// Scopes a block can have.
const (
	SessionBlockScopeGroup  = "group"  // one class: its assignments and bulk starts
	SessionBlockScopeOrg    = "org"    // every session started in one organization
	SessionBlockScopeGlobal = "global" // every session, and the terminal key itself
)

// SessionBlock keeps a participant from starting sessions in one scope. ScopeID
// is the class or organization; nil for a global block. One row per user and
// scope: the composite unique index covers class and organization blocks, and
// MigrateSessionBlockGlobalIndex covers global ones, whose NULL scope_id the
// composite index would let repeat.
type SessionBlock struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id" mapstructure:"id"`
	UserID    string     `gorm:"not null;uniqueIndex:idx_session_blocks_user_scope,priority:1" json:"user_id" mapstructure:"user_id"`
	ScopeType string     `gorm:"not null;size:16;uniqueIndex:idx_session_blocks_user_scope,priority:2" json:"scope_type" mapstructure:"scope_type"`
	ScopeID   *uuid.UUID `gorm:"type:uuid;uniqueIndex:idx_session_blocks_user_scope,priority:3" json:"scope_id,omitempty" mapstructure:"scope_id"`
	Reason    string     `gorm:"size:500" json:"reason" mapstructure:"reason"`
	BlockedBy string     `json:"blocked_by" mapstructure:"blocked_by"`
	CreatedAt time.Time  `json:"created_at" mapstructure:"created_at"`
}

// BeforeCreate generates the ID when the caller did not set one.
func (b *SessionBlock) BeforeCreate(tx *gorm.DB) error {
	if b.ID == uuid.Nil {
		b.ID = uuid.New()
	}
	return nil
}

// MigrateSessionBlockGlobalIndex makes a global block unique per user. Same SQL
// on PostgreSQL and SQLite; idempotent.
func MigrateSessionBlockGlobalIndex(db *gorm.DB) error {
	return db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_session_blocks_user_global
		ON session_blocks (user_id) WHERE scope_type = 'global'`).Error
}

// IsSessionStartBlocked reports whether userID may not start a session in the
// organization context orgID (nil: their personal context). A global block
// applies everywhere and wins over an organization block; an organization block
// applies only to that organization. Class blocks never refuse a start here:
// they act on that class's assignments (ExcludeClassBlocked) and on its bulk
// starts (BlockedInClass).
func IsSessionStartBlocked(db *gorm.DB, userID string, orgID *uuid.UUID) (bool, string, string, error) {
	query := db.Model(&SessionBlock{}).Where("user_id = ?", userID)
	if orgID != nil {
		query = query.Where("scope_type = ? OR (scope_type = ? AND scope_id = ?)",
			SessionBlockScopeGlobal, SessionBlockScopeOrg, *orgID)
	} else {
		query = query.Where("scope_type = ?", SessionBlockScopeGlobal)
	}
	var blocks []SessionBlock
	if err := query.Select("scope_type", "reason").Find(&blocks).Error; err != nil {
		return false, "", "", err
	}
	if len(blocks) == 0 {
		return false, "", "", nil
	}
	chosen := blocks[0]
	for _, b := range blocks {
		if b.ScopeType == SessionBlockScopeGlobal {
			chosen = b
		}
	}
	return true, chosen.ScopeType, chosen.Reason, nil
}

// SessionStartRefusal is IsSessionStartBlocked as the error a start returns:
// nil when the start may go on.
func SessionStartRefusal(db *gorm.DB, userID string, orgID *uuid.UUID) error {
	blocked, scope, reason, err := IsSessionStartBlocked(db, userID, orgID)
	if err != nil {
		return err
	}
	if blocked {
		return &SessionStartBlockedError{Scope: scope, Reason: reason}
	}
	return nil
}

// BlockedInClass returns which of userIDs may not be started by a class-wide
// action on groupID, run in the organization context orgID: blocked in that
// class, in that organization, or on the whole platform. One query.
func BlockedInClass(db *gorm.DB, userIDs []string, groupID uuid.UUID, orgID *uuid.UUID) (map[string]bool, error) {
	blocked := make(map[string]bool)
	if len(userIDs) == 0 {
		return blocked, nil
	}
	scope := db.Where("scope_type = ?", SessionBlockScopeGlobal).
		Or("scope_type = ? AND scope_id = ?", SessionBlockScopeGroup, groupID)
	if orgID != nil {
		scope = scope.Or("scope_type = ? AND scope_id = ?", SessionBlockScopeOrg, *orgID)
	}
	var ids []string
	if err := db.Model(&SessionBlock{}).
		Where("user_id IN ?", userIDs).
		Where(scope).
		Distinct().Pluck("user_id", &ids).Error; err != nil {
		return nil, err
	}
	for _, id := range ids {
		blocked[id] = true
	}
	return blocked, nil
}

// ExcludeClassBlocked scopes a group_members query to the memberships whose
// user is not blocked in that class.
func ExcludeClassBlocked(db *gorm.DB) *gorm.DB {
	return db.Where(`NOT EXISTS (SELECT 1 FROM session_blocks sb
		WHERE sb.user_id = group_members.user_id AND sb.scope_type = ? AND sb.scope_id = group_members.group_id)`,
		SessionBlockScopeGroup)
}

// DeleteClassBlock removes userID's block in the class groupID: a class block
// belongs to the membership and leaves with it.
func DeleteClassBlock(db *gorm.DB, userID string, groupID uuid.UUID) error {
	return db.Where("user_id = ? AND scope_type = ? AND scope_id = ?", userID, SessionBlockScopeGroup, groupID).
		Delete(&SessionBlock{}).Error
}
