package organizations_tests

// #427: organization_members.is_active was stored but never enforced on the
// authorization predicates the hooks run. The Layer-2 middleware
// (GormMembershipChecker) and the effective-plan resolver already filter on
// it, and offboarding is what sets it false; the service predicates read the
// row through organizationRepository.GetOrganizationMember, which returned an
// offboarded manager as if nothing had happened. That lookup is the one place
// to enforce it, so every predicate built on it agrees with the middleware.

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	groupModels "soli/formations/src/groups/models"
	"soli/formations/src/organizations/models"
	"soli/formations/src/organizations/services"
)

// seedOrgWithOffboardedManager seeds an org whose "offboarded-manager" holds
// the manager role but is no longer active, plus one class in that org.
func seedOrgWithOffboardedManager(t *testing.T, db *gorm.DB) (orgID, classID uuid.UUID) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&groupModels.ClassGroup{}))
	orgID = seedOrgWithMembers(t, db, "org-owner-account", map[string]models.OrganizationMemberRole{
		"org-owner-account":  models.OrgRoleOwner,
		"offboarded-manager": models.OrgRoleManager,
	})
	require.NoError(t, db.Model(&models.OrganizationMember{}).
		Where("organization_id = ? AND user_id = ?", orgID, "offboarded-manager").
		Update("is_active", false).Error)

	class := &groupModels.ClassGroup{Name: "class-427", DisplayName: "Class", OwnerUserID: "org-owner-account", OrganizationID: &orgID}
	class.ID = uuid.New()
	require.NoError(t, db.Omit("Metadata").Create(class).Error)
	return orgID, class.ID
}

func TestOffboardedManager_IsNotInTheOrganization(t *testing.T) {
	db := newOrgRoleCapDB(t)
	orgID, _ := seedOrgWithOffboardedManager(t, db)

	in, err := services.NewOrganizationService(db).IsUserInOrganization(orgID, "offboarded-manager")
	require.NoError(t, err)
	assert.False(t, in)
}

func TestOffboardedManager_HasNoOrganizationRole(t *testing.T) {
	db := newOrgRoleCapDB(t)
	orgID, _ := seedOrgWithOffboardedManager(t, db)

	_, err := services.NewOrganizationService(db).GetUserOrganizationRole(orgID, "offboarded-manager")
	assert.Error(t, err, "a deactivated membership resolves to no role, not to its former one")
}

func TestOffboardedManager_CannotManageTheOrganization(t *testing.T) {
	db := newOrgRoleCapDB(t)
	orgID, _ := seedOrgWithOffboardedManager(t, db)

	canManage, err := services.NewOrganizationService(db).CanUserManageOrganization(orgID, "offboarded-manager")
	require.NoError(t, err)
	assert.False(t, canManage)
}

func TestOffboardedManager_CannotReachTheOrganizationsClassesViaOrg(t *testing.T) {
	db := newOrgRoleCapDB(t)
	_, classID := seedOrgWithOffboardedManager(t, db)

	canAccess, err := services.NewOrganizationService(db).CanUserAccessGroupViaOrg(classID, "offboarded-manager")
	require.NoError(t, err)
	assert.False(t, canAccess)
}

// Guard against over-restriction: the active owner keeps every right.
func TestActiveOwner_StillManagesTheOrganization(t *testing.T) {
	db := newOrgRoleCapDB(t)
	orgID, classID := seedOrgWithOffboardedManager(t, db)
	svc := services.NewOrganizationService(db)

	canManage, err := svc.CanUserManageOrganization(orgID, "org-owner-account")
	require.NoError(t, err)
	assert.True(t, canManage)
	canAccess, err := svc.CanUserAccessGroupViaOrg(classID, "org-owner-account")
	require.NoError(t, err)
	assert.True(t, canAccess)
}
