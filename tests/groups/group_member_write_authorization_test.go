package groups_tests

// Tests for issue #288 (write-side coverage gap): verify that authorization
// for write operations on group members honors the org-role hierarchy and
// the direct group-role hierarchy.
//
// MR !201's fix is read-only (MembershipConfig.OrgIDViaParent is consumed by
// the generic membership filter on read paths). Writes go through a separate
// path: src/groups/hooks/groupHooks.go's GroupMemberValidationHook calls
// groupService.CanUserManageGroup() to gate creates.
//
// These tests target groupService.CanUserManageGroup() directly. They assert
// the existing behavior on main and serve as regression guards so future
// refactors of CanUserManageGroup or CanUserAccessGroupViaOrg cannot silently
// regress org-role-based group-member writes.
//
// Per issue #288 scope (read-only), these tests must NOT modify production
// code. If a scenario fails on current main, it indicates a separate latent
// bug and the test is skipped with a FIXME pointer.

import (
	"testing"
	"time"

	groupModels "soli/formations/src/groups/models"
	groupServices "soli/formations/src/groups/services"
	orgModels "soli/formations/src/organizations/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// addGroupMemberWithRole inserts a row in group_members with the given role.
// The seedFixture already inserts two plain "member" rows; this helper covers
// the cases where we need an explicit "manager" or "owner" group_members row.
func addGroupMemberWithRole(t *testing.T, db *gorm.DB, groupID uuid.UUID, userID string, role groupModels.GroupMemberRole) {
	t.Helper()
	gm := &groupModels.GroupMember{
		GroupID:  groupID,
		UserID:   userID,
		Role:     role,
		JoinedAt: time.Now(),
		IsActive: true,
	}
	err := db.Omit("Metadata").Create(gm).Error
	require.NoError(t, err, "failed to insert group member %s with role %s", userID, role)
}

// --- Tests ---

// TestGroupMember_WriteAuthz_OrgOwnerCanManage_True: the OWNER of the parent
// organization can manage (write to) GroupG, even when not a direct member.
func TestGroupMember_WriteAuthz_OrgOwnerCanManage_True(t *testing.T) {
	withFreshRegistration(func() {
		db := setupGroupMemberVisibilityDB(t)
		orgOneID, _, groupGID := seedFixture(t, db)

		// userA is OWNER of OrgOne, NOT a direct member of GroupG
		addOrgMember(t, db, orgOneID, "userA", orgModels.OrgRoleOwner)

		svc := groupServices.NewGroupService(db)
		canManage, err := svc.CanUserManageGroup(groupGID, "userA")
		require.NoError(t, err, "CanUserManageGroup returned an unexpected error for org owner")
		assert.True(t, canManage,
			"userA (org owner of OrgOne) MUST be allowed to manage GroupG (in OrgOne)")
	})
}

// TestGroupMember_WriteAuthz_OrgManagerCanManage_True: an org MANAGER can
// manage groups in that org, even without direct group membership.
func TestGroupMember_WriteAuthz_OrgManagerCanManage_True(t *testing.T) {
	withFreshRegistration(func() {
		db := setupGroupMemberVisibilityDB(t)
		orgOneID, _, groupGID := seedFixture(t, db)

		// userA is MANAGER of OrgOne, NOT a direct member of GroupG
		addOrgMember(t, db, orgOneID, "userA", orgModels.OrgRoleManager)

		svc := groupServices.NewGroupService(db)
		canManage, err := svc.CanUserManageGroup(groupGID, "userA")
		require.NoError(t, err, "CanUserManageGroup returned an unexpected error for org manager")
		assert.True(t, canManage,
			"userA (org manager of OrgOne) MUST be allowed to manage GroupG (in OrgOne)")
	})
}

// TestGroupMember_WriteAuthz_PlainOrgMemberCannotManage_False: a plain
// "member" of the parent org has NO write access to a group in that org.
func TestGroupMember_WriteAuthz_PlainOrgMemberCannotManage_False(t *testing.T) {
	withFreshRegistration(func() {
		db := setupGroupMemberVisibilityDB(t)
		orgOneID, _, groupGID := seedFixture(t, db)

		// userA is a plain MEMBER of OrgOne (not owner/manager)
		addOrgMember(t, db, orgOneID, "userA", orgModels.OrgRoleMember)

		svc := groupServices.NewGroupService(db)
		canManage, err := svc.CanUserManageGroup(groupGID, "userA")
		require.NoError(t, err, "CanUserManageGroup returned an unexpected error for plain org member")
		assert.False(t, canManage,
			"userA (plain org member) MUST NOT be allowed to manage GroupG")
	})
}

// TestGroupMember_WriteAuthz_OrgOwnerOfDifferentOrgCannotManage_False:
// owning a DIFFERENT org grants no write access to a group outside that org.
func TestGroupMember_WriteAuthz_OrgOwnerOfDifferentOrgCannotManage_False(t *testing.T) {
	withFreshRegistration(func() {
		db := setupGroupMemberVisibilityDB(t)
		_, orgOtherID, groupGID := seedFixture(t, db)

		// userA owns OrgOther — GroupG is in OrgOne. Cross-org isolation must hold.
		addOrgMember(t, db, orgOtherID, "userA", orgModels.OrgRoleOwner)

		svc := groupServices.NewGroupService(db)
		canManage, err := svc.CanUserManageGroup(groupGID, "userA")
		require.NoError(t, err, "CanUserManageGroup returned an unexpected error for owner-of-other-org")
		assert.False(t, canManage,
			"userA (owner of OrgOther) MUST NOT be allowed to manage GroupG (in OrgOne)")
	})
}

// TestGroupMember_WriteAuthz_DirectGroupOwnerCanManage_True: the user set as
// ClassGroup.OwnerUserID is allowed to manage. seedFixture sets OwnerUserID
// to "userTeacher" — this exercises the OwnerUserID short-circuit at the top
// of CanUserManageGroup.
func TestGroupMember_WriteAuthz_DirectGroupOwnerCanManage_True(t *testing.T) {
	withFreshRegistration(func() {
		db := setupGroupMemberVisibilityDB(t)
		_, _, groupGID := seedFixture(t, db)

		svc := groupServices.NewGroupService(db)
		canManage, err := svc.CanUserManageGroup(groupGID, "userTeacher")
		require.NoError(t, err, "CanUserManageGroup returned an unexpected error for direct group owner")
		assert.True(t, canManage,
			"userTeacher (OwnerUserID of GroupG) MUST be allowed to manage GroupG")
	})
}

// TestGroupMember_WriteAuthz_DirectGroupManagerCanManage_True: a user who is
// a direct group member with role "manager" can manage the group.
func TestGroupMember_WriteAuthz_DirectGroupManagerCanManage_True(t *testing.T) {
	withFreshRegistration(func() {
		db := setupGroupMemberVisibilityDB(t)
		_, _, groupGID := seedFixture(t, db)

		// Insert userM as a direct group manager
		addGroupMemberWithRole(t, db, groupGID, "userM", groupModels.GroupMemberRoleManager)

		svc := groupServices.NewGroupService(db)
		canManage, err := svc.CanUserManageGroup(groupGID, "userM")
		require.NoError(t, err, "CanUserManageGroup returned an unexpected error for direct group manager")
		assert.True(t, canManage,
			"userM (direct group manager) MUST be allowed to manage GroupG")
	})
}

// TestGroupMember_WriteAuthz_DirectGroupMemberCannotManage_False: a plain
// direct group member (role = "member") CANNOT manage the group.
func TestGroupMember_WriteAuthz_DirectGroupMemberCannotManage_False(t *testing.T) {
	withFreshRegistration(func() {
		db := setupGroupMemberVisibilityDB(t)
		_, _, groupGID := seedFixture(t, db)

		// userStudent1 is already inserted by seedFixture as a plain group member.
		svc := groupServices.NewGroupService(db)
		canManage, err := svc.CanUserManageGroup(groupGID, "userStudent1")
		require.NoError(t, err, "CanUserManageGroup returned an unexpected error for plain group member")
		assert.False(t, canManage,
			"userStudent1 (plain group member) MUST NOT be allowed to manage GroupG")
	})
}

// TestGroupMember_WriteAuthz_UnrelatedUserCannotManage_False: a user with
// no rows in organization_members or group_members CANNOT manage.
func TestGroupMember_WriteAuthz_UnrelatedUserCannotManage_False(t *testing.T) {
	withFreshRegistration(func() {
		db := setupGroupMemberVisibilityDB(t)
		_, _, groupGID := seedFixture(t, db)

		svc := groupServices.NewGroupService(db)
		canManage, err := svc.CanUserManageGroup(groupGID, "userOutsider")
		require.NoError(t, err, "CanUserManageGroup returned an unexpected error for unrelated user")
		assert.False(t, canManage,
			"userOutsider (no org / no group) MUST NOT be allowed to manage GroupG")
	})
}
