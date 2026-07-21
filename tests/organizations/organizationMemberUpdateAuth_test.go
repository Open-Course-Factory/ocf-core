package organizations_tests

// RED tests for the #426 scope extension: authorizing PATCH on the OrganizationMember
// entity (role/status change via the generic route).
//
// Today PATCH is a two-part gap:
//   - Layer 1: the Casbin Member policy for OrganizationMember is (GET|POST|DELETE) — no
//     PATCH — so org managers get 403 (admin-only), the wrong outcome.
//   - Layer 2: there is NO BeforeUpdate authorization hook on OrganizationMember. So if
//     PATCH is simply added to the Member policy, role changes become an UNPROTECTED write
//     for every member — a two-layer-rule violation.
//
// The fix does both together. These tests pin the Layer-2 contract of the new
// BeforeUpdate authorization hook (mirroring the existing create-side
// OrganizationMemberValidationHook and delete-side OrganizationMemberDeletionHook) plus
// the Layer-1 policy addition.
//
// The hook tests drive the REAL org hooks through the global registry (populated by
// InitOrganizationHooks, exactly as production) and assert on the user-observable outcome:
// the error the generic update path surfaces to the caller. RED today: no BeforeUpdate
// authorization hook is registered for "OrganizationMember", so ExecuteHooks runs no hook,
// returns nil, and the update is allowed to proceed — so the deny assertions fail.
//
// BeforeUpdate context shape (genericService.EditEntityWithUser, genericService.go:280-289,
// and OrganizationPlanProtectionHook's BeforeUpdate branch): ctx.OldEntity is the loaded
// current row (*models.OrganizationMember), ctx.NewEntity is the patch map[string]any whose
// "role" key carries a models.OrganizationMemberRole (per the registration's DtoToMap). The
// hook must read the actor's org role and the target's current role from these, NOT trust
// the patch for identity.
//
// Shared helpers newOrgRoleCapDB / seedOrgWithMembers (organizationMemberRoleCap_test.go),
// installMockEnforcer / registerOrgHooks (organizationMemberPermissionSync_test.go), and the
// platformMember var are reused.

import (
	"net/http"
	"testing"

	authModels "soli/formations/src/auth/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	"soli/formations/src/entityManagement/hooks"
	orgEntityRegistration "soli/formations/src/organizations/entityRegistration"
	"soli/formations/src/organizations/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// runMemberUpdateAuth drives a BeforeUpdate on an existing OrganizationMember through the
// registry and returns the error the update path would surface. oldRole is the target's
// current role (from the loaded row); patchRole is the role the caller is trying to set.
func runMemberUpdateAuth(
	t *testing.T,
	db *gorm.DB,
	orgID uuid.UUID,
	targetUserID string,
	oldRole models.OrganizationMemberRole,
	actorID string,
	actorPlatformRoles []string,
	patchRole models.OrganizationMemberRole,
) error {
	t.Helper()

	ctx := &hooks.HookContext{
		EntityName: "OrganizationMember",
		HookType:   hooks.BeforeUpdate,
		EntityID:   uuid.New(),
		OldEntity: &models.OrganizationMember{
			OrganizationID: orgID,
			UserID:         targetUserID,
			Role:           oldRole,
		},
		NewEntity: map[string]any{"role": patchRole}, // matches registration DtoToMap output type
		UserID:    actorID,
		UserRoles: actorPlatformRoles,
	}
	return hooks.GlobalHookRegistry.ExecuteHooks(ctx)
}

// TestOrgMemberUpdateAuth_EmptyActor_Denied — a role change requested by an unknown actor
// (ctx.UserID == "") must be denied (fail-closed), mirroring the create/delete hooks. RED
// today: no BeforeUpdate authorization hook, so ExecuteHooks returns nil (allowed).
func TestOrgMemberUpdateAuth_EmptyActor_Denied(t *testing.T) {
	const ownerID, targetID = "org-owner-account", "target-member"

	installMockEnforcer(t)
	db := newOrgRoleCapDB(t)
	orgID := seedOrgWithMembers(t, db, ownerID, map[string]models.OrganizationMemberRole{
		ownerID:  models.OrgRoleOwner,
		targetID: models.OrgRoleMember,
	})
	registerOrgHooks(t, db)

	err := runMemberUpdateAuth(t, db, orgID, targetID, models.OrgRoleMember, "", nil, models.OrgRoleManager)
	require.Error(t, err,
		"changing a member's role with an unknown actor (empty UserID) must be denied; the manage check must not be skipped for an empty actor")
}

// TestOrgMemberUpdateAuth_PlainMemberActor_Denied — a plain member of the org (no manage
// rights) must not be able to change another member's role. RED today.
func TestOrgMemberUpdateAuth_PlainMemberActor_Denied(t *testing.T) {
	const ownerID, actorID, targetID = "org-owner-account", "plain-member-actor", "target-member"

	installMockEnforcer(t)
	db := newOrgRoleCapDB(t)
	orgID := seedOrgWithMembers(t, db, ownerID, map[string]models.OrganizationMemberRole{
		ownerID:  models.OrgRoleOwner,
		actorID:  models.OrgRoleMember,
		targetID: models.OrgRoleMember,
	})
	registerOrgHooks(t, db)

	err := runMemberUpdateAuth(t, db, orgID, targetID, models.OrgRoleMember, actorID, platformMember, models.OrgRoleManager)
	require.Error(t, err,
		"a plain org member must not be able to change another member's role; only managers/owners may")
}

// TestOrgMemberUpdateAuth_ManagerActor_Allowed — an org manager may change a member's role
// (here: promote a member to a peer manager, manager50 >= manager50). Green now (no hook)
// and must STAY green after the fix — guards against the new hook over-restricting.
func TestOrgMemberUpdateAuth_ManagerActor_Allowed(t *testing.T) {
	const ownerID, actorID, targetID = "org-owner-account", "manager-actor", "target-member"

	installMockEnforcer(t)
	db := newOrgRoleCapDB(t)
	orgID := seedOrgWithMembers(t, db, ownerID, map[string]models.OrganizationMemberRole{
		ownerID:  models.OrgRoleOwner,
		actorID:  models.OrgRoleManager,
		targetID: models.OrgRoleMember,
	})
	registerOrgHooks(t, db)

	err := runMemberUpdateAuth(t, db, orgID, targetID, models.OrgRoleMember, actorID, platformMember, models.OrgRoleManager)
	require.NoError(t, err,
		"an org manager must be allowed to change a member's role within their own rank (promote member to manager)")
}

// TestOrgMemberUpdateAuth_ManagerPromotesToOwner_Denied — role-cap mirror of the create-side
// step 5b (organizationHooks.go:330-343): a manager must not promote someone to owner
// (owner100 > manager50). RED today.
func TestOrgMemberUpdateAuth_ManagerPromotesToOwner_Denied(t *testing.T) {
	const ownerID, actorID, targetID = "org-owner-account", "manager-actor", "target-member"

	installMockEnforcer(t)
	db := newOrgRoleCapDB(t)
	orgID := seedOrgWithMembers(t, db, ownerID, map[string]models.OrganizationMemberRole{
		ownerID:  models.OrgRoleOwner,
		actorID:  models.OrgRoleManager,
		targetID: models.OrgRoleMember,
	})
	registerOrgHooks(t, db)

	err := runMemberUpdateAuth(t, db, orgID, targetID, models.OrgRoleMember, actorID, platformMember, models.OrgRoleOwner)
	require.Error(t, err,
		"a manager must not be able to promote a member to owner (owner100 > manager50); the role cap must apply on update as on create")
}

// TestOrgMemberUpdateAuth_AdminPromotesToOwner_Allowed — a platform administrator bypasses
// the role cap. The actor is seeded as a manager (so the manage check resolves) but carries
// UserRoles=["administrator"]. Green now and must STAY green after the fix.
func TestOrgMemberUpdateAuth_AdminPromotesToOwner_Allowed(t *testing.T) {
	const ownerID, actorID, targetID = "org-owner-account", "manager-actor", "target-member"

	installMockEnforcer(t)
	db := newOrgRoleCapDB(t)
	orgID := seedOrgWithMembers(t, db, ownerID, map[string]models.OrganizationMemberRole{
		ownerID:  models.OrgRoleOwner,
		actorID:  models.OrgRoleManager,
		targetID: models.OrgRoleMember,
	})
	registerOrgHooks(t, db)

	err := runMemberUpdateAuth(t, db, orgID, targetID, models.OrgRoleMember, actorID, []string{string(authModels.Admin)}, models.OrgRoleOwner)
	require.NoError(t, err,
		"a platform administrator must bypass the role cap and be able to promote to owner")
}

// TestOrgMemberUpdateAuth_ChangingOwnerRole_Denied — changing the role of an existing owner
// member must be denied, mirroring the delete hook's cannot-remove-owner guard. The actor is
// a manager who can otherwise manage the org, so the denial is attributable to the
// owner-target guard, not a missing manage right. RED today.
func TestOrgMemberUpdateAuth_ChangingOwnerRole_Denied(t *testing.T) {
	const ownerID, actorID = "org-owner-account", "manager-actor"

	installMockEnforcer(t)
	db := newOrgRoleCapDB(t)
	orgID := seedOrgWithMembers(t, db, ownerID, map[string]models.OrganizationMemberRole{
		ownerID: models.OrgRoleOwner,
		actorID: models.OrgRoleManager,
	})
	registerOrgHooks(t, db)

	err := runMemberUpdateAuth(t, db, orgID, ownerID, models.OrgRoleOwner, actorID, platformMember, models.OrgRoleMember)
	require.Error(t, err,
		"changing the role of an organization owner must be denied, mirroring the cannot-remove-owner guard")
}

// runMemberStatusPatchAuth drives a BeforeUpdate whose patch touches only is_active (no
// "role" key) through the registry and returns the error the update path would surface. This
// is the role-absent path where the manage check is the SOLE barrier: the role cap and the
// owner guard both sit behind a role-change check and never run, so nothing else stands
// between a plain member and deactivating an arbitrary member.
func runMemberStatusPatchAuth(
	t *testing.T,
	orgID uuid.UUID,
	targetUserID string,
	oldRole models.OrganizationMemberRole,
	actorID string,
	actorPlatformRoles []string,
) error {
	t.Helper()

	ctx := &hooks.HookContext{
		EntityName: "OrganizationMember",
		HookType:   hooks.BeforeUpdate,
		EntityID:   uuid.New(),
		OldEntity: &models.OrganizationMember{
			OrganizationID: orgID,
			UserID:         targetUserID,
			Role:           oldRole,
		},
		NewEntity: map[string]any{"is_active": false}, // no "role" key: role-absent patch
		UserID:    actorID,
		UserRoles: actorPlatformRoles,
	}
	return hooks.GlobalHookRegistry.ExecuteHooks(ctx)
}

// TestOrgMemberUpdateAuth_PlainMemberActor_RoleAbsentPatch_Denied — a plain org member must
// not be able to change another member's status (is_active) via a role-absent patch. This
// isolates the manage check: on the role-absent path the role cap and owner guard do not run,
// so the manage check is the only barrier. Without it, any member could deactivate any other
// member. Green against current production; the manage-check mutation must flip it red.
func TestOrgMemberUpdateAuth_PlainMemberActor_RoleAbsentPatch_Denied(t *testing.T) {
	const ownerID, actorID, targetID = "org-owner-account", "plain-member-actor", "target-member"

	installMockEnforcer(t)
	db := newOrgRoleCapDB(t)
	orgID := seedOrgWithMembers(t, db, ownerID, map[string]models.OrganizationMemberRole{
		ownerID:  models.OrgRoleOwner,
		actorID:  models.OrgRoleMember,
		targetID: models.OrgRoleMember,
	})
	registerOrgHooks(t, db)

	err := runMemberStatusPatchAuth(t, orgID, targetID, models.OrgRoleMember, actorID, platformMember)
	require.Error(t, err,
		"a plain org member must not be able to deactivate another member via a role-absent patch; the manage check is the sole barrier on this path")
}

// TestOrgMemberUpdateAuth_ManagerActor_RoleAbsentPatch_Allowed — a manager may change a
// member's status via a role-absent patch. Pins the early-return path (manage check passes,
// no role change to cap). Green now and must stay green.
func TestOrgMemberUpdateAuth_ManagerActor_RoleAbsentPatch_Allowed(t *testing.T) {
	const ownerID, actorID, targetID = "org-owner-account", "manager-actor", "target-member"

	installMockEnforcer(t)
	db := newOrgRoleCapDB(t)
	orgID := seedOrgWithMembers(t, db, ownerID, map[string]models.OrganizationMemberRole{
		ownerID:  models.OrgRoleOwner,
		actorID:  models.OrgRoleManager,
		targetID: models.OrgRoleMember,
	})
	registerOrgHooks(t, db)

	err := runMemberStatusPatchAuth(t, orgID, targetID, models.OrgRoleMember, actorID, platformMember)
	require.NoError(t, err,
		"an org manager must be allowed to change a member's status via a role-absent patch")
}

// TestOrgMemberRegistration_MemberRoleIncludesPatch — Layer-1 pin asserted against the REAL
// production registration (not the hand-maintained rbac_matrix mirror): the Member role for
// the OrganizationMember entity must grant PATCH so org managers can change member roles. RED
// today: the registration grants Member only (GET|POST|DELETE).
func TestOrgMemberRegistration_MemberRoleIncludesPatch(t *testing.T) {
	svc := ems.NewEntityRegistrationService()
	orgEntityRegistration.RegisterOrganizationMember(svc)

	roles, ok := svc.GetAllEntityRoles()["OrganizationMember"]
	require.True(t, ok, "OrganizationMember must be registered with a roles map")

	memberRegex := roles.Roles[string(authModels.Member)]
	require.Contains(t, memberRegex, http.MethodPatch,
		"the Member role for OrganizationMember must include PATCH so org managers can change member roles; got %q", memberRegex)
}
