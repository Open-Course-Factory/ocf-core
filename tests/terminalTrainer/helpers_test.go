package terminalTrainer_tests

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"soli/formations/src/terminalTrainer/models"
)

// setupTestDB returns a fresh shared DB with all rows cleaned.
// Kept as a thin wrapper for backward compatibility.
func setupTestDB(t *testing.T) *gorm.DB {
	return freshTestDB(t)
}

// createTestUserKey creates a test user key
func createTestUserKey(db *gorm.DB, userID string) (*models.UserTerminalKey, error) {
	userKey := &models.UserTerminalKey{
		UserID:      userID,
		APIKey:      "test-api-key-" + userID,
		KeyName:     "test-key-" + userID,
		IsActive:    true,
		MaxSessions: 5,
	}
	err := db.Create(userKey).Error
	return userKey, err
}

// createTestTerminal creates a test terminal session. The `lifecycleLabel`
// argument accepts both the canonical State values ("running"/"stopped"/
// "deleted") and the legacy convenience labels ("active"/"expired") that
// pre-date the SSOT consolidation — those are translated via labelToState
// so existing call sites keep compiling.
func createTestTerminal(db *gorm.DB, userID string, lifecycleLabel string, userKeyIDOrExpiry interface{}) (*models.Terminal, error) {
	// Support both old signature (userKeyID uuid.UUID) and new signature (expiresAt time.Time)
	var userKeyID uuid.UUID
	var expiresAt time.Time

	switch v := userKeyIDOrExpiry.(type) {
	case uuid.UUID:
		// Old signature - create a key if needed
		userKeyID = v
		expiresAt = time.Now().Add(time.Hour)
	case time.Time:
		// New signature - create a key
		userKey, err := createTestUserKey(db, userID)
		if err != nil {
			return nil, err
		}
		userKeyID = userKey.ID
		expiresAt = v
	default:
		// Default case - create a key
		userKey, err := createTestUserKey(db, userID)
		if err != nil {
			return nil, err
		}
		userKeyID = userKey.ID
		expiresAt = time.Now().Add(time.Hour)
	}

	terminal := &models.Terminal{
		SessionID:         "test-session-" + uuid.New().String(),
		UserID:            userID,
		Name:              "Test Terminal",
		State:             labelToState(lifecycleLabel),
		ExpiresAt:         expiresAt,
		InstanceType:      "test",
		MachineSize:       "S",
		UserTerminalKeyID: userKeyID,
	}
	err := db.Create(terminal).Error
	return terminal, err
}

// labelToState maps both legacy Status labels and canonical State values to
// State-space. Test call sites still use "active"/"expired" for readability;
// this keeps that ergonomic without resurrecting a parallel field.
func labelToState(label string) string {
	switch label {
	case "active":
		return "running"
	case "expired":
		return "deleted"
	default:
		// "running", "stopped", "deleted", "hibernating", ... all pass through.
		return label
	}
}



