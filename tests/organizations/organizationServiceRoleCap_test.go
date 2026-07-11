package organizations_tests

// Tests for #410 (audit-P, MR P): organizationService.UpdateMemberRole must cap the
// role a granter can assign at the granter's own rank. Today the method checks that the
// granter can *manage* the org (manager+), then updates the role WITHOUT capping it — so
// a manager could promote a member to owner (reopening M4 the moment this currently-
// unrouted method is wired to a handler).
//
// The rule to enforce, after the existing CanUserManageOrganization gate and before the
// write:
//
//	granter role >= newRole   (access.IsRoleAtLeast, owner100 > manager50 > member10)
//
// with an owner short-circuit: when requestingUserID == org.OwnerUserID the cap is
// skipped (the owner may assign any role). Equal is allowed (a manager may set manager).
//
// These tests drive the real service (NewOrganizationService) against an in-memory DB and
// assert the USER-OBSERVABLE outcome: the returned error AND the persisted role read back
// from organization_members — never a mock-call count.
//
// Shared helpers newOrgRoleCapDB / seedOrgWithMembers live in
// organizationMemberRoleCap_test.go (same package) and are reused here.

import (
	"testing"

	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/mocks"
	"soli/formations/src/organizations/models"
	orgServices "soli/formations/src/organizations/services"

	"github.com/stretchr/testify/require"
)

// runOrgServiceUpdateRoleCase seeds a team org whose owner_user_id is ownerID, seeds the
// granter as a real member with granterRole (so both the CanUserManageOrganization gate
// and the granter's org-role lookup resolve), and seeds the target as a member with
// initialTargetRole. It then calls organizationService.UpdateMemberRole(orgID, granterID,
// targetID, newRole) and returns the method error plus the target's role read back from
// organization_members.
//
// A MockEnforcer is installed for the duration of the call because the ALLOWED branches of
// UpdateMemberRole grant/revoke Casbin manager permissions; without it the real (nil)
// enforcer would panic on the green cases. The mock has no bearing on the cap logic.
func runOrgServiceUpdateRoleCase(
	t *testing.T,
	ownerID string,
	granterID string,
	granterRole models.OrganizationMemberRole,
	initialTargetRole models.OrganizationMemberRole,
	newRole models.OrganizationMemberRole,
) (error, models.OrganizationMemberRole) {
	t.Helper()

	const targetID = "role-cap-target"

	seeded := map[string]models.OrganizationMemberRole{
		ownerID:  models.OrgRoleOwner,
		targetID: initialTargetRole,
	}
	// When the granter is the owner itself, it is already seeded above with the owner role.
	if granterID != ownerID {
		seeded[granterID] = granterRole
	}

	db := newOrgRoleCapDB(t)
	orgID := seedOrgWithMembers(t, db, ownerID, seeded)

	originalEnforcer := casdoor.Enforcer
	casdoor.Enforcer = mocks.NewMockEnforcer()
	defer func() { casdoor.Enforcer = originalEnforcer }()

	svc := orgServices.NewOrganizationService(db)
	err := svc.UpdateMemberRole(orgID, granterID, targetID, newRole)

	var persisted models.OrganizationMember
	require.NoError(t,
		db.Where("organization_id = ? AND user_id = ?", orgID, targetID).First(&persisted).Error,
		"failed to read back the target member row")

	return err, persisted.Role
}

// TestOrgServiceUpdateMemberRole_ManagerPromotesToOwner_Rejected is the core M4 guard for
// the service path: a manager granter must NOT be able to promote a member to owner.
// Expected RED today (no cap → the update succeeds and the role becomes owner).
func TestOrgServiceUpdateMemberRole_ManagerPromotesToOwner_Rejected(t *testing.T) {
	err, role := runOrgServiceUpdateRoleCase(t,
		"org-owner-account", "manager-granter",
		models.OrgRoleManager, models.OrgRoleMember, models.OrgRoleOwner)

	require.Error(t, err,
		"a manager must not promote a member to owner (owner100 > manager50); "+
			"UpdateMemberRole must reject, but it returned nil")
	require.Equal(t, models.OrgRoleMember, role,
		"the rejected promotion must NOT persist — target role must remain member, got %q", role)
}

// TestOrgServiceUpdateMemberRole_ManagerSetsRoleAtOrBelowOwn_Allowed is a green guard
// against over-restriction: a manager may set manager (equal, >=) and member (below).
func TestOrgServiceUpdateMemberRole_ManagerSetsRoleAtOrBelowOwn_Allowed(t *testing.T) {
	t.Run("manager_to_manager", func(t *testing.T) {
		err, role := runOrgServiceUpdateRoleCase(t,
			"org-owner-account", "manager-granter",
			models.OrgRoleManager, models.OrgRoleMember, models.OrgRoleManager)
		require.NoError(t, err,
			"a manager assigning manager must be allowed (manager50 >= manager50)")
		require.Equal(t, models.OrgRoleManager, role,
			"the allowed update must persist — target role must be manager, got %q", role)
	})

	t.Run("manager_to_member", func(t *testing.T) {
		err, role := runOrgServiceUpdateRoleCase(t,
			"org-owner-account", "manager-granter",
			models.OrgRoleManager, models.OrgRoleManager, models.OrgRoleMember)
		require.NoError(t, err,
			"a manager assigning member must be allowed (manager50 >= member10)")
		require.Equal(t, models.OrgRoleMember, role,
			"the allowed update must persist — target role must be member, got %q", role)
	})
}

// TestOrgServiceUpdateMemberRole_OwnerPromotesToOwner_Allowed is a green guard for the
// owner short-circuit: when the granter is the org owner (requestingUserID ==
// org.OwnerUserID) the cap is skipped, so promoting a member to owner is allowed.
func TestOrgServiceUpdateMemberRole_OwnerPromotesToOwner_Allowed(t *testing.T) {
	const ownerID = "org-owner-account"
	err, role := runOrgServiceUpdateRoleCase(t,
		ownerID, ownerID,
		models.OrgRoleOwner, models.OrgRoleMember, models.OrgRoleOwner)

	require.NoError(t, err,
		"the org owner must be able to promote a member to owner (owner short-circuit)")
	require.Equal(t, models.OrgRoleOwner, role,
		"the owner-driven promotion must persist — target role must be owner, got %q", role)
}

// TestOrgServiceUpdateMemberRole_CannotChangeOwnersRole_Regression guards the pre-existing
// invariant that the org owner's own role cannot be changed away from owner. The cap must
// not remove this protection. (Green today; must stay green after the fix.)
func TestOrgServiceUpdateMemberRole_CannotChangeOwnersRole_Regression(t *testing.T) {
	const ownerID = "org-owner-account"

	db := newOrgRoleCapDB(t)
	orgID := seedOrgWithMembers(t, db, ownerID, map[string]models.OrganizationMemberRole{
		ownerID: models.OrgRoleOwner,
	})

	originalEnforcer := casdoor.Enforcer
	casdoor.Enforcer = mocks.NewMockEnforcer()
	defer func() { casdoor.Enforcer = originalEnforcer }()

	svc := orgServices.NewOrganizationService(db)
	// The owner tries to demote itself to member — ValidateNotOwner must block it.
	err := svc.UpdateMemberRole(orgID, ownerID, ownerID, models.OrgRoleMember)
	require.Error(t, err,
		"changing the organization owner's role must be rejected (ValidateNotOwner)")

	var persisted models.OrganizationMember
	require.NoError(t,
		db.Where("organization_id = ? AND user_id = ?", orgID, ownerID).First(&persisted).Error,
		"failed to read back the owner member row")
	require.Equal(t, models.OrgRoleOwner, persisted.Role,
		"the owner's role must remain owner after a rejected self-demotion, got %q", persisted.Role)
}
