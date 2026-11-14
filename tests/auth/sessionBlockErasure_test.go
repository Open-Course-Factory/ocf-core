// tests/auth/sessionBlockErasure_test.go
//
// Session start blocks (#579) are personal data: erasing an account erases
// every block on it, whatever its scope, and leaves other users' blocks alone.
package auth_tests

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	auditModels "soli/formations/src/audit/models"
	testTools "soli/formations/tests/testTools"
)

func TestDeleteMyAccount_ErasesSessionBlocks(t *testing.T) {
	db := setupDeleteMyAccountDB(t)
	require.NoError(t, testTools.CreateSessionBlocksTable(db))
	userID, otherID := newUserID(), newUserID()
	class, org := uuid.New(), uuid.New()
	require.NoError(t, testTools.SeedSessionBlock(db, userID, "group", &class, "late"))
	require.NoError(t, testTools.SeedSessionBlock(db, userID, "org", &org, "absences"))
	require.NoError(t, testTools.SeedSessionBlock(db, userID, "global", nil, "fraud"))
	require.NoError(t, testTools.SeedSessionBlock(db, otherID, "global", nil, "fraud"))

	casdoorMock, helperMock, _ := happyMocks(userID)
	require.NoError(t, composedUserDeletionService(db, casdoorMock, helperMock).DeleteMyAccount(userID))

	assert.Zero(t, testTools.CountSessionBlocks(db, userID), "every block on the erased account is deleted")
	assert.EqualValues(t, 1, testTools.CountSessionBlocks(db, otherID), "other users' blocks stay")
}

func TestDeleteMyAccount_BlocksPlacedOnOthers_KeptWithAuthorBlanked(t *testing.T) {
	db := setupDeleteMyAccountDB(t)
	require.NoError(t, testTools.CreateSessionBlocksTable(db))
	userID, otherID := newUserID(), newUserID()
	class := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO session_blocks (id, user_id, scope_type, scope_id, reason, blocked_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		uuid.NewString(), otherID, "group", class.String(), "late", userID, time.Now(),
	).Error)
	require.NoError(t, testTools.SeedSessionBlock(db, otherID, "global", nil, "fraud"))

	casdoorMock, helperMock, _ := happyMocks(userID)
	require.NoError(t, composedUserDeletionService(db, casdoorMock, helperMock).DeleteMyAccount(userID))

	assert.EqualValues(t, 2, testTools.CountSessionBlocks(db, otherID), "blocks the erased user placed on others stay")
	var placedByErased int64
	db.Table("session_blocks").Where("blocked_by = ?", userID).Count(&placedByErased)
	assert.Zero(t, placedByErased, "no block keeps the erased user as its author")
	var otherAuthors int64
	db.Table("session_blocks").Where("blocked_by = ?", "test-blocker").Count(&otherAuthors)
	assert.EqualValues(t, 1, otherAuthors, "blocks placed by someone else keep their author")
}

// =============================================================================
// Final round (#580): sticky internet cuts and audit rows naming the user
// =============================================================================

func TestDeleteMyAccount_ErasesNetworkCutsAndBlanksTheirAuthor(t *testing.T) {
	db := setupDeleteMyAccountDB(t)
	require.NoError(t, testTools.CreateSessionBlocksTable(db))
	require.NoError(t, testTools.CreateParticipantNetworkCutsTable(db))
	userID, otherID := newUserID(), newUserID()
	org := uuid.New()
	require.NoError(t, testTools.SeedNetworkCut(db, userID, uuid.New(), &org, "some-manager"))
	require.NoError(t, testTools.SeedNetworkCut(db, otherID, uuid.New(), &org, userID))
	require.NoError(t, testTools.SeedNetworkCut(db, otherID, uuid.New(), &org, "some-manager"))

	casdoorMock, helperMock, _ := happyMocks(userID)
	require.NoError(t, composedUserDeletionService(db, casdoorMock, helperMock).DeleteMyAccount(userID))

	assert.Zero(t, testTools.CountNetworkCuts(db, userID), "every cut on the erased account is deleted")
	assert.EqualValues(t, 2, testTools.CountNetworkCuts(db, otherID), "cuts the erased user placed on others stay")
	var byErased int64
	db.Table("participant_network_cuts").Where("cut_by = ?", userID).Count(&byErased)
	assert.Zero(t, byErased, "no cut keeps the erased user as its author")
}

func seedAuditTargeting(t *testing.T, db *gorm.DB, targetID string) uuid.UUID {
	t.Helper()
	target := uuid.MustParse(targetID)
	id := uuid.New()
	require.NoError(t, db.Create(&auditModels.AuditLog{
		ID: id, EventType: auditModels.AuditEventUserSessionBlockSet, Severity: auditModels.AuditSeverityWarning,
		TargetID: &target, TargetType: "user", TargetName: "Name of the target",
		Action: "user.session_block.set", Status: "requested",
		CreatedAt: time.Now(), ExpiresAt: time.Now().Add(365 * 24 * time.Hour),
	}).Error)
	return id
}

func TestDeleteMyAccount_AnonymisesAuditRowsTargetingTheUser(t *testing.T) {
	db := setupDeleteMyAccountDB(t)
	require.NoError(t, testTools.CreateSessionBlocksTable(db))
	require.NoError(t, testTools.CreateParticipantNetworkCutsTable(db))
	userID, otherID := newUserID(), newUserID()
	mine := seedAuditTargeting(t, db, userID)
	theirs := seedAuditTargeting(t, db, otherID)

	casdoorMock, helperMock, _ := happyMocks(userID)
	require.NoError(t, composedUserDeletionService(db, casdoorMock, helperMock).DeleteMyAccount(userID))

	var row auditModels.AuditLog
	require.NoError(t, db.First(&row, "id = ?", mine).Error, "the event itself is kept")
	assert.Nil(t, row.TargetID, "an audit row targeting the erased user loses its target_id")
	assert.Empty(t, row.TargetName, "and its target_name")

	var other auditModels.AuditLog
	require.NoError(t, db.First(&other, "id = ?", theirs).Error)
	require.NotNil(t, other.TargetID, "rows targeting other users are untouched")
	assert.Equal(t, "Name of the target", other.TargetName)
}
