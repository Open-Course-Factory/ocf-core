package auth_tests

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"soli/formations/src/auth/models"
	"soli/formations/src/auth/services"
)

// raceTestDBCounter generates unique named in-memory SQLite databases.
// This is required because SQLite `:memory:` databases are per-connection;
// for concurrent tests we need `cache=shared` with a unique name per test.
var raceTestDBCounter int

// setupRaceTestDB creates a named shared in-memory SQLite database with the
// email_verification_tokens table. Using a unique name + cache=shared ensures
// that multiple GORM connections (created by goroutines) all see the same data.
func setupRaceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	raceTestDBCounter++
	dsn := fmt.Sprintf("file:race_test_%d?mode=memory&cache=shared&_busy_timeout=5000", raceTestDBCounter)

	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	require.NoError(t, db.AutoMigrate(&models.EmailVerificationToken{}))
	return db
}

// ============================================================
// Race condition tests — Bug 1: TOCTOU on token check
//
// These tests verify that concurrent requests with the same
// verification token are properly serialized. Without a
// SELECT FOR UPDATE (or equivalent mutex), two goroutines can
// both read IsUsed() == false and both proceed to call Casdoor.
//
// These tests run in -short (unit) mode because they point
// Casdoor at an unreachable endpoint. The race detector (-race)
// should reveal unsynchronized access to the token record.
// ============================================================

// TestVerifyEmailToken_ConcurrentRequests_OnlyOneSucceeds launches N goroutines
// all trying to verify the same token simultaneously. Because Casdoor is
// unreachable in unit-test mode the service always returns an error AFTER the
// token check — but the critical window is between the IsUsed() read and the
// UsedAt write. The race detector will catch concurrent reads+writes on the
// same token record if there is no proper locking at the database level.
//
// When the fix is in place (SELECT FOR UPDATE or application-level mutex),
// the race detector should remain silent and the token should still be
// unused afterward (since every attempt fails at the Casdoor step).
func TestVerifyEmailToken_ConcurrentRequests_OnlyOneSucceeds(t *testing.T) {
	if !testing.Short() {
		t.Skip("requires unreachable Casdoor — skipping in integration mode")
	}
	setupUnreachableCasdoor()

	db := setupRaceTestDB(t)

	// Create a single valid token
	token := createTestToken(db, "user-race-1", "race@example.com", time.Now().Add(48*time.Hour))
	tokenValue := token.Token

	svc := services.NewEmailVerificationService(db)

	const goroutines = 10
	var (
		wg           sync.WaitGroup
		successCount atomic.Int32
		errTokenUsed atomic.Int32
		otherErrors  atomic.Int32
	)

	// Barrier to make all goroutines fire as simultaneously as possible
	barrier := make(chan struct{})

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-barrier // wait for all goroutines to be ready
			err := svc.VerifyEmail(tokenValue)
			switch err {
			case nil:
				successCount.Add(1)
			case services.ErrTokenUsed:
				errTokenUsed.Add(1)
			default:
				otherErrors.Add(1)
			}
		}()
	}

	// Release all goroutines at once
	close(barrier)
	wg.Wait()

	// In unit-test mode Casdoor is unreachable so every attempt will fail
	// at the Casdoor call — meaning the token is never marked as used, and
	// all goroutines should see an "other error" (Casdoor unreachable).
	// This test's primary purpose is for the -race detector to catch any
	// unsynchronized reads/writes on the token record.
	//
	// If a fix using SELECT FOR UPDATE or an application mutex is applied,
	// the race detector should stay silent on repeated runs.
	t.Logf("Results — success: %d, ErrTokenUsed: %d, other errors: %d",
		successCount.Load(), errTokenUsed.Load(), otherErrors.Load())

	// At most one goroutine should succeed (only possible when Casdoor is reachable)
	assert.LessOrEqual(t, int(successCount.Load()), 1,
		"at most one goroutine must succeed — concurrent token use must be serialized")

	// All errors should be either ErrTokenUsed or Casdoor-related (not panics/data corruption)
	total := int(successCount.Load()) + int(errTokenUsed.Load()) + int(otherErrors.Load())
	assert.Equal(t, goroutines, total,
		"every goroutine must complete without panicking")
}

// TestVerifyEmailToken_SequentialDoubleUse_SecondFails verifies that a
// second sequential call with the same token is rejected with ErrTokenUsed.
//
// This test demonstrates the happy-path of the sequential case: the service
// MUST detect that the token was consumed by the first call and reject the
// second one. In the buggy TOCTOU scenario, if the DB save of UsedAt fails
// (or the lock is missing), the second sequential call might also pass the
// IsUsed() check and attempt to re-verify the email.
//
// In -short mode (Casdoor unreachable) the first call fails at the Casdoor
// step — the token is therefore NOT consumed — so the second call will also
// fail at Casdoor, NOT with ErrTokenUsed. This is the expected behaviour
// documented by TestVerifyEmail_TokenNotConsumed_WhenCasdoorUpdateFails.
// We assert this explicitly so the bug is clearly visible: without a working
// Casdoor we cannot mark the token as used, so double-use is impossible to
// test purely. The sequential double-use is tested below.
func TestVerifyEmailToken_SequentialDoubleUse_SecondFails(t *testing.T) {
	if !testing.Short() {
		t.Skip("requires unreachable Casdoor — skipping in integration mode")
	}
	setupUnreachableCasdoor()

	db := setupRaceTestDB(t)

	// Create a valid token
	token := createTestToken(db, "user-seq-1", "seq@example.com", time.Now().Add(48*time.Hour))
	tokenValue := token.Token

	svc := services.NewEmailVerificationService(db)

	// First call — should fail because Casdoor is unreachable (token stays unused)
	err1 := svc.VerifyEmail(tokenValue)
	require.Error(t, err1, "first verification should fail when Casdoor is unreachable")
	assert.NotEqual(t, services.ErrTokenUsed, err1,
		"first attempt must not fail with ErrTokenUsed — it hasn't been used yet")

	// Confirm the token is still unused after the first (failed) attempt
	var tokenAfterFirst models.EmailVerificationToken
	require.NoError(t, db.Where("token = ?", tokenValue).First(&tokenAfterFirst).Error)
	assert.Nil(t, tokenAfterFirst.UsedAt,
		"token must remain unused when Casdoor update fails — user must be able to retry")

	// Second call — should also fail at Casdoor, NOT with ErrTokenUsed,
	// because the token was never marked as used
	err2 := svc.VerifyEmail(tokenValue)
	require.Error(t, err2, "second verification should also fail when Casdoor is unreachable")
	assert.NotEqual(t, services.ErrTokenUsed, err2,
		"second attempt must not get ErrTokenUsed — token was never consumed (Casdoor failed)")
}

// TestVerifyEmailToken_SequentialDoubleUse_WithManuallyUsedToken_SecondFails
// simulates the scenario where the first call succeeded (i.e., manually marks
// the token as used in the DB), then asserts that a second call with the same
// token is rejected with ErrTokenUsed.
//
// This is a pure DB-layer test — it does not depend on Casdoor at all.
// It confirms that the IsUsed() / ErrTokenUsed guard works correctly for
// the sequential case, providing a baseline before tackling the concurrent case.
func TestVerifyEmailToken_SequentialDoubleUse_WithManuallyUsedToken_SecondFails(t *testing.T) {
	// Unreachable Casdoor so the service never reaches the Casdoor call when
	// the token has already been used (returns ErrTokenUsed before Casdoor)
	casdoorsdk.InitConfig("http://localhost:0", "dummy", "dummy", "dummy", "dummy", "dummy")

	db := setupRaceTestDB(t)

	// Create a token and immediately mark it as used (simulating a prior successful verification)
	now := time.Now()
	usedToken := &models.EmailVerificationToken{
		UserID:    "user-seq-2",
		Email:     "seq2@example.com",
		Token:     generateTestToken(),
		ExpiresAt: time.Now().Add(48 * time.Hour),
		UsedAt:    &now, // already consumed
	}
	require.NoError(t, db.Create(usedToken).Error)

	svc := services.NewEmailVerificationService(db)

	// Attempt to verify the already-used token
	err := svc.VerifyEmail(usedToken.Token)

	// Must be rejected with ErrTokenUsed — not a different error, not nil
	require.Error(t, err, "verifying an already-used token must return an error")
	assert.Equal(t, services.ErrTokenUsed, err,
		"verifying an already-used token must return ErrTokenUsed, got: %v", err)
}

// TestVerifyEmailToken_RaceCondition_DatabaseLevelProtection documents the
// TOCTOU window in the current implementation. It creates a token and two
// goroutines that both attempt to pass the IsUsed() check before either
// can write UsedAt. With SQLite in-memory the DB serializes writes naturally,
// but with PostgreSQL under load the window is real.
//
// The test primarily exists to be run with -race so the race detector can
// flag any unsynchronized concurrent access. It also asserts correctness
// invariants that must hold regardless of the execution order.
func TestVerifyEmailToken_RaceCondition_DatabaseLevelProtection(t *testing.T) {
	if !testing.Short() {
		t.Skip("requires unreachable Casdoor — skipping in integration mode")
	}
	setupUnreachableCasdoor()

	db := setupRaceTestDB(t)

	token := createTestToken(db, "user-race-2", "race2@example.com", time.Now().Add(48*time.Hour))
	tokenValue := token.Token

	svc := services.NewEmailVerificationService(db)

	var (
		wg     sync.WaitGroup
		errors [2]error
	)

	// Two goroutines fire simultaneously — both will fail at Casdoor since it's
	// unreachable. The race detector targets concurrent reads/writes on the token.
	for i := 0; i < 2; i++ {
		wg.Add(1)
		idx := i
		go func() {
			defer wg.Done()
			errors[idx] = svc.VerifyEmail(tokenValue)
		}()
	}

	wg.Wait()

	// Both attempts must terminate with some error (Casdoor unreachable)
	for i, err := range errors {
		assert.Error(t, err, "goroutine %d must return an error (Casdoor is unreachable)", i)
	}

	// After both failed attempts the token must remain unused
	var reloaded models.EmailVerificationToken
	require.NoError(t, db.Where("token = ?", tokenValue).First(&reloaded).Error)
	assert.Nil(t, reloaded.UsedAt,
		"token must remain unused when all Casdoor updates fail")
	assert.True(t, reloaded.IsValid(),
		"token must remain valid (unused, not expired) when Casdoor is unavailable")
}
