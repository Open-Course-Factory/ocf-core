package organizations_tests

// Tests pinning the Casbin permission-sync contract for the OrganizationMember
// lifecycle (GitLab ocf-core#426).
//
// The generic route POST/PATCH/DELETE /api/v1/organization-members must keep the
// Casbin grouping policies in sync with a member's role, exactly the way the group
// side does via GroupMemberPermissionHook. The observable outcome lives in the
// enforcer's grouping policies:
//
//   - base membership   -> grouping (userID, "organization:<orgID>")
//   - manager privilege -> grouping (userID, "organization_manager:<orgID>")
//
// These are the g-rows the Layer-1 gateway reads: without the manager row a member
// promoted to manager via the generic PATCH still gets 403 on PATCH /organizations/:id.
//
// The tests drive the REAL org hooks through the global hook registry (populated by
// InitOrganizationHooks, the same registration used in production) and assert on the
// grouping policies recorded by a MockEnforcer installed as the global
// casdoor.Enforcer — the enforcer is the boundary these hooks write to, so recorded
// AddGroupingPolicy / RemoveGroupingPolicy calls with exact (user, role) arguments are
// the user-observable state.
//
// RED today: no AfterCreate / AfterUpdate permission-sync hook is registered for
// "OrganizationMember", so ExecuteHooks finds no hook and no grouping is granted or
// revoked. The create and update tests fail for that reason (missing grant/revoke),
// not a compile error.
//
// The two delete tests are GREEN pins: OrganizationMemberDeletionHook already revokes
// BOTH the base and manager groupings, so they lock in the current (correct) behavior
// and guard the fix against regressing it.
//
// Shared helpers newOrgRoleCapDB / seedOrgWithMembers (organizationMemberRoleCap_test.go)
// and the platformMember var are reused, not redefined.

import (
	"testing"

	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/mocks"
	"soli/formations/src/entityManagement/hooks"
	organizationHooks "soli/formations/src/organizations/hooks"
	"soli/formations/src/organizations/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// installMockEnforcer swaps the global casdoor.Enforcer for a recording MockEnforcer
// and returns it so a test can inspect the grouping policies the hooks write. The
// original enforcer is restored on cleanup.
func installMockEnforcer(t *testing.T) *mocks.MockEnforcer {
	t.Helper()
	orig := casdoor.Enforcer
	m := mocks.NewMockEnforcer()
	casdoor.Enforcer = m
	t.Cleanup(func() { casdoor.Enforcer = orig })
	return m
}

// registerOrgHooks populates the global hook registry with the real organization hooks
// (exactly as production does) for the duration of the test, then clears it again so no
// hook leaks into sibling tests. It also clears any globalDisable a prior test may have
// left set, so ExecuteHooks actually runs the hooks.
func registerOrgHooks(t *testing.T, db *gorm.DB) {
	t.Helper()
	hooks.GlobalHookRegistry.ClearAllHooks()
	hooks.GlobalHookRegistry.DisableAllHooks(false)
	organizationHooks.InitOrganizationHooks(db)
	t.Cleanup(func() { hooks.GlobalHookRegistry.ClearAllHooks() })
}

func orgGrouping(orgID uuid.UUID) string {
	return "organization:" + orgID.String()
}

func orgManagerGrouping(orgID uuid.UUID) string {
	return "organization_manager:" + orgID.String()
}

// hasGroupingCall reports whether the recorded grouping calls contain an entry whose
// first two params are exactly (userID, roleID). MockEnforcer records each call as the
// variadic params slice, i.e. [userID, roleID].
func hasGroupingCall(calls [][]any, userID, roleID string) bool {
	for _, c := range calls {
		if len(c) < 2 {
			continue
		}
		u, okU := c[0].(string)
		r, okR := c[1].(string)
		if okU && okR && u == userID && r == roleID {
			return true
		}
	}
	return false
}

// TestOrgMemberCreate_MemberRole_GrantsBaseGrouping — creating an OrganizationMember
// with role "member" must grant the base organization grouping so Casbin recognises the
// user as an org member. RED today: no AfterCreate permission hook is registered, so the
// grouping is never added.
func TestOrgMemberCreate_MemberRole_GrantsBaseGrouping(t *testing.T) {
	const ownerID, targetID = "org-owner-account", "new-plain-member"

	enforcer := installMockEnforcer(t)
	db := newOrgRoleCapDB(t)
	orgID := seedOrgWithMembers(t, db, ownerID, map[string]models.OrganizationMemberRole{
		ownerID: models.OrgRoleOwner,
	})
	registerOrgHooks(t, db)

	ctx := &hooks.HookContext{
		EntityName: "OrganizationMember",
		HookType:   hooks.AfterCreate,
		NewEntity: &models.OrganizationMember{
			OrganizationID: orgID,
			UserID:         targetID,
			Role:           models.OrgRoleMember,
		},
		UserID:    ownerID,
		UserRoles: platformMember,
	}

	require.NoError(t, hooks.GlobalHookRegistry.ExecuteHooks(ctx))
	require.True(t,
		hasGroupingCall(enforcer.AddGroupingPolicyCalls, targetID, orgGrouping(orgID)),
		"creating an org member (role=member) must grant grouping (%s, %s); no permission-sync AfterCreate hook exists, so nothing was granted",
		targetID, orgGrouping(orgID))
}

// TestOrgMemberCreate_ManagerRole_GrantsManagerGrouping — creating an OrganizationMember
// with role "manager" must additionally grant the manager grouping (organization_manager:<id>),
// the g-row the Layer-1 gateway requires for PATCH /organizations/:id. RED today.
func TestOrgMemberCreate_ManagerRole_GrantsManagerGrouping(t *testing.T) {
	const ownerID, targetID = "org-owner-account", "new-manager"

	enforcer := installMockEnforcer(t)
	db := newOrgRoleCapDB(t)
	orgID := seedOrgWithMembers(t, db, ownerID, map[string]models.OrganizationMemberRole{
		ownerID: models.OrgRoleOwner,
	})
	registerOrgHooks(t, db)

	ctx := &hooks.HookContext{
		EntityName: "OrganizationMember",
		HookType:   hooks.AfterCreate,
		NewEntity: &models.OrganizationMember{
			OrganizationID: orgID,
			UserID:         targetID,
			Role:           models.OrgRoleManager,
		},
		UserID:    ownerID,
		UserRoles: platformMember,
	}

	require.NoError(t, hooks.GlobalHookRegistry.ExecuteHooks(ctx))
	require.True(t,
		hasGroupingCall(enforcer.AddGroupingPolicyCalls, targetID, orgManagerGrouping(orgID)),
		"creating an org member with role=manager must grant the manager grouping (%s, %s); "+
			"without it the manager gets 403 on PATCH /organizations/:id",
		targetID, orgManagerGrouping(orgID))
}

// TestOrgMemberUpdate_PromoteToManager_GrantsManagerGrouping — updating a member's role
// from member to manager must grant the manager grouping. On the generic PATCH path the
// AfterUpdate hook sees OldEntity/NewEntity as the pre- and post-patch member rows. RED
// today: no AfterUpdate hook, so promotion never mints the manager g-row.
func TestOrgMemberUpdate_PromoteToManager_GrantsManagerGrouping(t *testing.T) {
	const ownerID, targetID = "org-owner-account", "promoted-member"

	enforcer := installMockEnforcer(t)
	db := newOrgRoleCapDB(t)
	orgID := seedOrgWithMembers(t, db, ownerID, map[string]models.OrganizationMemberRole{
		ownerID:  models.OrgRoleOwner,
		targetID: models.OrgRoleMember,
	})
	registerOrgHooks(t, db)

	ctx := &hooks.HookContext{
		EntityName: "OrganizationMember",
		HookType:   hooks.AfterUpdate,
		OldEntity:  &models.OrganizationMember{OrganizationID: orgID, UserID: targetID, Role: models.OrgRoleMember},
		NewEntity:  &models.OrganizationMember{OrganizationID: orgID, UserID: targetID, Role: models.OrgRoleManager},
		UserID:     ownerID,
		UserRoles:  platformMember,
	}

	require.NoError(t, hooks.GlobalHookRegistry.ExecuteHooks(ctx))
	require.True(t,
		hasGroupingCall(enforcer.AddGroupingPolicyCalls, targetID, orgManagerGrouping(orgID)),
		"promoting a member to manager must grant the manager grouping (%s, %s); no AfterUpdate permission-sync hook exists",
		targetID, orgManagerGrouping(orgID))
}

// TestOrgMemberUpdate_DemoteToMember_RevokesManagerGrouping — updating a member's role
// from manager to member must revoke the manager grouping. RED today: no AfterUpdate
// hook, so a demoted manager keeps organization_manager:<id> and stays over-privileged.
func TestOrgMemberUpdate_DemoteToMember_RevokesManagerGrouping(t *testing.T) {
	const ownerID, targetID = "org-owner-account", "demoted-manager"

	enforcer := installMockEnforcer(t)
	db := newOrgRoleCapDB(t)
	orgID := seedOrgWithMembers(t, db, ownerID, map[string]models.OrganizationMemberRole{
		ownerID:  models.OrgRoleOwner,
		targetID: models.OrgRoleManager,
	})
	registerOrgHooks(t, db)

	ctx := &hooks.HookContext{
		EntityName: "OrganizationMember",
		HookType:   hooks.AfterUpdate,
		OldEntity:  &models.OrganizationMember{OrganizationID: orgID, UserID: targetID, Role: models.OrgRoleManager},
		NewEntity:  &models.OrganizationMember{OrganizationID: orgID, UserID: targetID, Role: models.OrgRoleMember},
		UserID:     ownerID,
		UserRoles:  platformMember,
	}

	require.NoError(t, hooks.GlobalHookRegistry.ExecuteHooks(ctx))
	require.True(t,
		hasGroupingCall(enforcer.RemoveGroupingPolicyCalls, targetID, orgManagerGrouping(orgID)),
		"demoting a manager to member must revoke the manager grouping (%s, %s); no AfterUpdate permission-sync hook exists",
		targetID, orgManagerGrouping(orgID))
}

// TestOrgMemberUpdate_DemoteToMember_KeepsBaseGrouping — a demotion must NOT revoke the
// base organization grouping: the user is still a member. Guard against an over-broad
// fix that revokes everything on demotion. Green today (nothing happens) and must stay
// green after the fix.
func TestOrgMemberUpdate_DemoteToMember_KeepsBaseGrouping(t *testing.T) {
	const ownerID, targetID = "org-owner-account", "demoted-manager"

	enforcer := installMockEnforcer(t)
	db := newOrgRoleCapDB(t)
	orgID := seedOrgWithMembers(t, db, ownerID, map[string]models.OrganizationMemberRole{
		ownerID:  models.OrgRoleOwner,
		targetID: models.OrgRoleManager,
	})
	registerOrgHooks(t, db)

	ctx := &hooks.HookContext{
		EntityName: "OrganizationMember",
		HookType:   hooks.AfterUpdate,
		OldEntity:  &models.OrganizationMember{OrganizationID: orgID, UserID: targetID, Role: models.OrgRoleManager},
		NewEntity:  &models.OrganizationMember{OrganizationID: orgID, UserID: targetID, Role: models.OrgRoleMember},
		UserID:     ownerID,
		UserRoles:  platformMember,
	}

	require.NoError(t, hooks.GlobalHookRegistry.ExecuteHooks(ctx))
	require.False(t,
		hasGroupingCall(enforcer.RemoveGroupingPolicyCalls, targetID, orgGrouping(orgID)),
		"demoting a manager must not revoke the base organization grouping (%s, %s); the user remains a member",
		targetID, orgGrouping(orgID))
}

// TestOrgMemberDelete_Manager_RevokesBothGroupings — GREEN pin. Deleting a manager
// member must revoke BOTH the base and the manager grouping. OrganizationMemberDeletionHook
// already does this; this test locks it in so the #426 fix does not regress it.
func TestOrgMemberDelete_Manager_RevokesBothGroupings(t *testing.T) {
	const ownerID, targetID = "org-owner-account", "manager-to-remove"

	enforcer := installMockEnforcer(t)
	db := newOrgRoleCapDB(t)
	orgID := seedOrgWithMembers(t, db, ownerID, map[string]models.OrganizationMemberRole{
		ownerID:  models.OrgRoleOwner,
		targetID: models.OrgRoleManager,
	})
	registerOrgHooks(t, db)

	ctx := &hooks.HookContext{
		EntityName: "OrganizationMember",
		HookType:   hooks.BeforeDelete,
		NewEntity: &models.OrganizationMember{
			OrganizationID: orgID,
			UserID:         targetID,
			Role:           models.OrgRoleManager,
		},
		UserID:    ownerID,
		UserRoles: platformMember,
	}

	require.NoError(t, hooks.GlobalHookRegistry.ExecuteHooks(ctx))
	require.True(t,
		hasGroupingCall(enforcer.RemoveGroupingPolicyCalls, targetID, orgGrouping(orgID)),
		"deleting a manager must revoke the base grouping (%s, %s)", targetID, orgGrouping(orgID))
	require.True(t,
		hasGroupingCall(enforcer.RemoveGroupingPolicyCalls, targetID, orgManagerGrouping(orgID)),
		"deleting a manager must revoke the manager grouping (%s, %s)", targetID, orgManagerGrouping(orgID))
}

// TestOrgMemberDelete_Member_RevokesBaseGrouping — GREEN pin. Deleting a plain member
// must revoke the base organization grouping.
func TestOrgMemberDelete_Member_RevokesBaseGrouping(t *testing.T) {
	const ownerID, targetID = "org-owner-account", "member-to-remove"

	enforcer := installMockEnforcer(t)
	db := newOrgRoleCapDB(t)
	orgID := seedOrgWithMembers(t, db, ownerID, map[string]models.OrganizationMemberRole{
		ownerID:  models.OrgRoleOwner,
		targetID: models.OrgRoleMember,
	})
	registerOrgHooks(t, db)

	ctx := &hooks.HookContext{
		EntityName: "OrganizationMember",
		HookType:   hooks.BeforeDelete,
		NewEntity: &models.OrganizationMember{
			OrganizationID: orgID,
			UserID:         targetID,
			Role:           models.OrgRoleMember,
		},
		UserID:    ownerID,
		UserRoles: platformMember,
	}

	require.NoError(t, hooks.GlobalHookRegistry.ExecuteHooks(ctx))
	require.True(t,
		hasGroupingCall(enforcer.RemoveGroupingPolicyCalls, targetID, orgGrouping(orgID)),
		"deleting a plain member must revoke the base grouping (%s, %s)", targetID, orgGrouping(orgID))
}
