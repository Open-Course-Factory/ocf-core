package impersonation_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	authModels "soli/formations/src/auth/models"
	"soli/formations/src/auth/services"
)

const (
	testImpersonator = "admin-1"
	testTarget       = "user-1"
	testIP           = "10.0.0.1"
	testUA           = "Mozilla/5.0 (test)"
)

func newImpersonationService(t *testing.T) (services.ImpersonationService, *gorm.DB) {
	t.Helper()
	db := freshTestDB(t)
	return services.NewImpersonationService(db), db
}

// --- StartSession ---

func TestImpersonationService_StartSession_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	svc, db := newImpersonationService(t)

	before := time.Now().Add(-1 * time.Second)
	session, err := svc.StartSession(testImpersonator, testTarget, testIP, testUA)
	require.NoError(t, err)
	require.NotNil(t, session)
	after := time.Now().Add(1 * time.Second)

	assert.Equal(t, testImpersonator, session.ImpersonatorID)
	assert.Equal(t, testTarget, session.TargetID)
	assert.Equal(t, testIP, session.ActorIP)
	assert.Equal(t, testUA, session.ActorUserAgent)
	assert.Nil(t, session.EndedAt, "active session must have nil EndedAt")
	assert.True(t, session.StartedAt.After(before) && session.StartedAt.Before(after))
	assert.True(t, session.LastActivityAt.After(before) && session.LastActivityAt.Before(after))

	// Confirm one row was persisted.
	var count int64
	require.NoError(t, db.Model(&authModels.ImpersonationSession{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestImpersonationService_StartSession_AlreadyActive_ReturnsError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	svc, db := newImpersonationService(t)

	_, err := svc.StartSession(testImpersonator, testTarget, testIP, testUA)
	require.NoError(t, err)

	// Second start for the same impersonator must be rejected.
	second, err := svc.StartSession(testImpersonator, "user-2", testIP, testUA)
	assert.Nil(t, second)
	assert.True(t, errors.Is(err, services.ErrAlreadyImpersonating),
		"expected ErrAlreadyImpersonating, got %v", err)

	// And no second row should be created.
	var count int64
	require.NoError(t, db.Model(&authModels.ImpersonationSession{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestImpersonationService_StartSession_SelfImpersonation_ReturnsError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	svc, db := newImpersonationService(t)

	session, err := svc.StartSession(testImpersonator, testImpersonator, testIP, testUA)
	assert.Nil(t, session)
	assert.True(t, errors.Is(err, services.ErrSelfImpersonation),
		"expected ErrSelfImpersonation, got %v", err)

	var count int64
	require.NoError(t, db.Model(&authModels.ImpersonationSession{}).Count(&count).Error)
	assert.Equal(t, int64(0), count, "no row should be created for self-impersonation")
}

func TestImpersonationService_StartSession_AfterStop_AllowsNewSession(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	svc, _ := newImpersonationService(t)

	_, err := svc.StartSession(testImpersonator, testTarget, testIP, testUA)
	require.NoError(t, err)

	require.NoError(t, svc.StopSession(testImpersonator, "manual"))

	// Now a fresh start should work.
	second, err := svc.StartSession(testImpersonator, "user-2", testIP, testUA)
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.Equal(t, "user-2", second.TargetID)
	assert.Nil(t, second.EndedAt)
}

func TestImpersonationService_PersistsActorContext(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	svc, db := newImpersonationService(t)

	_, err := svc.StartSession(testImpersonator, testTarget, "192.168.42.7", "curl/8.0")
	require.NoError(t, err)

	var stored authModels.ImpersonationSession
	require.NoError(t, db.Where("impersonator_id = ?", testImpersonator).First(&stored).Error)
	assert.Equal(t, "192.168.42.7", stored.ActorIP)
	assert.Equal(t, "curl/8.0", stored.ActorUserAgent)
}

// --- StopSession ---

func TestImpersonationService_StopSession_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	svc, db := newImpersonationService(t)

	_, err := svc.StartSession(testImpersonator, testTarget, testIP, testUA)
	require.NoError(t, err)

	before := time.Now().Add(-1 * time.Second)
	err = svc.StopSession(testImpersonator, "manual")
	require.NoError(t, err)
	after := time.Now().Add(1 * time.Second)

	var stored authModels.ImpersonationSession
	require.NoError(t, db.Where("impersonator_id = ?", testImpersonator).First(&stored).Error)
	require.NotNil(t, stored.EndedAt, "EndedAt must be set after StopSession")
	assert.True(t, stored.EndedAt.After(before) && stored.EndedAt.Before(after))
	assert.Equal(t, "manual", stored.EndReason)
}

func TestImpersonationService_StopSession_NoActive_ReturnsError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	svc, _ := newImpersonationService(t)

	err := svc.StopSession(testImpersonator, "manual")
	assert.True(t, errors.Is(err, services.ErrNoActiveSession),
		"expected ErrNoActiveSession, got %v", err)
}

// --- GetActiveSession ---

func TestImpersonationService_GetActiveSession_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	svc, _ := newImpersonationService(t)

	created, err := svc.StartSession(testImpersonator, testTarget, testIP, testUA)
	require.NoError(t, err)

	got, err := svc.GetActiveSession(testImpersonator)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, created.ID, got.ID)
	assert.Equal(t, testImpersonator, got.ImpersonatorID)
	assert.Equal(t, testTarget, got.TargetID)
	assert.Nil(t, got.EndedAt)
}

func TestImpersonationService_GetActiveSession_NoActive_ReturnsError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	svc, _ := newImpersonationService(t)

	got, err := svc.GetActiveSession(testImpersonator)
	assert.Nil(t, got)
	assert.True(t, errors.Is(err, services.ErrNoActiveSession),
		"expected ErrNoActiveSession, got %v", err)
}

func TestImpersonationService_GetActiveSession_IgnoresEndedSessions(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	svc, _ := newImpersonationService(t)

	// Start then stop a session.
	_, err := svc.StartSession(testImpersonator, testTarget, testIP, testUA)
	require.NoError(t, err)
	require.NoError(t, svc.StopSession(testImpersonator, "manual"))

	// GetActiveSession must not return the ended session.
	got, err := svc.GetActiveSession(testImpersonator)
	assert.Nil(t, got)
	assert.True(t, errors.Is(err, services.ErrNoActiveSession),
		"expected ErrNoActiveSession, got %v", err)
}

// --- Touch ---

func TestImpersonationService_Touch_UpdatesLastActivityAt(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	svc, db := newImpersonationService(t)

	created, err := svc.StartSession(testImpersonator, testTarget, testIP, testUA)
	require.NoError(t, err)
	originalActivity := created.LastActivityAt

	// Sleep briefly so the new timestamp will be observably different.
	time.Sleep(20 * time.Millisecond)

	require.NoError(t, svc.Touch(created.ID))

	var stored authModels.ImpersonationSession
	require.NoError(t, db.Where("id = ?", created.ID).First(&stored).Error)
	assert.True(t, stored.LastActivityAt.After(originalActivity),
		"Touch should bump LastActivityAt; original=%v, after=%v",
		originalActivity, stored.LastActivityAt)
}

func TestImpersonationService_Touch_OnEndedSession_ReturnsError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	svc, _ := newImpersonationService(t)

	created, err := svc.StartSession(testImpersonator, testTarget, testIP, testUA)
	require.NoError(t, err)
	require.NoError(t, svc.StopSession(testImpersonator, "manual"))

	err = svc.Touch(created.ID)
	assert.Error(t, err, "Touch on an ended session must return an error")
}

// --- ExpireStale ---

func TestImpersonationService_ExpireStale_ClosesIdleSessions(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	svc, db := newImpersonationService(t)

	// Stale session: started long ago, no recent activity.
	staleStart := time.Now().Add(-2 * time.Hour)
	staleActivity := time.Now().Add(-1 * time.Hour)
	stale := &authModels.ImpersonationSession{
		ImpersonatorID: "admin-stale",
		TargetID:       "user-stale",
		StartedAt:      staleStart,
		LastActivityAt: staleActivity,
	}
	require.NoError(t, db.Create(stale).Error)

	// Fresh session via the service (sets activity to now).
	freshSession, err := svc.StartSession("admin-fresh", "user-fresh", testIP, testUA)
	require.NoError(t, err)

	closed, err := svc.ExpireStale(30 * time.Minute)
	require.NoError(t, err)
	assert.Equal(t, 1, closed, "exactly one stale session should be closed")

	// Verify the stale session is now ended with reason="expired".
	var staleAfter authModels.ImpersonationSession
	require.NoError(t, db.Where("id = ?", stale.ID).First(&staleAfter).Error)
	require.NotNil(t, staleAfter.EndedAt, "stale session must be closed")
	assert.Equal(t, "expired", staleAfter.EndReason)

	// Fresh session must still be active.
	var freshAfter authModels.ImpersonationSession
	require.NoError(t, db.Where("id = ?", freshSession.ID).First(&freshAfter).Error)
	assert.Nil(t, freshAfter.EndedAt, "fresh session must remain active")
	assert.Empty(t, freshAfter.EndReason)
}

func TestImpersonationService_ExpireStale_NoStaleSessions_ReturnsZero(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	svc, _ := newImpersonationService(t)

	// Only fresh sessions exist.
	_, err := svc.StartSession(testImpersonator, testTarget, testIP, testUA)
	require.NoError(t, err)

	closed, err := svc.ExpireStale(30 * time.Minute)
	require.NoError(t, err)
	assert.Equal(t, 0, closed)
}

// --- Constant ---

func TestImpersonationService_IdleTimeoutConstant(t *testing.T) {
	// Spec contract: the idle timeout constant must exist and be 30 minutes.
	assert.Equal(t, 30*time.Minute, services.ImpersonationIdleTimeout)
}
