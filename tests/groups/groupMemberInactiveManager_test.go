package groups_tests

// A deactivated group membership must grant nothing, like a deactivated
// organization membership: the organization cleanup stands a class roster down
// with is_active=false, and from then on a former class manager must no longer
// add, remove or re-rank members through the GroupMember hooks.
//
// Each case runs the real hook as a non-creator class manager, once with the
// row active (control) and once with it deactivated the way production does it
// (an UPDATE of the column, which the GORM bool-default pitfall cannot swallow).

import (
	"testing"

	"soli/formations/src/auth/casdoor"
	authMocks "soli/formations/src/auth/mocks"
	"soli/formations/src/entityManagement/hooks"
	groupHooks "soli/formations/src/groups/hooks"
	groupModels "soli/formations/src/groups/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const (
	inactiveCaseOwner   = "class-creator"
	inactiveCaseManager = "class-manager"
	inactiveCaseMember  = "class-member"
)

// seedManagedClass seeds a class whose creator is distinct from its manager,
// and deactivates the manager's row when managerActive is false.
func seedManagedClass(t *testing.T, managerActive bool) (*gorm.DB, uuid.UUID) {
	t.Helper()

	orig := casdoor.Enforcer
	casdoor.Enforcer = authMocks.NewMockEnforcer()
	t.Cleanup(func() { casdoor.Enforcer = orig })

	db := newGroupRoleCapDB(t)
	groupID := seedGroupRoleCap(t, db, inactiveCaseOwner, nil, map[string]groupModels.GroupMemberRole{
		inactiveCaseOwner:   groupModels.GroupMemberRoleOwner,
		inactiveCaseManager: groupModels.GroupMemberRoleManager,
		inactiveCaseMember:  groupModels.GroupMemberRoleMember,
	})
	if !managerActive {
		require.NoError(t, db.Model(&groupModels.GroupMember{}).
			Where("group_id = ? AND user_id = ?", groupID, inactiveCaseManager).
			Update("is_active", false).Error)
	}
	return db, groupID
}

func runAddMemberAsManager(t *testing.T, managerActive bool) error {
	db, groupID := seedManagedClass(t, managerActive)
	return groupHooks.NewGroupMemberValidationHook(db).Execute(&hooks.HookContext{
		EntityName: "GroupMember",
		HookType:   hooks.BeforeCreate,
		NewEntity: &groupModels.GroupMember{
			GroupID: groupID,
			UserID:  "newcomer",
			Role:    groupModels.GroupMemberRoleMember,
		},
		UserID:    inactiveCaseManager,
		UserRoles: platformMember,
	})
}

func runRemoveMemberAsManager(t *testing.T, managerActive bool) error {
	db, groupID := seedManagedClass(t, managerActive)
	return groupHooks.NewGroupMemberCleanupHook(db).Execute(&hooks.HookContext{
		EntityName: "GroupMember",
		HookType:   hooks.BeforeDelete,
		NewEntity: &groupModels.GroupMember{
			GroupID: groupID,
			UserID:  inactiveCaseMember,
			Role:    groupModels.GroupMemberRoleMember,
		},
		UserID:    inactiveCaseManager,
		UserRoles: platformMember,
	})
}

func runChangeRoleAsManager(t *testing.T, managerActive bool) error {
	db, groupID := seedManagedClass(t, managerActive)
	return groupHooks.NewGroupMemberRoleChangeHook(db).Execute(&hooks.HookContext{
		EntityName: "GroupMember",
		HookType:   hooks.BeforeUpdate,
		OldEntity: &groupModels.GroupMember{
			GroupID: groupID,
			UserID:  inactiveCaseMember,
			Role:    groupModels.GroupMemberRoleMember,
		},
		NewEntity: map[string]any{"role": groupModels.GroupMemberRoleManager},
		UserID:    inactiveCaseManager,
		UserRoles: platformMember,
	})
}

func TestGroupMemberHooks_ActiveManagerAddsMember_Allowed(t *testing.T) {
	require.NoError(t, runAddMemberAsManager(t, true))
}

func TestGroupMemberHooks_DeactivatedManagerAddsMember_Rejected(t *testing.T) {
	require.Error(t, runAddMemberAsManager(t, false),
		"a deactivated class manager must not add members to the class")
}

func TestGroupMemberHooks_ActiveManagerRemovesMember_Allowed(t *testing.T) {
	require.NoError(t, runRemoveMemberAsManager(t, true))
}

func TestGroupMemberHooks_DeactivatedManagerRemovesMember_Rejected(t *testing.T) {
	require.Error(t, runRemoveMemberAsManager(t, false),
		"a deactivated class manager must not remove members from the class")
}

func TestGroupMemberHooks_ActiveManagerChangesRole_Allowed(t *testing.T) {
	require.NoError(t, runChangeRoleAsManager(t, true))
}

func TestGroupMemberHooks_DeactivatedManagerChangesRole_Rejected(t *testing.T) {
	require.Error(t, runChangeRoleAsManager(t, false),
		"a deactivated class manager must not change roles in the class")
}
