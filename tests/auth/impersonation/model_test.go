package impersonation_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authModels "soli/formations/src/auth/models"
)

// TestImpersonationSession_BeforeCreate_GeneratesUUID verifies that
// inserting a session with a zero ID assigns a fresh UUID via the
// BeforeCreate GORM hook.
func TestImpersonationSession_BeforeCreate_GeneratesUUID(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)

	session := &authModels.ImpersonationSession{
		ImpersonatorID: "admin-user",
		TargetID:       "target-user",
	}
	require.True(t, session.ID == uuid.Nil, "precondition: ID should be zero before insert")

	err := db.Create(session).Error
	require.NoError(t, err)

	assert.NotEqual(t, uuid.Nil, session.ID, "BeforeCreate should generate a UUID")
}

// TestImpersonationSession_BeforeCreate_PreservesGivenUUID verifies that
// supplying an explicit UUID is honored (helps with deterministic tests).
func TestImpersonationSession_BeforeCreate_PreservesGivenUUID(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)

	explicit := uuid.New()
	session := &authModels.ImpersonationSession{
		ID:             explicit,
		ImpersonatorID: "admin-user",
		TargetID:       "target-user",
	}

	err := db.Create(session).Error
	require.NoError(t, err)

	assert.Equal(t, explicit, session.ID, "BeforeCreate should not overwrite an explicit UUID")
}

// TestImpersonationSession_BeforeCreate_SetsTimestamps verifies that
// StartedAt and LastActivityAt default to "now" when zero.
func TestImpersonationSession_BeforeCreate_SetsTimestamps(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)

	before := time.Now().Add(-1 * time.Second)
	session := &authModels.ImpersonationSession{
		ImpersonatorID: "admin-user",
		TargetID:       "target-user",
	}

	err := db.Create(session).Error
	require.NoError(t, err)
	after := time.Now().Add(1 * time.Second)

	assert.False(t, session.StartedAt.IsZero(), "StartedAt should be set")
	assert.False(t, session.LastActivityAt.IsZero(), "LastActivityAt should be set")
	assert.True(t, session.StartedAt.After(before) && session.StartedAt.Before(after),
		"StartedAt should be approximately now, got %v", session.StartedAt)
	assert.True(t, session.LastActivityAt.After(before) && session.LastActivityAt.Before(after),
		"LastActivityAt should be approximately now, got %v", session.LastActivityAt)
	assert.Nil(t, session.EndedAt, "EndedAt should be nil for an active session")
}
