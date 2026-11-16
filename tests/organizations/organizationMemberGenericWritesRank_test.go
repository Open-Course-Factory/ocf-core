// tests/organizations/organizationMemberGenericWritesRank_test.go
//
// The generic organization-member PATCH and DELETE follow the rank rule of
// offboarding and reinstating: a caller acts only on a member they outrank (the
// owner on anyone but a platform administrator, a manager on plain members).
// is_active belongs to offboard / reinstate, and an offboarded row belongs to
// its offboarding: reinstate or erase are the ways out, not a generic delete.
package organizations_tests

import (
	"testing"
	"time"

	ems "soli/formations/src/entityManagement/entityManagementService"
	"soli/formations/src/entityManagement/hooks"
	orgRegistration "soli/formations/src/organizations/entityRegistration"
	"soli/formations/src/organizations/models"

	"github.com/google/uuid"
	"github.com/mitchellh/mapstructure"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// seedGenericWritesOrg seeds owner-1 (owner), manager-1 and manager-2
// (managers), student-1 (member) and admin-1 (a member who is a platform
// administrator), with the real organization hooks registered.
func seedGenericWritesOrg(t *testing.T) (*gorm.DB, uuid.UUID) {
	t.Helper()
	installMockEnforcer(t).GetRolesForUserFunc = adminRoleFor("admin-1")
	db := newOrgRoleCapDB(t)
	orgID := seedOrgWithMembers(t, db, "owner-1", map[string]models.OrganizationMemberRole{
		"owner-1":   models.OrgRoleOwner,
		"manager-1": models.OrgRoleManager,
		"manager-2": models.OrgRoleManager,
		"student-1": models.OrgRoleMember,
		"admin-1":   models.OrgRoleMember,
	})
	registerOrgHooks(t, db)
	return db, orgID
}

func markOffboarded(t *testing.T, db *gorm.DB, orgID uuid.UUID, userID string) {
	t.Helper()
	now := time.Now()
	require.NoError(t, db.Model(&models.OrganizationMember{}).
		Where("organization_id = ? AND user_id = ?", orgID, userID).
		Updates(map[string]any{"is_active": false, "left_at": now, "scheduled_erasure_at": now.AddDate(0, 0, 10)}).Error)
}

// genericPatch runs body through the generic PATCH path: decoded and mapped by
// the real registration, authorized by the real BeforeUpdate hooks against the
// stored row, then written when authorized. Returns the hook error.
func genericPatch(t *testing.T, db *gorm.DB, orgID uuid.UUID, actor, target string, body map[string]any) error {
	t.Helper()
	svc := ems.NewEntityRegistrationService()
	orgRegistration.RegisterOrganizationMember(svc)
	ops, ok := svc.GetEntityOps("OrganizationMember")
	require.True(t, ok)
	edit := ops.NewEditDto()
	require.NoError(t, mapstructure.Decode(body, &edit))
	updates, err := ops.ConvertEditDtoToMap(edit)
	require.NoError(t, err)

	current := loadMember(t, db, orgID, target)
	err = hooks.GlobalHookRegistry.ExecuteHooks(&hooks.HookContext{
		EntityName: "OrganizationMember",
		HookType:   hooks.BeforeUpdate,
		EntityID:   current.ID,
		OldEntity:  &current,
		NewEntity:  updates,
		UserID:     actor,
		UserRoles:  platformMember,
	})
	delete(updates, "metadata") // SQLite cannot bind the jsonb map; no test reads it back
	if err == nil && len(updates) > 0 {
		require.NoError(t, db.Model(&models.OrganizationMember{}).Where("id = ?", current.ID).Updates(updates).Error)
	}
	return err
}

// genericDelete runs the real BeforeDelete hooks on the stored row, as the
// generic DELETE does. Returns the hook error.
func genericDelete(t *testing.T, db *gorm.DB, orgID uuid.UUID, actor string, actorRoles []string, target string) error {
	t.Helper()
	current := loadMember(t, db, orgID, target)
	return hooks.GlobalHookRegistry.ExecuteHooks(&hooks.HookContext{
		EntityName: "OrganizationMember",
		HookType:   hooks.BeforeDelete,
		EntityID:   current.ID,
		NewEntity:  &current,
		UserID:     actor,
		UserRoles:  actorRoles,
	})
}

func TestOrgMemberGenericPatch_IsActive_NeitherReactivatesNorDeactivates(t *testing.T) {
	for _, actor := range []string{"manager-1", "owner-1"} {
		t.Run(actor, func(t *testing.T) {
			db, orgID := seedGenericWritesOrg(t)
			markOffboarded(t, db, orgID, "student-1")

			_ = genericPatch(t, db, orgID, actor, "student-1", map[string]any{"is_active": true})
			_ = genericPatch(t, db, orgID, actor, "admin-1", map[string]any{"is_active": false})

			offboarded := loadMember(t, db, orgID, "student-1")
			assert.False(t, offboarded.IsActive, "only reinstate brings an offboarded member back")
			assert.NotNil(t, offboarded.LeftAt)
			assert.True(t, loadMember(t, db, orgID, "admin-1").IsActive, "only offboard deactivates a member")
		})
	}
}

func TestOrgMemberGenericPatch_RoleChange_FollowsTheRankRule(t *testing.T) {
	for _, c := range []struct {
		name, actor, target string
		role                models.OrganizationMemberRole
		allowed             bool
	}{
		{"manager demotes a peer manager", "manager-1", "manager-2", models.OrgRoleMember, false},
		{"manager promotes a member who is a platform administrator", "manager-1", "admin-1", models.OrgRoleManager, false},
		{"owner demotes a manager", "owner-1", "manager-2", models.OrgRoleMember, true},
		{"manager promotes a plain member", "manager-1", "student-1", models.OrgRoleManager, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			db, orgID := seedGenericWritesOrg(t)
			before := loadMember(t, db, orgID, c.target).Role

			err := genericPatch(t, db, orgID, c.actor, c.target, map[string]any{"role": string(c.role)})

			if c.allowed {
				require.NoError(t, err)
				assert.Equal(t, c.role, loadMember(t, db, orgID, c.target).Role)
				return
			}
			require.Error(t, err)
			assert.Equal(t, before, loadMember(t, db, orgID, c.target).Role)
		})
	}
}

// A patch without a role still writes the member: the caller must outrank them.
func TestOrgMemberGenericPatch_MetadataOnly_FollowsTheRankRule(t *testing.T) {
	db, orgID := seedGenericWritesOrg(t)
	body := map[string]any{"metadata": map[string]any{"note": "x"}}

	assert.Error(t, genericPatch(t, db, orgID, "manager-1", "manager-2", body), "a peer manager is out of reach")
	assert.Error(t, genericPatch(t, db, orgID, "manager-1", "admin-1", body), "a platform administrator is out of reach")
}

func TestOrgMemberGenericDelete_FollowsTheRankRule(t *testing.T) {
	for _, c := range []struct {
		name, actor, target string
		allowed             bool
	}{
		{"manager removes a peer manager", "manager-1", "manager-2", false},
		{"manager removes a member who is a platform administrator", "manager-1", "admin-1", false},
		{"owner removes a manager", "owner-1", "manager-2", true},
		{"manager removes a plain member", "manager-1", "student-1", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			db, orgID := seedGenericWritesOrg(t)

			err := genericDelete(t, db, orgID, c.actor, platformMember, c.target)

			if c.allowed {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestOrgMemberGenericDelete_OffboardedRow_Refused(t *testing.T) {
	for _, c := range []struct {
		actor string
		roles []string
	}{
		{"owner-1", platformMember},
		{"platform-admin", platformAdministrator},
	} {
		t.Run(c.actor, func(t *testing.T) {
			db, orgID := seedGenericWritesOrg(t)
			markOffboarded(t, db, orgID, "student-1")

			err := genericDelete(t, db, orgID, c.actor, c.roles, "student-1")

			assert.Error(t, err, "the offboarding owns this row: reinstate or erase it")
		})
	}
}
