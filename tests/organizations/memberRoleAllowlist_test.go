package organizations_tests

// #429: a platform administrator could persist any role string on an
// organization member ("garbage"), because the binding tag oneof=member manager
// is inert on the generic entity path (#390) and the admin bypasses the role
// cap, the only place the value was compared. A garbage role resolves to
// priority 0 everywhere afterwards. The hooks now check the role against the
// one registered hierarchy (access.IsKnownRole) before any cap.

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	entityErrors "soli/formations/src/entityManagement/errors"
	"soli/formations/src/entityManagement/hooks"
	organizationHooks "soli/formations/src/organizations/hooks"
	"soli/formations/src/organizations/models"
)

var platformAdministrator = []string{"administrator"}

// requireValidationRejection asserts the hook refused with the structured
// error the generic path maps to 400, not a permission refusal.
func requireValidationRejection(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	var structured *entityErrors.EntityError
	require.True(t, errors.As(err, &structured), "expected a structured entity error, got %T: %v", err, err)
	assert.Equal(t, http.StatusBadRequest, structured.HTTPStatus, "an unknown role is a client error: %v", err)
}

func TestOrgMemberValidationHook_AdminAssignsUnknownRole_Rejected(t *testing.T) {
	err := runRoleCapCase(t, models.OrgRoleOwner, platformAdministrator, models.OrganizationMemberRole("garbage"))
	requireValidationRejection(t, err)
}

func TestOrgMemberValidationHook_OwnerAssignsUnknownRole_Rejected(t *testing.T) {
	err := runRoleCapCase(t, models.OrgRoleOwner, platformMember, models.OrganizationMemberRole("garbage"))
	requireValidationRejection(t, err)
}

func TestOrgMemberUpdateHook_AdminPatchesUnknownRole_Rejected(t *testing.T) {
	db := newOrgRoleCapDB(t)
	orgID := seedOrgWithMembers(t, db, "org-owner-account", map[string]models.OrganizationMemberRole{
		"org-owner-account": models.OrgRoleOwner,
		"target-account":    models.OrgRoleMember,
	})

	ctx := &hooks.HookContext{
		EntityName: "OrganizationMember",
		HookType:   hooks.BeforeUpdate,
		OldEntity: &models.OrganizationMember{
			OrganizationID: orgID,
			UserID:         "target-account",
			Role:           models.OrgRoleMember,
		},
		NewEntity: map[string]any{"role": models.OrganizationMemberRole("garbage")},
		UserID:    "org-owner-account",
		UserRoles: platformAdministrator,
	}
	err := organizationHooks.NewOrganizationMemberUpdateAuthorizationHook(db).Execute(ctx)
	requireValidationRejection(t, err)
}

// Guard against over-restriction: a registered role still passes the allowlist.
func TestOrgMemberUpdateHook_OwnerPatchesKnownRole_Allowed(t *testing.T) {
	db := newOrgRoleCapDB(t)
	orgID := seedOrgWithMembers(t, db, "org-owner-account", map[string]models.OrganizationMemberRole{
		"org-owner-account": models.OrgRoleOwner,
		"target-account":    models.OrgRoleMember,
	})

	ctx := &hooks.HookContext{
		EntityName: "OrganizationMember",
		HookType:   hooks.BeforeUpdate,
		OldEntity: &models.OrganizationMember{
			OrganizationID: orgID,
			UserID:         "target-account",
			Role:           models.OrgRoleMember,
		},
		NewEntity: map[string]any{"role": models.OrgRoleManager},
		UserID:    "org-owner-account",
		UserRoles: platformMember,
	}
	require.NoError(t, organizationHooks.NewOrganizationMemberUpdateAuthorizationHook(db).Execute(ctx))
}
