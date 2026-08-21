package groups_tests

import (
	"testing"

	access "soli/formations/src/auth/access"
	groupModels "soli/formations/src/groups/models"
	groupServices "soli/formations/src/groups/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// A membership is only worth anything if the authorization layer accepts it.
//
// CheckGroupRole reads group_members and nothing else — it never consults
// class_groups.owner_user_id — so any path that records a member without
// writing that row produces someone the product treats as a member and every
// GroupRole gate refuses. The bulk import did exactly that: it set the owner on
// the class and wrote no membership, so the trainer who imported a class could
// not list the scenarios assignable to it, assign one, bulk-start it, or read
// its results, and each refusal surfaced as an empty list rather than an error.
//
// These tests pin the two halves of that invariant together: EnrolMember writes
// a row, and the row it writes is one CheckGroupRole honours.

func seedGroup(t *testing.T, db *gorm.DB, ownerUserID string) groupModels.ClassGroup {
	t.Helper()
	group := groupModels.ClassGroup{
		Name:        "import-created-class",
		DisplayName: "Import Created Class",
		OwnerUserID: ownerUserID,
		MaxMembers:  20,
		IsActive:    true,
	}
	// Omit the jsonb map: the SQLite driver these tests run on cannot bind one,
	// which is the same reason every other group fixture here omits it.
	require.NoError(t, db.Omit("Metadata", "Members", "SubGroups", "ParentGroup").Create(&group).Error)
	return group
}

func TestEnrolMember_OwnerPassesGroupRoleCheck(t *testing.T) {
	db := freshTestDB(t)
	owner := uuid.NewString()
	group := seedGroup(t, db, owner)

	service := groupServices.NewGroupService(db)
	require.NoError(t, service.EnrolMember(
		group.ID, owner, groupModels.GroupMemberRoleOwner, owner,
	))

	checker := access.NewGormMembershipChecker(db)

	// Manager is the bar the scenario-assignment routes set; an owner clears it.
	allowed, err := checker.CheckGroupRole(group.ID.String(), owner, "manager")
	require.NoError(t, err)
	assert.True(t, allowed,
		"the owner of a class must clear the manager gate on it — this is what a "+
			"CSV-imported class used to fail")
}

func TestEnrolMember_WritesAnActiveMembershipRow(t *testing.T) {
	db := freshTestDB(t)
	owner := uuid.NewString()
	group := seedGroup(t, db, owner)

	service := groupServices.NewGroupService(db)
	require.NoError(t, service.EnrolMember(
		group.ID, owner, groupModels.GroupMemberRoleOwner, owner,
	))

	var member groupModels.GroupMember
	require.NoError(t,
		db.Where("group_id = ? AND user_id = ?", group.ID, owner).First(&member).Error,
		"owning a class is not the same as being in it: the row has to exist")
	assert.Equal(t, groupModels.GroupMemberRoleOwner, member.Role)
	assert.True(t, member.IsActive, "an inactive row is invisible to CheckGroupRole")
}

// Setting owner_user_id alone — what the import used to do — leaves the class
// unmanageable. This is the failure the fix removes, asserted directly so the
// next person to write a group through raw GORM sees why it is not enough.
func TestGroupOwnerWithoutMembership_FailsGroupRoleCheck(t *testing.T) {
	db := freshTestDB(t)
	owner := uuid.NewString()
	group := seedGroup(t, db, owner)

	checker := access.NewGormMembershipChecker(db)
	allowed, err := checker.CheckGroupRole(group.ID.String(), owner, "manager")

	require.NoError(t, err)
	assert.False(t, allowed,
		"owner_user_id is invisible to the authorization layer — a writer that "+
			"sets it without enrolling the owner has created an unmanageable class")
}

func TestEnrolMember_LearnerGetsMemberRole(t *testing.T) {
	db := freshTestDB(t)
	owner := uuid.NewString()
	learner := uuid.NewString()
	group := seedGroup(t, db, owner)

	service := groupServices.NewGroupService(db)
	require.NoError(t, service.EnrolMember(
		group.ID, learner, groupModels.GroupMemberRoleMember, owner,
	))

	checker := access.NewGormMembershipChecker(db)

	allowed, err := checker.CheckGroupRole(group.ID.String(), learner, "member")
	require.NoError(t, err)
	assert.True(t, allowed, "an imported learner is a member of their class")

	allowed, err = checker.CheckGroupRole(group.ID.String(), learner, "manager")
	require.NoError(t, err)
	assert.False(t, allowed, "being enrolled must not promote a learner")
}
