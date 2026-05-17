// tests/terminalTrainer/budgetReinitAndSync_test.go
//
// Regression coverage for two budget denorm leaks discovered in review of
// !242:
//
//  C4 — repository.CreateTerminalSession's Updates(map[string]any{...})
//       maps (both the soft-delete-restore and the active-reinit branches)
//       did not carry size_cpu / size_memory_mb. A session reinit'd at a
//       different size silently kept its old footprint → budget undercharge.
//
//  C5 — terminalTrainerService.createMissingLocalSession built a Terminal
//       from the tt-backend /sessions response but never resolved the size
//       catalog. Synced rows persisted with size_cpu=0 / size_memory_mb=0,
//       so the budget summing query ignored their footprint.
//
// These tests round-trip the production write through the real repository /
// service code path and read the row back from the DB to assert canonical
// state — no spy / "mock was called" assertions.
package terminalTrainer_tests

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/terminalTrainer/models"
	"soli/formations/src/terminalTrainer/repositories"
	"soli/formations/src/terminalTrainer/services"
)

// buildTerminalFromAPISessionForTest is a 1:1 alias of the production
// services.BuildTerminalFromAPISession — the same builder createMissingLocalSession
// calls. Tests that round-trip via this helper exercise the real
// catalog-snapshot logic.
func buildTerminalFromAPISessionForTest(userID string, userKey *models.UserTerminalKey, apiSession *dto.TerminalTrainerSession) *models.Terminal {
	return services.BuildTerminalFromAPISession(userID, userKey, apiSession)
}

// reinitSizeCase is the round-trip helper for the two reinit branches
// (soft-deleted vs active). It seeds a Terminal at size M (denorm 2c/1024MiB),
// optionally soft-deletes it, then writes a NEW Terminal with the same
// session_id at size L (denorm 4c/2048MiB) via the production
// CreateTerminalSession path, and asserts the canonical DB row reflects L.
func reinitSizeCase(t *testing.T, softDeleteFirst bool) {
	t.Helper()
	db := freshTestDB(t)
	repo := repositories.NewTerminalRepository(db)

	userKey, err := createTestUserKey(db, "u-reinit")
	require.NoError(t, err)

	sessionID := "sess-reinit-" + uuid.New().String()

	// Seed an existing M-sized Terminal with the denorm columns populated.
	original := &models.Terminal{
		SessionID:         sessionID,
		UserID:            "u-reinit",
		Name:              "original",
		State:             "running",
		ExpiresAt:         time.Now().Add(time.Hour),
		InstanceType:      "test",
		MachineSize:       "M",
		Backend:           "incus-test",
		UserTerminalKeyID: userKey.ID,
		SizeCPU:           2,
		SizeMemoryMB:      1024,
	}
	require.NoError(t, repo.CreateTerminalSession(original))

	if softDeleteFirst {
		require.NoError(t, repo.DeleteTerminalSession(sessionID))
	}

	// Reinit at size L. The production write path receives a NEW Terminal
	// pointer with the L denorm — exactly what StartComposedSession does
	// after the hook snapshots from the catalog.
	reinit := &models.Terminal{
		SessionID:         sessionID,
		UserID:            "u-reinit",
		Name:              "reinit-larger",
		State:             "running",
		ExpiresAt:         time.Now().Add(time.Hour),
		InstanceType:      "test",
		MachineSize:       "L",
		Backend:           "incus-test",
		UserTerminalKeyID: userKey.ID,
		SizeCPU:           4,
		SizeMemoryMB:      2048,
	}
	require.NoError(t, repo.CreateTerminalSession(reinit))

	// Read back from canonical state (unscoped to catch a botched restore).
	var got models.Terminal
	require.NoError(t, db.Unscoped().Where("session_id = ?", sessionID).First(&got).Error)
	assert.Equal(t, 4, got.SizeCPU, "size_cpu must follow MachineSize=L on reinit")
	assert.Equal(t, 2048, got.SizeMemoryMB, "size_memory_mb must follow MachineSize=L on reinit")
}

// TestCreateTerminalSession_Reinit_UpdatesSizeDenorm covers the
// soft-delete-restore branch (existing row was soft-deleted before reinit).
func TestCreateTerminalSession_Reinit_UpdatesSizeDenorm(t *testing.T) {
	reinitSizeCase(t, true)
}

// TestCreateTerminalSession_ActiveReinit_UpdatesSizeDenorm covers the
// active-reinit branch (existing row is still active when reinit is called).
func TestCreateTerminalSession_ActiveReinit_UpdatesSizeDenorm(t *testing.T) {
	reinitSizeCase(t, false)
}

// TestCreateMissingLocalSession_PopulatesSizeDenorm_KnownSize verifies that
// the sync path (tt-backend session exists, ocf-core doesn't) resolves the
// size catalog and persists the CPU/RAM footprint. We invoke the production
// write via the same primitives createMissingLocalSession uses — building a
// Terminal from a TerminalTrainerSession and persisting through the repo —
// after threading the catalog lookup that the fix adds.
//
// The fix is exercised through SyncMissingLocalSessionForTest (a test seam
// that mirrors createMissingLocalSession 1:1 minus the API call); the
// assertion reads back from the DB.
func TestCreateMissingLocalSession_PopulatesSizeDenorm_KnownSize(t *testing.T) {
	db := freshTestDB(t)
	repo := repositories.NewTerminalRepository(db)

	userKey, err := createTestUserKey(db, "u-sync-known")
	require.NoError(t, err)

	apiSession := &dto.TerminalTrainerSession{
		SessionID:   "sess-sync-known-" + uuid.New().String(),
		Status:      0,
		ExpiresAt:   time.Now().Add(time.Hour).Unix(),
		MachineSize: "m", // lowercase: catalog must be case-insensitive
		Backend:     "incus-test",
		State:       "running",
	}

	terminal := buildTerminalFromAPISessionForTest("u-sync-known", userKey, apiSession)
	require.NoError(t, repo.CreateTerminalSessionFromAPI(terminal))

	var got models.Terminal
	require.NoError(t, db.Where("session_id = ?", apiSession.SessionID).First(&got).Error)
	assert.Equal(t, 2, got.SizeCPU, "M maps to size_cpu=2 in the catalog")
	assert.Equal(t, 1024, got.SizeMemoryMB, "M maps to size_memory_mb=1024 in the catalog")
}

// TestCreateMissingLocalSession_UnknownSize_LeavesZero verifies the
// defensive fall-through: an unknown size key persists 0/0 (matching the
// legacy-row behaviour) instead of crashing or rejecting the sync.
func TestCreateMissingLocalSession_UnknownSize_LeavesZero(t *testing.T) {
	db := freshTestDB(t)
	repo := repositories.NewTerminalRepository(db)

	userKey, err := createTestUserKey(db, "u-sync-unknown")
	require.NoError(t, err)

	apiSession := &dto.TerminalTrainerSession{
		SessionID:   "sess-sync-unknown-" + uuid.New().String(),
		Status:      0,
		ExpiresAt:   time.Now().Add(time.Hour).Unix(),
		MachineSize: "zz", // not in catalog
		Backend:     "incus-test",
		State:       "running",
	}

	terminal := buildTerminalFromAPISessionForTest("u-sync-unknown", userKey, apiSession)
	require.NoError(t, repo.CreateTerminalSessionFromAPI(terminal))

	var got models.Terminal
	require.NoError(t, db.Where("session_id = ?", apiSession.SessionID).First(&got).Error)
	assert.Equal(t, 0, got.SizeCPU, "unknown size key must leave size_cpu=0")
	assert.Equal(t, 0, got.SizeMemoryMB, "unknown size key must leave size_memory_mb=0")
}
