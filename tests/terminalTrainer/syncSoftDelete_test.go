package terminalTrainer_tests

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/terminalTrainer/models"
	"soli/formations/src/terminalTrainer/repositories"
)

// TestSyncCreateSession_SoftDeletedExists_ShouldRestoreInsteadOfFail demonstrates a bug
// where the sync logic tries to INSERT a new Terminal record with the same session_id
// as a soft-deleted row. Because the unique index on session_id covers all rows
// (including soft-deleted ones), the INSERT fails with a unique constraint violation.
//
// Expected behavior: creating a terminal with a session_id that only exists in a
// soft-deleted row should succeed (either by restoring the row or by using a
// composite unique index that includes deleted_at).
//
// Actual behavior: CreateTerminalSession fails with a UNIQUE constraint error
// because the soft-deleted row still occupies the unique index slot.
func TestSyncCreateSession_SoftDeletedExists_ShouldRestoreInsteadOfFail(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	db := setupTestDB(t)
	repo := repositories.NewTerminalRepository(db)

	userKey, err := createTestUserKey(db, "sync-user-1")
	require.NoError(t, err)

	sessionID := "sync-session-" + uuid.New().String()

	// Step 1: Create a terminal with a specific session_id
	original := &models.Terminal{
		SessionID:         sessionID,
		UserID:            "sync-user-1",
		Name:              "Original Terminal",
		Status:            "active",
		ExpiresAt:         time.Now().Add(time.Hour),
		InstanceType:      "test",
		MachineSize:       "S",
		UserTerminalKeyID: userKey.ID,
	}
	err = repo.CreateTerminalSession(original)
	require.NoError(t, err, "initial creation should succeed")

	// Step 2: Soft-delete the terminal (simulates what happens when a session is cleaned up)
	err = db.Delete(original).Error
	require.NoError(t, err, "soft-delete should succeed")

	// Step 3: Verify the record is soft-deleted:
	// - Normal query (respects soft-delete) should NOT find it
	var notFound models.Terminal
	err = db.Where("session_id = ?", sessionID).First(&notFound).Error
	assert.Error(t, err, "normal query should not find soft-deleted record")

	// - Unscoped query should still find it (row exists with deleted_at set)
	var found models.Terminal
	err = db.Unscoped().Where("session_id = ?", sessionID).First(&found).Error
	require.NoError(t, err, "unscoped query should find soft-deleted record")
	assert.NotNil(t, found.DeletedAt, "deleted_at should be set")

	// Step 4: Try to create a NEW terminal with the same session_id.
	// This simulates what createMissingLocalSession does during sync:
	// the API returns a session that was previously soft-deleted locally,
	// and the sync logic tries to create it again.
	newTerminal := &models.Terminal{
		SessionID:         sessionID, // same session_id as the soft-deleted record
		UserID:            "sync-user-1",
		Name:              "Restored Terminal",
		Status:            "active",
		ExpiresAt:         time.Now().Add(2 * time.Hour),
		InstanceType:      "test",
		MachineSize:       "S",
		UserTerminalKeyID: userKey.ID,
	}

	// BUG: This fails with "UNIQUE constraint failed: terminals.session_id"
	// because the soft-deleted row still holds the unique index slot.
	err = repo.CreateTerminalSession(newTerminal)
	assert.NoError(t, err, "creating a terminal with a session_id that only exists in a soft-deleted row should succeed, but fails due to unique constraint violation (BUG)")
}

// TestSyncRepository_CreateTerminalSessionFromAPI_SoftDeletedExists_ShouldRestore verifies
// that CreateTerminalSessionFromAPI correctly restores a soft-deleted record instead of
// failing with a unique constraint violation.
//
// Previously, CreateTerminalSessionFromAPI called GetTerminalSessionBySessionID which
// uses a scoped GORM query (excludes soft-deleted rows), returned nil, then tried
// db.Create which failed because the soft-deleted row still occupies the unique index.
func TestSyncRepository_CreateTerminalSessionFromAPI_SoftDeletedExists_ShouldRestore(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	db := setupTestDB(t)
	repo := repositories.NewTerminalRepository(db)

	userKey, err := createTestUserKey(db, "sync-user-2")
	require.NoError(t, err)

	sessionID := "api-sync-session-" + uuid.New().String()

	// Step 1: Create a terminal with a specific session_id
	original := &models.Terminal{
		SessionID:         sessionID,
		UserID:            "sync-user-2",
		Name:              "Original API Terminal",
		Status:            "active",
		ExpiresAt:         time.Now().Add(time.Hour),
		InstanceType:      "test",
		MachineSize:       "S",
		UserTerminalKeyID: userKey.ID,
	}
	err = repo.CreateTerminalSession(original)
	require.NoError(t, err, "initial creation should succeed")

	// Step 2: Soft-delete it
	err = db.Delete(original).Error
	require.NoError(t, err, "soft-delete should succeed")

	// Step 3: Verify GetTerminalSessionBySessionID doesn't see the soft-deleted record
	// (this is part of the bug — the lookup misses the soft-deleted row)
	existing, err := repo.GetTerminalSessionBySessionID(sessionID)
	assert.NoError(t, err, "GetTerminalSessionBySessionID should not error")
	assert.Nil(t, existing, "GetTerminalSessionBySessionID should return nil for soft-deleted records (it doesn't check unscoped)")

	// Step 4: Try CreateTerminalSessionFromAPI — it will call GetTerminalSessionBySessionID
	// (which returns nil, since it can't see the soft-deleted row), then attempt db.Create,
	// which fails with a unique constraint violation.
	newTerminal := &models.Terminal{
		SessionID:         sessionID, // same session_id as the soft-deleted record
		UserID:            "sync-user-2",
		Name:              "Re-synced Terminal",
		Status:            "active",
		ExpiresAt:         time.Now().Add(2 * time.Hour),
		InstanceType:      "test",
		MachineSize:       "S",
		UserTerminalKeyID: userKey.ID,
	}

	// BUG: CreateTerminalSessionFromAPI thinks the session doesn't exist (because
	// GetTerminalSessionBySessionID doesn't see soft-deleted rows), so it tries to
	// INSERT — but the INSERT fails with "UNIQUE constraint failed: terminals.session_id".
	err = repo.CreateTerminalSessionFromAPI(newTerminal)
	assert.NoError(t, err, "CreateTerminalSessionFromAPI should handle soft-deleted records gracefully, but fails with unique constraint violation (BUG)")
}

// TestSyncCreateSession_NoSoftDelete_ShouldWork is a sanity check that creating a
// terminal with a brand-new session_id works correctly when no soft-deleted record
// exists. This confirms the test infrastructure is sound and the bug is specifically
// about the soft-delete + unique constraint interaction.
func TestSyncCreateSession_NoSoftDelete_ShouldWork(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	db := setupTestDB(t)
	repo := repositories.NewTerminalRepository(db)

	userKey, err := createTestUserKey(db, "sync-user-3")
	require.NoError(t, err)

	sessionID := "fresh-session-" + uuid.New().String()

	terminal := &models.Terminal{
		SessionID:         sessionID,
		UserID:            "sync-user-3",
		Name:              "Fresh Terminal",
		Status:            "active",
		ExpiresAt:         time.Now().Add(time.Hour),
		InstanceType:      "test",
		MachineSize:       "S",
		UserTerminalKeyID: userKey.ID,
	}

	// This should work — no prior record exists with this session_id
	err = repo.CreateTerminalSession(terminal)
	assert.NoError(t, err, "creating a terminal with a new session_id should succeed")

	// Verify it was created
	found, err := repo.GetTerminalSessionBySessionID(sessionID)
	assert.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, sessionID, found.SessionID)
	assert.Equal(t, "Fresh Terminal", found.Name)
}
