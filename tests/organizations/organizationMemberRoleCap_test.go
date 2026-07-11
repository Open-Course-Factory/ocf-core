package organizations_tests

// Tests for M4: OrganizationMemberValidationHook must cap the role a granter can
// assign to a new member at the granter's own role. Today the hook checks that the
// granter can *manage* the org (manager+), then only DEFAULTS an empty role to
// "member" (step 5) — it NEVER caps the assigned role. So a manager can create an
// OrganizationMember with Role="owner" (the client supplies Role; the binding tag is
// inert on the generic create path). The rule to enforce is:
//
//	granter role >= assigned role   (access.IsRoleAtLeast, owner100 > manager50 > member10)
//
// Equal is allowed (a manager may add a peer manager). Platform administrators
// (ctx.IsAdmin()) bypass the cap entirely.
//
// These tests drive the hook directly via hook.Execute(ctx) and assert the
// user-observable outcome (the returned error), which is the reject signal the
// generic create path surfaces to the caller.

import (
	"testing"
	"time"

	"soli/formations/src/entityManagement/hooks"
	organizationHooks "soli/formations/src/organizations/hooks"
	"soli/formations/src/organizations/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// newOrgRoleCapDB creates a fresh in-memory SQLite DB with the organizations and
// organization_members tables migrated.
func newOrgRoleCapDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err, "failed to open in-memory SQLite DB")

	err = db.AutoMigrate(
		&models.Organization{},
		&models.OrganizationMember{},
	)
	require.NoError(t, err, "failed to auto-migrate Organization / OrganizationMember")

	return db
}

// seedOrgWithMembers creates a team organization owned by ownerUserID and inserts a
// real OrganizationMember row for every (userID -> role) pair in members. Seeding the
// granter as a real member row is what lets GetUserOrganizationRole / the
// CanUserManageOrganization check resolve the granter's role during hook execution.
// Returns the organization ID.
func seedOrgWithMembers(t *testing.T, db *gorm.DB, ownerUserID string, members map[string]models.OrganizationMemberRole) uuid.UUID {
	t.Helper()

	orgID := uuid.New()
	org := &models.Organization{
		Name:             "RoleCapOrg",
		DisplayName:      "Role Cap Org",
		OwnerUserID:      ownerUserID,
		OrganizationType: models.OrgTypeTeam,
		MaxGroups:        250,
		MaxMembers:       50,
		IsActive:         true,
	}
	org.ID = orgID
	// Omit jsonb / array relations to avoid SQLite serialisation edge-cases; they are
	// irrelevant to the role-cap logic. Mirrors organization_member_visibility_test.go.
	err := db.Omit("Metadata", "OwnerIDs", "Members", "Groups").Create(org).Error
	require.NoError(t, err, "failed to create org")

	for userID, role := range members {
		m := &models.OrganizationMember{
			OrganizationID: orgID,
			UserID:         userID,
			Role:           role,
			JoinedAt:       time.Now(),
			IsActive:       true,
		}
		err := db.Omit("Metadata").Create(m).Error
		require.NoError(t, err, "failed to seed member %s (role %s)", userID, role)
	}

	return orgID
}

// runRoleCapCase seeds an org whose owner_user_id is a distinct account, seeds the
// granter as a member with granterRole (so the step-4 manage check and the granter's
// org-role lookup both resolve), then runs the member-validation hook for a brand-new
// target being assigned assignedRole. granterPlatformRoles is the Layer-1 platform role
// set (e.g. ["member"], or ["administrator"] for the admin-bypass guard). Returns the
// hook's error.
func runRoleCapCase(
	t *testing.T,
	granterRole models.OrganizationMemberRole,
	granterPlatformRoles []string,
	assignedRole models.OrganizationMemberRole,
) error {
	t.Helper()

	const (
		ownerID   = "org-owner-account"
		granterID = "granter-account"
		targetID  = "brand-new-target"
	)

	seededMembers := map[string]models.OrganizationMemberRole{
		ownerID: models.OrgRoleOwner,
	}
	if granterRole != "" {
		seededMembers[granterID] = granterRole
	}

	db := newOrgRoleCapDB(t)
	orgID := seedOrgWithMembers(t, db, ownerID, seededMembers)

	hook := organizationHooks.NewOrganizationMemberValidationHook(db)

	ctx := &hooks.HookContext{
		EntityName: "OrganizationMember",
		HookType:   hooks.BeforeCreate,
		NewEntity: &models.OrganizationMember{
			OrganizationID: orgID,
			UserID:         targetID,
			Role:           assignedRole,
		},
		UserID:    granterID,
		UserRoles: granterPlatformRoles,
	}

	return hook.Execute(ctx)
}

// memberRole is the platform-level role every real user carries (Layer 1). All
// students, teachers, trainers and org managers are platform "member".
var platformMember = []string{"member"}

// TestOrgMemberValidationHook_ManagerAssignsOwner_Rejected is the core M4 guard:
// a granter who is only a manager must NOT be able to mint an owner. Expected RED
// today (no cap → hook returns nil and the owner assignment is accepted).
func TestOrgMemberValidationHook_ManagerAssignsOwner_Rejected(t *testing.T) {
	err := runRoleCapCase(t, models.OrgRoleManager, platformMember, models.OrgRoleOwner)
	require.Error(t, err,
		"a manager must not be able to assign the owner role (owner100 > manager50); "+
			"the hook must reject, but it returned nil")
}

// TestOrgMemberValidationHook_ManagerAssignsManager_Allowed is a green guard against
// over-restriction: a manager may add a peer manager because the rule is >= (equal is
// allowed), not strictly-greater.
func TestOrgMemberValidationHook_ManagerAssignsManager_Allowed(t *testing.T) {
	err := runRoleCapCase(t, models.OrgRoleManager, platformMember, models.OrgRoleManager)
	require.NoError(t, err,
		"a manager assigning manager must be allowed (manager50 >= manager50); "+
			"the cap must not over-restrict equal roles")
}

// TestOrgMemberValidationHook_ManagerAssignsMember_Allowed is a green guard: a manager
// may add a plain member (manager50 >= member10).
func TestOrgMemberValidationHook_ManagerAssignsMember_Allowed(t *testing.T) {
	err := runRoleCapCase(t, models.OrgRoleManager, platformMember, models.OrgRoleMember)
	require.NoError(t, err,
		"a manager assigning member must be allowed (manager50 >= member10)")
}

// TestOrgMemberValidationHook_OwnerAssignsOwner_Allowed is a green guard: an owner may
// assign owner (owner100 >= owner100).
func TestOrgMemberValidationHook_OwnerAssignsOwner_Allowed(t *testing.T) {
	err := runRoleCapCase(t, models.OrgRoleOwner, platformMember, models.OrgRoleOwner)
	require.NoError(t, err,
		"an owner assigning owner must be allowed (owner100 >= owner100)")
}

// TestOrgMemberValidationHook_AdminManagerBypassesCap_AssignsOwner_Allowed asserts
// that a platform administrator bypasses the role CAP specifically. The granter is
// seeded as a real manager member (so the pre-existing step-4 CanUserManageOrganization
// check passes and the granter's org role resolves to manager) and carries
// UserRoles=["administrator"]. Assigning owner would normally be rejected by the cap
// (manager50 < owner100), but ctx.IsAdmin() bypasses it.
//
// This is a GREEN guard: green today (no cap exists) and must STAY green after the
// fix — proving the new cap does not newly block an admin manager.
func TestOrgMemberValidationHook_AdminManagerBypassesCap_AssignsOwner_Allowed(t *testing.T) {
	err := runRoleCapCase(t, models.OrgRoleManager, []string{"administrator"}, models.OrgRoleOwner)
	require.NoError(t, err,
		"a platform admin (ctx.IsAdmin()) who is a manager member must be able to assign owner; "+
			"the role cap must be bypassed for administrators (manager50 < owner100 would otherwise reject)")
}
