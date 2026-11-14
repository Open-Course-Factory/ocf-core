package groups_tests

// Session start blocks (#579): a class block belongs to the membership. When
// the learner leaves the class, its class-scope block goes with it; their
// blocks in other classes, their organisation and the platform stay.

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"soli/formations/src/auth/casdoor"
	authMocks "soli/formations/src/auth/mocks"
	"soli/formations/src/entityManagement/hooks"
	groupHooks "soli/formations/src/groups/hooks"
	groupModels "soli/formations/src/groups/models"
	testTools "soli/formations/tests/testTools"
)

// blockedLearnerInClass seeds a class with an owner, a manager and a learner,
// and gives the learner a block in that class plus blocks of every other kind.
func blockedLearnerInClass(t *testing.T) (db *gorm.DB, groupID, otherClass uuid.UUID) {
	t.Helper()
	orig := casdoor.Enforcer
	casdoor.Enforcer = authMocks.NewMockEnforcer()
	t.Cleanup(func() { casdoor.Enforcer = orig })

	db = newGroupRoleCapDB(t)
	require.NoError(t, testTools.CreateSessionBlocksTable(db))
	groupID = seedGroupRoleCap(t, db, "class-owner", nil, map[string]groupModels.GroupMemberRole{
		"class-owner":   groupModels.GroupMemberRoleOwner,
		"class-manager": groupModels.GroupMemberRoleManager,
		"blk-learner":   groupModels.GroupMemberRoleMember,
	})
	otherClass, org := uuid.New(), uuid.New()
	require.NoError(t, testTools.SeedSessionBlock(db, "blk-learner", "group", &groupID, "late"))
	require.NoError(t, testTools.SeedSessionBlock(db, "blk-learner", "group", &otherClass, "late"))
	require.NoError(t, testTools.SeedSessionBlock(db, "blk-learner", "org", &org, ""))
	require.NoError(t, testTools.SeedSessionBlock(db, "blk-learner", "global", nil, ""))
	return db, groupID, otherClass
}

func assertOnlyThatClassBlockRemoved(t *testing.T, db *gorm.DB, groupID, otherClass uuid.UUID) {
	t.Helper()
	var inClass, inOtherClass int64
	db.Table("session_blocks").Where("user_id = ? AND scope_type = 'group' AND scope_id = ?", "blk-learner", groupID.String()).Count(&inClass)
	db.Table("session_blocks").Where("user_id = ? AND scope_type = 'group' AND scope_id = ?", "blk-learner", otherClass.String()).Count(&inOtherClass)
	assert.Zero(t, inClass, "the block in the class the learner left is deleted")
	assert.EqualValues(t, 1, inOtherClass, "the block in another class stays")
	assert.EqualValues(t, 3, testTools.CountSessionBlocks(db, "blk-learner"), "org and platform blocks stay")
}

func TestGroupMemberDelete_DeletesThatClassSessionBlock(t *testing.T) {
	db, groupID, otherClass := blockedLearnerInClass(t)

	hook := groupHooks.NewGroupMemberCleanupHook(db)
	require.NoError(t, hook.Execute(&hooks.HookContext{
		EntityName: "GroupMember",
		HookType:   hooks.BeforeDelete,
		NewEntity:  &groupModels.GroupMember{GroupID: groupID, UserID: "blk-learner", Role: groupModels.GroupMemberRoleMember},
		UserID:     "class-manager",
		UserRoles:  platformMember,
	}))

	assertOnlyThatClassBlockRemoved(t, db, groupID, otherClass)
}
