package groups_tests

// Tests for #410 (audit-P, MR P): the GroupMemberValidationHook (BeforeCreate on
// GroupMember) must cap the role a granter can assign to a NEW member at the granter's own
// rank — mirroring the org-member create cap already merged in
// OrganizationMemberValidationHook (MR !293). Today the group hook checks that the granter
// can manage the group (step 5) but NEVER caps the assigned role, so a manager can create
// a GroupMember with Role="owner" (the client supplies Role; the binding tag is inert on
// the generic create path).
//
// Rule to enforce (matching the org hook):
//
//	granter role >= assigned role   (access.IsRoleAtLeast, owner100 > manager50 > member10)
//
// Platform administrators (ctx.IsAdmin()) bypass the cap.
//
// These tests drive the hook directly via hook.Execute(ctx) and assert the returned error,
// which is the reject signal the generic create path surfaces to the caller. The granter is
// seeded as a real direct group MANAGER so GetUserGroupRole resolves to manager. Shared
// helpers newGroupRoleCapDB / seedGroupRoleCap live in groupServiceRoleCap_test.go.

import (
	"testing"

	"soli/formations/src/entityManagement/hooks"
	groupHooks "soli/formations/src/groups/hooks"
	groupModels "soli/formations/src/groups/models"

	"github.com/stretchr/testify/require"
)

// runGroupCreateHookRoleCapCase seeds a group owned by a distinct account with granterID as
// a direct group manager, then runs the member-validation hook for a brand-new target being
// assigned assignedRole. platformRoles is the Layer-1 platform role set. Returns the hook
// error.
func runGroupCreateHookRoleCapCase(
	t *testing.T,
	platformRoles []string,
	assignedRole groupModels.GroupMemberRole,
) error {
	t.Helper()

	const (
		ownerID   = "group-owner-account"
		granterID = "group-manager-granter"
		targetID  = "brand-new-target"
	)

	db := newGroupRoleCapDB(t)
	groupID := seedGroupRoleCap(t, db, ownerID, nil, map[string]groupModels.GroupMemberRole{
		ownerID:   groupModels.GroupMemberRoleOwner,
		granterID: groupModels.GroupMemberRoleManager,
	})

	hook := groupHooks.NewGroupMemberValidationHook(db)
	ctx := &hooks.HookContext{
		EntityName: "GroupMember",
		HookType:   hooks.BeforeCreate,
		NewEntity: &groupModels.GroupMember{
			GroupID: groupID,
			UserID:  targetID,
			Role:    assignedRole,
		},
		UserID:    granterID,
		UserRoles: platformRoles,
	}

	return hook.Execute(ctx)
}

// platformMember is the Layer-1 role every real user carries.
var platformMember = []string{"member"}

// TestGroupMemberCreateHook_ManagerAssignsOwner_Rejected is the core cap guard for the
// group create path: a manager granter must NOT mint an owner. Expected RED today (the
// hook has no cap → it returns nil and the owner assignment is accepted).
func TestGroupMemberCreateHook_ManagerAssignsOwner_Rejected(t *testing.T) {
	err := runGroupCreateHookRoleCapCase(t, platformMember, groupModels.GroupMemberRoleOwner)
	require.Error(t, err,
		"a manager must not create a group member with the owner role (owner100 > manager50); "+
			"the hook must reject, but it returned nil")
}

// TestGroupMemberCreateHook_ManagerAssignsMember_Allowed is a green guard against
// over-restriction: a manager may add a plain member (manager50 >= member10).
func TestGroupMemberCreateHook_ManagerAssignsMember_Allowed(t *testing.T) {
	err := runGroupCreateHookRoleCapCase(t, platformMember, groupModels.GroupMemberRoleMember)
	require.NoError(t, err,
		"a manager assigning member must be allowed (manager50 >= member10); "+
			"the cap must not over-restrict roles at or below the granter's own")
}
