package organizations_tests

// Tests for M5 (deny-when-actor-unknown), organization-member hook spots.
//
// Two org-member hooks gate their authorization check behind `if ctx.UserID != ""`:
//   - OrganizationMemberValidationHook.Execute  (add member, step 4 CanUserManageOrganization)
//   - OrganizationMemberDeletionHook.Execute    (remove member, step 2 CanUserManageOrganization)
//
// When the actor is unknown (ctx.UserID == ""), the guarded block is SKIPPED
// entirely, so the manage check never runs and the operation proceeds — a
// fail-open. The fix must DENY when the actor is empty on these
// externally-reachable member mutations.
//
// These tests drive the hooks directly and assert the user-observable outcome
// (the returned permission error). They are RED today (empty actor -> check
// skipped -> nil returned -> mutation allowed).
//
// Helpers newOrgRoleCapDB and seedOrgWithMembers are shared with
// organizationMemberRoleCap_test.go (same package) and are reused, not redefined.

import (
	"testing"

	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/mocks"
	"soli/formations/src/entityManagement/hooks"
	organizationHooks "soli/formations/src/organizations/hooks"
	"soli/formations/src/organizations/models"

	"github.com/stretchr/testify/require"
)

// useMockEnforcer installs a mock Casbin enforcer for the duration of a test.
// The deletion hook, when it (today) fails open on an empty actor, proceeds to the
// permission-revoke step, which dereferences the global casdoor.Enforcer. Without a
// mock that would nil-panic and mask the observable fail-open (a returned nil).
func useMockEnforcer(t *testing.T) {
	t.Helper()
	orig := casdoor.Enforcer
	casdoor.Enforcer = mocks.NewMockEnforcer()
	t.Cleanup(func() { casdoor.Enforcer = orig })
}

// TestOrgMemberAddHook_EmptyActor_Denied — adding a member with an unknown actor
// must be denied. RED today: the step-4 manage check is wrapped in
// `if ctx.UserID != ""`, so an empty actor skips it and the hook returns nil after
// defaulting the role.
func TestOrgMemberAddHook_EmptyActor_Denied(t *testing.T) {
	const ownerID = "org-owner-account"

	db := newOrgRoleCapDB(t)
	orgID := seedOrgWithMembers(t, db, ownerID, map[string]models.OrganizationMemberRole{
		ownerID: models.OrgRoleOwner,
	})

	hook := organizationHooks.NewOrganizationMemberValidationHook(db)

	ctx := &hooks.HookContext{
		EntityName: "OrganizationMember",
		HookType:   hooks.BeforeCreate,
		NewEntity: &models.OrganizationMember{
			OrganizationID: orgID,
			UserID:         "brand-new-target",
			Role:           models.OrgRoleMember,
		},
		UserID:    "", // unknown actor
		UserRoles: nil,
	}

	err := hook.Execute(ctx)
	require.Error(t, err,
		"adding an organization member with an unknown actor (empty UserID) must be denied; "+
			"the manage-permission check must not be skipped for an empty actor")
}

// TestOrgMemberDeleteHook_EmptyActor_Denied — removing a (non-owner) member with an
// unknown actor must be denied. RED today: the step-2 manage check is wrapped in
// `if ctx.UserID != ""`, so an empty actor skips it and the hook proceeds to revoke
// and returns nil.
func TestOrgMemberDeleteHook_EmptyActor_Denied(t *testing.T) {
	const (
		ownerID  = "org-owner-account"
		targetID = "member-to-remove"
	)

	useMockEnforcer(t)
	db := newOrgRoleCapDB(t)
	orgID := seedOrgWithMembers(t, db, ownerID, map[string]models.OrganizationMemberRole{
		ownerID:  models.OrgRoleOwner,
		targetID: models.OrgRoleMember,
	})

	hook := organizationHooks.NewOrganizationMemberDeletionHook(db)

	ctx := &hooks.HookContext{
		EntityName: "OrganizationMember",
		HookType:   hooks.BeforeDelete,
		NewEntity: &models.OrganizationMember{
			OrganizationID: orgID,
			UserID:         targetID,
			Role:           models.OrgRoleMember, // non-owner: step-1 owner guard does not short-circuit
		},
		UserID:    "", // unknown actor
		UserRoles: nil,
	}

	err := hook.Execute(ctx)
	require.Error(t, err,
		"removing an organization member with an unknown actor (empty UserID) must be denied; "+
			"the manage-permission check must not be skipped for an empty actor")
}
