// tests/terminalTrainer/zombiePersistentResume_test.go
//
// Bug under test (Part B): a user with a persistent session whose local
// timer hit 0 clicks Resume within tt-backend's graceful auto-stop window
// (~5s SIGTERM grace). At that instant ocf-core's terminals.state is still
// 'running' from the last sync but ExpiresAt is in the past. Pre-fix,
// ValidateSessionAccess returned "expired" → middleware 410'd the Resume
// even though the session is resumable.
//
// Fix: ValidateSessionAccess treats state='running' + past-ExpiresAt as
// 'stopped' when PersistenceMode='persistent' — the auto-stop is in flight
// or just landed; the middleware's allowStopped branch must let Resume /
// Delete through. Ephemeral sessions stay 'expired' (correct — they're
// about to be destroyed).
package terminalTrainer_tests

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/terminalTrainer/models"
	"soli/formations/src/terminalTrainer/services"
)

// TestValidateSessionAccess_PersistentZombieRunning_ReturnsStopped pins the
// Part B contract: state='running' + past ExpiresAt + PersistenceMode='persistent'
// must be reported as "stopped", NOT "expired". This lets the lifecycle
// middleware's allowStopped branch pass through, so Resume on a zombie
// persistent session works during tt-backend's graceful auto-stop window.
func TestValidateSessionAccess_PersistentZombieRunning_ReturnsStopped(t *testing.T) {
	db := freshTestDB(t)
	svc := services.NewTerminalTrainerService(db)

	userKey, err := createTestUserKey(db, "zombie-user")
	require.NoError(t, err)

	terminal := &models.Terminal{
		SessionID:         "zombie-persistent-session",
		UserID:            "zombie-user",
		Name:              "Zombie Persistent",
		State:             "running",
		PersistenceMode:   "persistent",
		ExpiresAt:         time.Now().Add(-30 * time.Second), // past — auto-stop is in flight
		UserTerminalKeyID: userKey.ID,
	}
	require.NoError(t, db.Create(terminal).Error)

	isValid, reason, err := svc.ValidateSessionAccess(terminal.SessionID, false)

	assert.NoError(t, err)
	assert.False(t, isValid,
		"zombie persistent session must not be marked valid for direct access")
	assert.Equal(t, "stopped", reason,
		"persistent session with state='running' but past-ExpiresAt is a zombie — the auto-stop is in flight; report 'stopped' so middleware's allowStopped branch lets Resume succeed")
}

// TestValidateSessionAccess_EphemeralZombieRunning_StillReturnsExpired is the
// regression guard. Ephemeral sessions in the same shape MUST stay "expired" —
// they are not resumable; the backend is about to destroy them.
func TestValidateSessionAccess_EphemeralZombieRunning_StillReturnsExpired(t *testing.T) {
	db := freshTestDB(t)
	svc := services.NewTerminalTrainerService(db)

	userKey, err := createTestUserKey(db, "zombie-user-ephemeral")
	require.NoError(t, err)

	terminal := &models.Terminal{
		SessionID:         "zombie-ephemeral-session",
		UserID:            "zombie-user-ephemeral",
		Name:              "Zombie Ephemeral",
		State:             "running",
		PersistenceMode:   "ephemeral",
		ExpiresAt:         time.Now().Add(-30 * time.Second),
		UserTerminalKeyID: userKey.ID,
	}
	require.NoError(t, db.Create(terminal).Error)

	isValid, reason, err := svc.ValidateSessionAccess(terminal.SessionID, false)

	assert.NoError(t, err)
	assert.False(t, isValid)
	assert.Equal(t, "expired", reason,
		"ephemeral session with past-ExpiresAt is correctly 'expired' — no Resume affordance, the container is going away")
}
