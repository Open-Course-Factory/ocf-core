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

// createTestTerminal creates a test terminal session (backwards compatible with old signature)
func createTestTerminal(db *gorm.DB, userID string, status string, userKeyIDOrExpiry interface{}) (*models.Terminal, error) {
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
		Status:            status,
		ExpiresAt:         expiresAt,
		InstanceType:      "test",
		MachineSize:       "S",
		UserTerminalKeyID: userKeyID,
		IsHiddenByOwner:   false,
	}
	err := db.Create(terminal).Error
	return terminal, err
}

// createTestTerminalShare creates a test terminal share
func createTestTerminalShare(db *gorm.DB, terminalID uuid.UUID, sharedByUserID, sharedWithUserID string) (*models.TerminalShare, error) {
	share := &models.TerminalShare{
		TerminalID:          terminalID,
		SharedWithUserID:    &sharedWithUserID,
		SharedByUserID:      sharedByUserID,
		AccessLevel:         models.AccessLevelRead,
		IsActive:            true,
		IsHiddenByRecipient: false,
	}
	err := db.Create(share).Error
	return share, err
}

// createTestGroupShare creates a test terminal share with a group
func createTestGroupShare(db *gorm.DB, terminalID uuid.UUID, sharedByUserID string, groupID uuid.UUID, accessLevel string) (*models.TerminalShare, error) {
	share := &models.TerminalShare{
		TerminalID:          terminalID,
		SharedWithGroupID:   &groupID,
		SharedByUserID:      sharedByUserID,
		AccessLevel:         accessLevel,
		IsActive:            true,
		IsHiddenByRecipient: false,
	}
	err := db.Create(share).Error
	return share, err
}

