package organizations_tests

import (
	"testing"
	"time"

	access "soli/formations/src/auth/access"
	"soli/formations/src/entityManagement/hooks"
	groupModels "soli/formations/src/groups/models"
	organizationHooks "soli/formations/src/organizations/hooks"
	organizationModels "soli/formations/src/organizations/models"
	paymentModels "soli/formations/src/payment/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Deleting an organization used to leave its classes behind, live: the row was
// soft-deleted and its class_groups were not touched, so a class whose
// organization no longer existed went on appearing on its teacher's console and
// every surface reading it went on answering.
//
// The classes are now retired instead — archived, not destroyed, and readable
// only by platform administrators. These tests pin all three properties,
// because each is carried by a different flag and any one of them regressing
// silently changes who can see a deleted organization's classes.

func archiveTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&paymentModels.SubscriptionPlan{},
		&organizationModels.Organization{},
		&organizationModels.OrganizationMember{},
		&groupModels.ClassGroup{},
		&groupModels.GroupMember{},
	))
	return db
}

// A team organization with one class, its owner enrolled and one learner.
func seedOrgWithClass(t *testing.T, db *gorm.DB) (organizationModels.Organization, groupModels.ClassGroup, string, string) {
	t.Helper()
	owner := uuid.NewString()
	learner := uuid.NewString()

	org := organizationModels.Organization{
		Name:             "acme",
		DisplayName:      "ACME",
		OwnerUserID:      owner,
		OrganizationType: organizationModels.OrgTypeTeam,
		IsActive:         true,
	}
	require.NoError(t, db.Omit("Metadata", "AllowedBackends", "Members", "Groups").Create(&org).Error)

	group := groupModels.ClassGroup{
		Name:           "promo-a",
		DisplayName:    "Promo A",
		OwnerUserID:    owner,
		OrganizationID: &org.ID,
		MaxMembers:     20,
		IsActive:       true,
	}
	require.NoError(t, db.Omit("Metadata", "Members", "SubGroups", "ParentGroup").Create(&group).Error)

	for _, m := range []struct {
		user string
		role groupModels.GroupMemberRole
	}{
		{owner, groupModels.GroupMemberRoleOwner},
		{learner, groupModels.GroupMemberRoleMember},
	} {
		member := groupModels.GroupMember{
			GroupID:  group.ID,
			UserID:   m.user,
			Role:     m.role,
			JoinedAt: time.Now(),
			IsActive: true,
		}
		require.NoError(t, db.Omit("Metadata").Create(&member).Error)
	}

	return org, group, owner, learner
}

func deleteOrganization(t *testing.T, db *gorm.DB, org organizationModels.Organization) error {
	t.Helper()
	hook := organizationHooks.NewOrganizationCleanupHook(db)
	return hook.Execute(&hooks.HookContext{
		HookType:  hooks.BeforeDelete,
		NewEntity: &org,
	})
}

func TestDeleteOrganization_ArchivesItsClasses(t *testing.T) {
	db := archiveTestDB(t)
	org, group, _, _ := seedOrgWithClass(t, db)

	require.NoError(t, deleteOrganization(t, db, org))

	var archived groupModels.ClassGroup
	require.NoError(t, db.First(&archived, "id = ?", group.ID).Error,
		"the class must survive its organization — past results name it")
	assert.False(t, archived.IsActive,
		"a class whose organization is gone must be archived, not left active")
}

func TestDeleteOrganization_LeavesTheClassReadableToAdmins(t *testing.T) {
	db := archiveTestDB(t)
	org, group, owner, learner := seedOrgWithClass(t, db)

	require.NoError(t, deleteOrganization(t, db, org))

	// Nothing is destroyed: the roster is still on record for whoever has to
	// answer for the class later.
	var members []groupModels.GroupMember
	require.NoError(t, db.Where("group_id = ?", group.ID).Find(&members).Error)
	assert.Len(t, members, 2, "the roster must survive, so the class stays answerable")

	roles := map[string]groupModels.GroupMemberRole{}
	for _, m := range members {
		roles[m.UserID] = m.Role
		assert.False(t, m.IsActive, "every membership is stood down with the class")
	}
	assert.Equal(t, groupModels.GroupMemberRoleOwner, roles[owner])
	assert.Equal(t, groupModels.GroupMemberRoleMember, roles[learner])
}

// The admin-only half, asserted through the enforcer that actually decides it
// rather than through the flag it reads: CheckGroupRole counts only active
// memberships, so standing the roster down is what refuses every non-admin.
// Platform administrators are bypassed before this check is ever reached.
func TestDeleteOrganization_RefusesTheClassToItsFormerStaff(t *testing.T) {
	db := archiveTestDB(t)
	org, group, owner, learner := seedOrgWithClass(t, db)
	checker := access.NewGormMembershipChecker(db)

	allowed, err := checker.CheckGroupRole(group.ID.String(), owner, "manager")
	require.NoError(t, err)
	require.True(t, allowed, "precondition: the owner manages the class while the org lives")

	require.NoError(t, deleteOrganization(t, db, org))

	allowed, err = checker.CheckGroupRole(group.ID.String(), owner, "manager")
	require.NoError(t, err)
	assert.False(t, allowed,
		"once the organization is deleted its classes are for administrators only")

	allowed, err = checker.CheckGroupRole(group.ID.String(), learner, "member")
	require.NoError(t, err)
	assert.False(t, allowed, "and the learners lose it too")
}

func TestDeleteOrganization_WithoutClassesIsNotAnError(t *testing.T) {
	db := archiveTestDB(t)
	org := organizationModels.Organization{
		Name:             "empty",
		DisplayName:      "Empty",
		OwnerUserID:      uuid.NewString(),
		OrganizationType: organizationModels.OrgTypeTeam,
		IsActive:         true,
	}
	require.NoError(t, db.Omit("Metadata", "AllowedBackends", "Members", "Groups").Create(&org).Error)

	assert.NoError(t, deleteOrganization(t, db, org),
		"an organization holding no class must delete as cleanly as one that does")
}

// The classes of OTHER organizations are none of this deletion's business — the
// archive is scoped by organization_id, and a bug there would retire a
// bystander's class silently.
func TestDeleteOrganization_LeavesOtherOrganizationsClassesAlone(t *testing.T) {
	db := archiveTestDB(t)
	doomed, _, _, _ := seedOrgWithClass(t, db)
	bystander, otherClass, otherOwner, _ := seedOrgWithClass(t, db)
	_ = bystander

	require.NoError(t, deleteOrganization(t, db, doomed))

	var survivor groupModels.ClassGroup
	require.NoError(t, db.First(&survivor, "id = ?", otherClass.ID).Error)
	assert.True(t, survivor.IsActive, "another organization's class must be untouched")

	checker := access.NewGormMembershipChecker(db)
	allowed, err := checker.CheckGroupRole(otherClass.ID.String(), otherOwner, "manager")
	require.NoError(t, err)
	assert.True(t, allowed, "and its owner must still manage it")
}
