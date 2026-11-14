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
