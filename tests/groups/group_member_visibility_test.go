package groups_tests

// Tests for issue #288: org owners/managers must see members of groups within
// their organization, even when they are not direct members of the group.
//
// The bug: GroupMember is registered with OrgAccessEnabled: false, so the
// generic membership filter only allows access via direct group membership
// (a row in group_members for the requesting user). Org owners/managers are
// therefore blocked from seeing members of groups they manage transitively
// via the parent ClassGroup's organization_id.
//
// These tests bypass the HTTP layer and call the generic repository directly
// so any failure points squarely at the SQL filter logic.
//
// IMPORTANT — assumption about the future fix:
// The fix will require a configuration change so that GroupMember's
// MembershipConfig declares OrgAccessEnabled: true and provides the path
// from group_members to the parent organization (via class_groups.organization_id).
// The current MembershipConfig shape only exposes OrgAccessEnabled + ManagerRoles
// and assumes the entity table itself has an organization_id column — which is
// NOT the case for group_members. Tests 1, 2, and 6 register the membership
// config with the future-shaped flags (OrgAccessEnabled: true, ManagerRoles:
// ["owner", "manager"]). On the current code these tests are EXPECTED TO FAIL
// because:
//   - Either the SQL fails (no such column: group_members.organization_id), OR
//   - The filter does not produce the org-access branch and returns 0 rows.
// Once backend-dev introduces the new config shape and updates the filter,
// these tests must be updated to use the new field names. Document any change
// here in the verify step.

import (
	"reflect"
	"testing"
	"time"

	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/entityManagement/repositories"
	groupModels "soli/formations/src/groups/models"
	orgModels "soli/formations/src/organizations/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupGroupMemberVisibilityDB creates a fresh in-memory SQLite DB with the
// tables needed to exercise the GroupMember membership filter, including the
// parent class_groups table (so the filter can resolve the org via the parent)
// and organization_members (so the filter can match the org-level role).
func setupGroupMemberVisibilityDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err, "failed to open in-memory SQLite DB")

	err = db.AutoMigrate(
		&orgModels.Organization{},
		&orgModels.OrganizationMember{},
		&groupModels.ClassGroup{},
		&groupModels.GroupMember{},
	)
	require.NoError(t, err, "failed to auto-migrate Organization/OrganizationMember/ClassGroup/GroupMember")

	return db
}

// registerGroupMemberMembershipConfig registers the MembershipConfig for
// "GroupMember" with the future-shaped flags (OrgAccessEnabled: true,
// ManagerRoles: ["owner", "manager"]).
//
// The current production code registers OrgAccessEnabled: false, which is
// precisely the bug. The tests register the future config to drive the fix.
func registerGroupMemberMembershipConfig() {
	ems.GlobalEntityRegistrationService.RegisterMembershipConfig("GroupMember",
		&entityManagementInterfaces.MembershipConfig{
			MemberTable:      "group_members",
			EntityIDColumn:   "group_id",
			UserIDColumn:     "user_id",
			RoleColumn:       "role",
			IsActiveColumn:   "is_active",
			OrgAccessEnabled: true,
			ManagerRoles:     []string{"owner", "manager"},
		},
	)
}

// seedFixture creates two organizations (OrgOne and OrgOther), a ClassGroup
// in OrgOne owned by userTeacher, and two student members of that group.
//
// Returns OrgOne ID, OrgOther ID, and the GroupG ID.
func seedFixture(t *testing.T, db *gorm.DB) (orgOneID, orgOtherID, groupGID uuid.UUID) {
	t.Helper()

	orgOneID = uuid.New()
	orgOtherID = uuid.New()
	groupGID = uuid.New()

	// OrgOne
	orgOne := &orgModels.Organization{
		Name:             "OrgOne",
		DisplayName:      "Org One",
		OwnerUserID:      "userTeacher",
		OrganizationType: orgModels.OrgTypeTeam,
		MaxGroups:        250,
		MaxMembers:       50,
		IsActive:         true,
	}
	orgOne.ID = orgOneID
	err := db.Omit("Metadata", "AllowedBackends", "Members", "Groups").Create(orgOne).Error
	require.NoError(t, err, "failed to create OrgOne")

	// OrgOther
	orgOther := &orgModels.Organization{
		Name:             "OrgOther",
		DisplayName:      "Org Other",
		OwnerUserID:      "userOtherTeacher",
		OrganizationType: orgModels.OrgTypeTeam,
		MaxGroups:        250,
		MaxMembers:       50,
		IsActive:         true,
	}
	orgOther.ID = orgOtherID
	err = db.Omit("Metadata", "AllowedBackends", "Members", "Groups").Create(orgOther).Error
	require.NoError(t, err, "failed to create OrgOther")

	// GroupG belongs to OrgOne, owned by userTeacher (NOT userA)
	group := &groupModels.ClassGroup{
		Name:           "GroupG",
		DisplayName:    "Group G",
		OwnerUserID:    "userTeacher",
		OrganizationID: &orgOneID,
		MaxMembers:     50,
		IsActive:       true,
	}
	group.ID = groupGID
	err = db.Omit("Metadata", "Members", "SubGroups", "ParentGroup").Create(group).Error
	require.NoError(t, err, "failed to create GroupG")

	// Two students as direct members of GroupG
	student1 := &groupModels.GroupMember{
		GroupID:  groupGID,
		UserID:   "userStudent1",
		Role:     groupModels.GroupMemberRoleMember,
		JoinedAt: time.Now(),
		IsActive: true,
	}
	err = db.Omit("Metadata").Create(student1).Error
	require.NoError(t, err, "failed to insert userStudent1 as group member")

	student2 := &groupModels.GroupMember{
		GroupID:  groupGID,
		UserID:   "userStudent2",
		Role:     groupModels.GroupMemberRoleMember,
		JoinedAt: time.Now(),
		IsActive: true,
	}
	err = db.Omit("Metadata").Create(student2).Error
	require.NoError(t, err, "failed to insert userStudent2 as group member")

	return orgOneID, orgOtherID, groupGID
}

// addOrgMember inserts a row in organization_members with the given role.
func addOrgMember(t *testing.T, db *gorm.DB, orgID uuid.UUID, userID string, role orgModels.OrganizationMemberRole) {
	t.Helper()
	m := &orgModels.OrganizationMember{
		OrganizationID: orgID,
		UserID:         userID,
		Role:           role,
		JoinedAt:       time.Now(),
		IsActive:       true,
	}
	err := db.Omit("Metadata").Create(m).Error
	require.NoError(t, err, "failed to insert org member %s with role %s", userID, role)
}

// extractGroupMembers extracts the slice of models.GroupMember from the []any
// result returned by GetAllEntities.
func extractGroupMembers(t *testing.T, results []any) []groupModels.GroupMember {
	t.Helper()

	if len(results) == 0 {
		return nil
	}

	raw := results[0]
	rv := reflect.ValueOf(raw)

	var members []groupModels.GroupMember
	for i := 0; i < rv.Len(); i++ {
		elem := rv.Index(i).Interface()
		gm, ok := elem.(groupModels.GroupMember)
		require.True(t, ok, "expected models.GroupMember in result slice, got %T", elem)
		members = append(members, gm)
	}
	return members
}

// withFreshRegistration runs fn against a fresh, isolated GlobalEntityRegistrationService
// and restores the original at the end. Critical to avoid cross-test contamination.
func withFreshRegistration(fn func()) {
	original := ems.GlobalEntityRegistrationService
	ems.GlobalEntityRegistrationService = ems.NewEntityRegistrationService()
	defer func() { ems.GlobalEntityRegistrationService = original }()
	fn()
}

// --- Tests ---

// TestGroupMember_Visibility_OrgOwnerSeesGroupMembers asserts that a user who
// is the OWNER of the parent organization sees the members of a group that
// belongs to that organization, even when they are not a direct member of the
// group.
//
// EXPECTED TO FAIL on current code: GroupMember registers
// OrgAccessEnabled: false, and even with OrgAccessEnabled: true the current
// generic filter references group_members.organization_id which does not
// exist (the org id lives on the parent class_groups row).
func TestGroupMember_Visibility_OrgOwnerSeesGroupMembers(t *testing.T) {
	withFreshRegistration(func() {
		db := setupGroupMemberVisibilityDB(t)
		orgOneID, _, _ := seedFixture(t, db)

		// userA is OWNER of OrgOne, but NOT a direct member of GroupG
		addOrgMember(t, db, orgOneID, "userA", orgModels.OrgRoleOwner)

		registerGroupMemberMembershipConfig()
		repo := repositories.NewGenericRepository(db)

		results, total, err := repo.GetAllEntities(
			groupModels.GroupMember{},
			1, 100,
			map[string]any{"user_member_id": "userA"},
			nil,
		)
		require.NoError(t, err, "GetAllEntities returned an unexpected error for userA (org owner)")
		assert.Equal(t, int64(2), total,
			"userA (org owner) must see 2 group members of GroupG (userStudent1, userStudent2), got %d", total)

		members := extractGroupMembers(t, results)
		assert.Len(t, members, 2,
			"userA should see exactly 2 group members of GroupG, got %d", len(members))
	})
}

// TestGroupMember_Visibility_OrgManagerSeesGroupMembers asserts that a user
// who has the MANAGER role in the parent organization sees the members of a
// group within that organization.
//
// EXPECTED TO FAIL on current code (same reason as the owner test).
func TestGroupMember_Visibility_OrgManagerSeesGroupMembers(t *testing.T) {
	withFreshRegistration(func() {
		db := setupGroupMemberVisibilityDB(t)
		orgOneID, _, _ := seedFixture(t, db)

		// userA is MANAGER of OrgOne, NOT a direct member of GroupG
		addOrgMember(t, db, orgOneID, "userA", orgModels.OrgRoleManager)

		registerGroupMemberMembershipConfig()
		repo := repositories.NewGenericRepository(db)

		results, total, err := repo.GetAllEntities(
			groupModels.GroupMember{},
			1, 100,
			map[string]any{"user_member_id": "userA"},
			nil,
		)
		require.NoError(t, err, "GetAllEntities returned an unexpected error for userA (org manager)")
		assert.Equal(t, int64(2), total,
			"userA (org manager) must see 2 group members of GroupG, got %d", total)

		members := extractGroupMembers(t, results)
		assert.Len(t, members, 2,
			"userA (org manager) should see exactly 2 group members, got %d", len(members))
	})
}

// TestGroupMember_Visibility_RegularOrgMemberDoesNotSeeGroupMembers asserts
// that a plain "member" (not manager/owner) of the parent org does NOT get
// access to group members purely by virtue of org membership.
//
// This currently passes for the wrong reason (orgAccess off entirely). After
// the fix, it must STILL pass because plain member is not in ManagerRoles.
func TestGroupMember_Visibility_RegularOrgMemberDoesNotSeeGroupMembers(t *testing.T) {
	withFreshRegistration(func() {
		db := setupGroupMemberVisibilityDB(t)
		orgOneID, _, _ := seedFixture(t, db)

		// userA is a plain MEMBER of OrgOne (not owner/manager), not a direct group member
		addOrgMember(t, db, orgOneID, "userA", orgModels.OrgRoleMember)

		registerGroupMemberMembershipConfig()
		repo := repositories.NewGenericRepository(db)

		results, total, err := repo.GetAllEntities(
			groupModels.GroupMember{},
			1, 100,
			map[string]any{"user_member_id": "userA"},
			nil,
		)
		require.NoError(t, err, "GetAllEntities returned an unexpected error for userA (plain org member)")
		assert.Equal(t, int64(0), total,
			"userA (plain org member) must NOT see group members, got total=%d", total)

		members := extractGroupMembers(t, results)
		assert.Empty(t, members,
			"userA (plain org member) should see no group members, got %d", len(members))
	})
}

// TestGroupMember_Visibility_DirectGroupMemberSeesPeers asserts that a direct
// member of a group continues to see their peers in that group. Regression
// guard: the fix must not break the existing direct-membership path.
func TestGroupMember_Visibility_DirectGroupMemberSeesPeers(t *testing.T) {
	withFreshRegistration(func() {
		db := setupGroupMemberVisibilityDB(t)
		seedFixture(t, db)

		// userStudent1 is already a direct member of GroupG (added by seedFixture)
		registerGroupMemberMembershipConfig()
		repo := repositories.NewGenericRepository(db)

		results, total, err := repo.GetAllEntities(
			groupModels.GroupMember{},
			1, 100,
			map[string]any{"user_member_id": "userStudent1"},
			nil,
		)
		require.NoError(t, err, "GetAllEntities returned an unexpected error for userStudent1")
		assert.Equal(t, int64(2), total,
			"userStudent1 (direct group member) must see both peers in GroupG, got %d", total)

		members := extractGroupMembers(t, results)
		assert.Len(t, members, 2,
			"userStudent1 should see 2 group members of GroupG, got %d", len(members))
	})
}

// TestGroupMember_Visibility_UnrelatedUserSeesNothing asserts that a user
// with no rows in organization_members or group_members sees nothing.
func TestGroupMember_Visibility_UnrelatedUserSeesNothing(t *testing.T) {
	withFreshRegistration(func() {
		db := setupGroupMemberVisibilityDB(t)
		seedFixture(t, db)

		// userOutsider is in NO organization and NO group
		registerGroupMemberMembershipConfig()
		repo := repositories.NewGenericRepository(db)

		results, total, err := repo.GetAllEntities(
			groupModels.GroupMember{},
			1, 100,
			map[string]any{"user_member_id": "userOutsider"},
			nil,
		)
		require.NoError(t, err, "GetAllEntities returned an unexpected error for userOutsider")
		assert.Equal(t, int64(0), total,
			"userOutsider must see no group members, got total=%d", total)

		members := extractGroupMembers(t, results)
		assert.Empty(t, members,
			"userOutsider should see no group members, got %d", len(members))
	})
}

// TestGroupMember_Visibility_OrgOwnerOfDifferentOrgSeesNothing asserts that
// an org owner only sees group members of groups within THEIR organization,
// not other organizations' groups.
//
// EXPECTED TO FAIL today: same SQL/config issue as tests 1 and 2. Once the
// fix lands, this confirms the org filter is correctly scoped.
func TestGroupMember_Visibility_OrgOwnerOfDifferentOrgSeesNothing(t *testing.T) {
	withFreshRegistration(func() {
		db := setupGroupMemberVisibilityDB(t)
		_, orgOtherID, _ := seedFixture(t, db)

		// userA owns OrgOther (the OTHER org). GroupG belongs to OrgOne — userA must NOT see its members.
		addOrgMember(t, db, orgOtherID, "userA", orgModels.OrgRoleOwner)

		registerGroupMemberMembershipConfig()
		repo := repositories.NewGenericRepository(db)

		results, total, err := repo.GetAllEntities(
			groupModels.GroupMember{},
			1, 100,
			map[string]any{"user_member_id": "userA"},
			nil,
		)
		require.NoError(t, err, "GetAllEntities returned an unexpected error for userA (owner of unrelated org)")
		assert.Equal(t, int64(0), total,
			"userA (owner of OrgOther) must NOT see group members of OrgOne's GroupG, got total=%d", total)

		members := extractGroupMembers(t, results)
		assert.Empty(t, members,
			"userA (owner of unrelated org) should see no group members, got %d", len(members))
	})
}
