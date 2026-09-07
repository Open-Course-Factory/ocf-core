package groups_tests

// Tests for #353: DELETE /group-members/:id had no Layer-2 authorization. The
// only BeforeDelete hook (GroupMemberCleanupHook) refused to remove an owner
// and revoked permissions, but never asked whether the requester may manage
// the group — while the POST path gates on groupService.CanUserManageGroup.
// The same predicate must decide both sides, with the platform-admin bypass
// the create hook already grants.
//
// These tests drive the hook directly via hook.Execute(ctx); the returned error
// is the reject signal the generic delete path surfaces to the caller.

import (
	"testing"

	"soli/formations/src/auth/casdoor"
	authMocks "soli/formations/src/auth/mocks"
	"soli/formations/src/entityManagement/hooks"
	groupHooks "soli/formations/src/groups/hooks"
	groupModels "soli/formations/src/groups/models"

	"github.com/stretchr/testify/require"
)

// runGroupMemberDeleteHookCase seeds a group with an owner, a manager and two
// plain members, then runs the cleanup hook as requesterID removing the
// plain member "removed-target". Returns the hook error.
func runGroupMemberDeleteHookCase(t *testing.T, requesterID string, platformRoles []string) error {
	t.Helper()

	orig := casdoor.Enforcer
	casdoor.Enforcer = authMocks.NewMockEnforcer()
	t.Cleanup(func() { casdoor.Enforcer = orig })

	db := newGroupRoleCapDB(t)
	groupID := seedGroupRoleCap(t, db, "group-owner-account", nil, map[string]groupModels.GroupMemberRole{
		"group-owner-account": groupModels.GroupMemberRoleOwner,
		"group-manager":       groupModels.GroupMemberRoleManager,
		"plain-classmate":     groupModels.GroupMemberRoleMember,
		"removed-target":      groupModels.GroupMemberRoleMember,
	})

	hook := groupHooks.NewGroupMemberCleanupHook(db)
	ctx := &hooks.HookContext{
		EntityName: "GroupMember",
		HookType:   hooks.BeforeDelete,
		NewEntity: &groupModels.GroupMember{
			GroupID: groupID,
			UserID:  "removed-target",
			Role:    groupModels.GroupMemberRoleMember,
		},
		UserID:    requesterID,
		UserRoles: platformRoles,
	}
	return hook.Execute(ctx)
}

func TestGroupMemberDeleteHook_PlainMemberRemovingAClassmate_Rejected(t *testing.T) {
	err := runGroupMemberDeleteHookCase(t, "plain-classmate", platformMember)
	require.Error(t, err, "a plain member must not remove another member; the delete hook must refuse")
}

func TestGroupMemberDeleteHook_StrangerRemovingAMember_Rejected(t *testing.T) {
	err := runGroupMemberDeleteHookCase(t, "no-tie-to-this-group", platformMember)
	require.Error(t, err, "a user with no tie to the group must not remove its members")
}

func TestGroupMemberDeleteHook_ManagerRemovingAMember_Allowed(t *testing.T) {
	err := runGroupMemberDeleteHookCase(t, "group-manager", platformMember)
	require.NoError(t, err, "a group manager may remove a plain member")
}

func TestGroupMemberDeleteHook_PlatformAdminWithoutMembership_Allowed(t *testing.T) {
	err := runGroupMemberDeleteHookCase(t, "platform-ops", []string{"administrator"})
	require.NoError(t, err, "a platform administrator bypasses the group-role check")
}
