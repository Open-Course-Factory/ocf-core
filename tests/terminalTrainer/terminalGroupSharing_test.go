package terminalTrainer_tests

import (
	"testing"

	groupModels "soli/formations/src/groups/models"
	"soli/formations/src/terminalTrainer/models"
	"soli/formations/src/terminalTrainer/repositories"
	"soli/formations/src/terminalTrainer/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShareTerminalWithGroup_Success(t *testing.T) {
	db := setupTestDB(t)

	ownerUserID := "owner-user"
	groupID := uuid.New()

	// Create terminal owned by the user
	userKey, err := createTestUserKey(db, ownerUserID)
	require.NoError(t, err)
	terminal, err := createTestTerminal(db, ownerUserID, "active", userKey.ID)
	require.NoError(t, err)

	// Create the service
	svc := services.NewTerminalTrainerService(db)

	// Share terminal with group
	err = svc.ShareTerminalWithGroup(terminal.SessionID, ownerUserID, groupID, models.AccessLevelRead, nil)
	assert.NoError(t, err)

	// Verify share was created in DB
	var share models.TerminalShare
	err = db.Where("terminal_id = ? AND shared_with_group_id = ?", terminal.ID, groupID).First(&share).Error
	assert.NoError(t, err)
	assert.Equal(t, models.AccessLevelRead, share.AccessLevel)
	assert.True(t, share.IsActive)
	assert.Nil(t, share.SharedWithUserID)
	assert.NotNil(t, share.SharedWithGroupID)
	assert.Equal(t, groupID, *share.SharedWithGroupID)
}

func TestShareTerminalWithGroup_UpdateExisting(t *testing.T) {
	db := setupTestDB(t)

	ownerUserID := "owner-user"
	groupID := uuid.New()

	// Create terminal
	userKey, err := createTestUserKey(db, ownerUserID)
	require.NoError(t, err)
	terminal, err := createTestTerminal(db, ownerUserID, "active", userKey.ID)
	require.NoError(t, err)

	svc := services.NewTerminalTrainerService(db)

	// Share with read access first
	err = svc.ShareTerminalWithGroup(terminal.SessionID, ownerUserID, groupID, models.AccessLevelRead, nil)
	require.NoError(t, err)

	// Share again with write access — should update, not duplicate
	err = svc.ShareTerminalWithGroup(terminal.SessionID, ownerUserID, groupID, models.AccessLevelWrite, nil)
	assert.NoError(t, err)

	// Verify only one share exists and it has write access
	var shares []models.TerminalShare
	err = db.Where("terminal_id = ? AND shared_with_group_id = ?", terminal.ID, groupID).Find(&shares).Error
	assert.NoError(t, err)
	assert.Len(t, shares, 1)
	assert.Equal(t, models.AccessLevelWrite, shares[0].AccessLevel)
}

func TestShareTerminalWithGroup_NotOwner(t *testing.T) {
	db := setupTestDB(t)

	ownerUserID := "owner-user"
	otherUserID := "other-user"
	groupID := uuid.New()

	// Create terminal owned by ownerUserID
	userKey, err := createTestUserKey(db, ownerUserID)
	require.NoError(t, err)
	terminal, err := createTestTerminal(db, ownerUserID, "active", userKey.ID)
	require.NoError(t, err)

	svc := services.NewTerminalTrainerService(db)

	// Try to share from a non-owner — should fail
	err = svc.ShareTerminalWithGroup(terminal.SessionID, otherUserID, groupID, models.AccessLevelRead, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "owner")
}

func TestShareTerminalWithGroup_InvalidAccessLevel(t *testing.T) {
	db := setupTestDB(t)

	ownerUserID := "owner-user"
	groupID := uuid.New()

	userKey, err := createTestUserKey(db, ownerUserID)
	require.NoError(t, err)
	terminal, err := createTestTerminal(db, ownerUserID, "active", userKey.ID)
	require.NoError(t, err)

	svc := services.NewTerminalTrainerService(db)

	err = svc.ShareTerminalWithGroup(terminal.SessionID, ownerUserID, groupID, "invalid", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid access level")
}

func TestShareTerminalWithGroup_TerminalNotFound(t *testing.T) {
	db := setupTestDB(t)

	svc := services.NewTerminalTrainerService(db)

	err := svc.ShareTerminalWithGroup("nonexistent-session", "some-user", uuid.New(), models.AccessLevelRead, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "terminal not found")
}

func TestHasTerminalAccess_GroupMember(t *testing.T) {
	db := setupTestDB(t)
	repo := repositories.NewTerminalRepository(db)

	ownerUserID := "owner-user"
	memberUserID := "member-user"
	groupID := uuid.New()

	// Create terminal
	userKey, err := createTestUserKey(db, ownerUserID)
	require.NoError(t, err)
	terminal, err := createTestTerminal(db, ownerUserID, "active", userKey.ID)
	require.NoError(t, err)

	// Create a group share
	_, err = createTestGroupShare(db, terminal.ID, ownerUserID, groupID, models.AccessLevelRead)
	require.NoError(t, err)

	// Create group membership for the user
	createTestGroupMember(t, db, groupID, memberUserID, groupModels.GroupMemberRoleMember)

	// Group member should have access
	hasAccess, err := repo.HasTerminalAccess(terminal.ID.String(), memberUserID, models.AccessLevelRead)
	assert.NoError(t, err)
	assert.True(t, hasAccess)
}

func TestHasTerminalAccess_GroupMember_InsufficientLevel(t *testing.T) {
	db := setupTestDB(t)
	repo := repositories.NewTerminalRepository(db)

	ownerUserID := "owner-user"
	memberUserID := "member-user"
	groupID := uuid.New()

	// Create terminal
	userKey, err := createTestUserKey(db, ownerUserID)
	require.NoError(t, err)
	terminal, err := createTestTerminal(db, ownerUserID, "active", userKey.ID)
	require.NoError(t, err)

	// Create a group share with read-only access
	_, err = createTestGroupShare(db, terminal.ID, ownerUserID, groupID, models.AccessLevelRead)
	require.NoError(t, err)

	// Create group membership
	createTestGroupMember(t, db, groupID, memberUserID, groupModels.GroupMemberRoleMember)

	// Group member should NOT have write access
	hasAccess, err := repo.HasTerminalAccess(terminal.ID.String(), memberUserID, models.AccessLevelWrite)
	assert.NoError(t, err)
	assert.False(t, hasAccess)
}

func TestHasTerminalAccess_NonGroupMember(t *testing.T) {
	db := setupTestDB(t)
	repo := repositories.NewTerminalRepository(db)

	ownerUserID := "owner-user"
	nonMemberUserID := "non-member-user"
	groupID := uuid.New()

	// Create terminal
	userKey, err := createTestUserKey(db, ownerUserID)
	require.NoError(t, err)
	terminal, err := createTestTerminal(db, ownerUserID, "active", userKey.ID)
	require.NoError(t, err)

	// Create a group share
	_, err = createTestGroupShare(db, terminal.ID, ownerUserID, groupID, models.AccessLevelRead)
	require.NoError(t, err)

	// Non-member should NOT have access (no membership created)
	hasAccess, err := repo.HasTerminalAccess(terminal.ID.String(), nonMemberUserID, models.AccessLevelRead)
	assert.NoError(t, err)
	assert.False(t, hasAccess)
}

func TestHasTerminalAccess_InactiveGroupMember(t *testing.T) {
	db := setupTestDB(t)
	repo := repositories.NewTerminalRepository(db)

	ownerUserID := "owner-user"
	memberUserID := "inactive-member"
	groupID := uuid.New()

	// Create terminal
	userKey, err := createTestUserKey(db, ownerUserID)
	require.NoError(t, err)
	terminal, err := createTestTerminal(db, ownerUserID, "active", userKey.ID)
	require.NoError(t, err)

	// Create a group share
	_, err = createTestGroupShare(db, terminal.ID, ownerUserID, groupID, models.AccessLevelRead)
	require.NoError(t, err)

	// Create an inactive group member
	member := createTestGroupMember(t, db, groupID, memberUserID, groupModels.GroupMemberRoleMember)
	db.Model(member).Update("is_active", false)

	// Inactive member should NOT have access
	hasAccess, err := repo.HasTerminalAccess(terminal.ID.String(), memberUserID, models.AccessLevelRead)
	assert.NoError(t, err)
	assert.False(t, hasAccess)
}

func TestGetSharedTerminals_IncludesGroupShares(t *testing.T) {
	db := setupTestDB(t)
	repo := repositories.NewTerminalRepository(db)

	ownerUserID := "owner-user"
	memberUserID := "member-user"
	groupID := uuid.New()

	// Create terminal
	userKey, err := createTestUserKey(db, ownerUserID)
	require.NoError(t, err)
	terminal, err := createTestTerminal(db, ownerUserID, "active", userKey.ID)
	require.NoError(t, err)

	// Create a group share
	_, err = createTestGroupShare(db, terminal.ID, ownerUserID, groupID, models.AccessLevelRead)
	require.NoError(t, err)

	// Create group membership
	createTestGroupMember(t, db, groupID, memberUserID, groupModels.GroupMemberRoleMember)

	// Group-shared terminals should appear in the shared list
	terminals, err := repo.GetSharedTerminalsForUserWithHidden(memberUserID, false)
	assert.NoError(t, err)
	require.NotNil(t, terminals)
	assert.Len(t, *terminals, 1)
	assert.Equal(t, terminal.ID, (*terminals)[0].ID)
}

func TestGetSharedTerminals_NonMemberGetsNothing(t *testing.T) {
	db := setupTestDB(t)
	repo := repositories.NewTerminalRepository(db)

	ownerUserID := "owner-user"
	nonMemberUserID := "non-member"
	groupID := uuid.New()

	// Create terminal
	userKey, err := createTestUserKey(db, ownerUserID)
	require.NoError(t, err)
	terminal, err := createTestTerminal(db, ownerUserID, "active", userKey.ID)
	require.NoError(t, err)

	// Create a group share
	_, err = createTestGroupShare(db, terminal.ID, ownerUserID, groupID, models.AccessLevelRead)
	require.NoError(t, err)

	// Non-member should get no terminals
	terminals, err := repo.GetSharedTerminalsForUserWithHidden(nonMemberUserID, false)
	assert.NoError(t, err)
	require.NotNil(t, terminals)
	assert.Len(t, *terminals, 0)
}

func TestGetSharedTerminals_NoDuplicates(t *testing.T) {
	db := setupTestDB(t)
	repo := repositories.NewTerminalRepository(db)

	ownerUserID := "owner-user"
	memberUserID := "member-user"
	groupID := uuid.New()

	// Create terminal
	userKey, err := createTestUserKey(db, ownerUserID)
	require.NoError(t, err)
	terminal, err := createTestTerminal(db, ownerUserID, "active", userKey.ID)
	require.NoError(t, err)

	// Create both a direct user share and a group share for the same terminal
	_, err = createTestTerminalShare(db, terminal.ID, ownerUserID, memberUserID)
	require.NoError(t, err)
	_, err = createTestGroupShare(db, terminal.ID, ownerUserID, groupID, models.AccessLevelRead)
	require.NoError(t, err)
	createTestGroupMember(t, db, groupID, memberUserID, groupModels.GroupMemberRoleMember)

	// Should not get duplicates
	terminals, err := repo.GetSharedTerminalsForUserWithHidden(memberUserID, false)
	assert.NoError(t, err)
	require.NotNil(t, terminals)
	assert.Len(t, *terminals, 1)
}
