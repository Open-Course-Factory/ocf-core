package groups_tests

// Tests for #410 (audit-P, MR P): groupService.UpdateMemberRole must cap the role a
// granter can assign at the granter's own rank. Today the method checks that the granter
// can *manage* the group (CanUserManageGroup: owner, direct manager member, OR org
// owner/manager of the parent org), then updates the role WITHOUT capping it — so a
// manager could promote a member to owner.
//
// The rule to enforce, after the CanUserManageGroup gate and before the write:
//
//	granter role >= newRole   (access.IsRoleAtLeast, owner100 > manager50 > member10)
//
// Two wrinkles specific to groups:
//   - Owner short-circuit: when requestingUserID == group.OwnerUserID the cap is skipped.
//   - GetUserGroupRole ERRORS for the owner (no member row) and for an org-based manager
//     (who manages the group transitively via the parent org, not via a group_members
//     row). The fix defaults a failed lookup to "manager" so an org manager is capped at
//     manager (may set manager/member, may NOT mint an owner).
//
// These tests drive the real service (NewGroupService) against an in-memory DB and assert
// the returned error AND the persisted role read back from group_members.

import (
	"testing"
	"time"

	groupModels "soli/formations/src/groups/models"
	groupServices "soli/formations/src/groups/services"
	orgModels "soli/formations/src/organizations/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// newGroupRoleCapDB creates a fresh in-memory SQLite DB with the tables the group service
// touches for role updates: organizations + organization_members (for the org-based
// manager path) and class_groups + group_members.
func newGroupRoleCapDB(t *testing.T) *gorm.DB {
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
	require.NoError(t, err, "failed to auto-migrate org/group tables")

	return db
}

// seedGroupRoleCap creates a group owned by ownerUserID (optionally linked to orgID when
// non-nil) and inserts a group_members row for every (userID -> role) pair. Returns the
// group ID.
func seedGroupRoleCap(
	t *testing.T,
	db *gorm.DB,
	ownerUserID string,
	orgID *uuid.UUID,
	members map[string]groupModels.GroupMemberRole,
) uuid.UUID {
	t.Helper()

	groupID := uuid.New()
	group := &groupModels.ClassGroup{
		Name:           "RoleCapGroup",
		DisplayName:    "Role Cap Group",
		OwnerUserID:    ownerUserID,
		OrganizationID: orgID,
		MaxMembers:     50,
		IsActive:       true,
	}
	group.ID = groupID
	err := db.Omit("Metadata", "Members", "SubGroups", "ParentGroup").Create(group).Error
	require.NoError(t, err, "failed to create group")

	for userID, role := range members {
		m := &groupModels.GroupMember{
			GroupID:  groupID,
			UserID:   userID,
			Role:     role,
			JoinedAt: time.Now(),
			IsActive: true,
		}
		err := db.Omit("Metadata").Create(m).Error
		require.NoError(t, err, "failed to seed group member %s (role %s)", userID, role)
	}

	return groupID
}

// seedOrgManager inserts a team org owned by a distinct account plus an
// organization_members row making orgManagerID a manager of that org. Returns the org ID
// so the caller can link a group to it.
func seedOrgManager(t *testing.T, db *gorm.DB, orgManagerID string) uuid.UUID {
	t.Helper()

	orgID := uuid.New()
	org := &orgModels.Organization{
		Name:             "RoleCapOrg",
		DisplayName:      "Role Cap Org",
		OwnerUserID:      "org-owner-distinct",
		OrganizationType: orgModels.OrgTypeTeam,
		MaxGroups:        250,
		MaxMembers:       50,
		IsActive:         true,
	}
	org.ID = orgID
	err := db.Omit("Metadata", "OwnerIDs", "AllowedBackends", "Members", "Groups").Create(org).Error
	require.NoError(t, err, "failed to create org")

	m := &orgModels.OrganizationMember{
		OrganizationID: orgID,
		UserID:         orgManagerID,
		Role:           orgModels.OrgRoleManager,
		JoinedAt:       time.Now(),
		IsActive:       true,
	}
	err = db.Omit("Metadata").Create(m).Error
	require.NoError(t, err, "failed to seed org manager %s", orgManagerID)

	return orgID
}

// readGroupMemberRole reads the persisted role of a group member.
func readGroupMemberRole(t *testing.T, db *gorm.DB, groupID uuid.UUID, userID string) groupModels.GroupMemberRole {
	t.Helper()
	var m groupModels.GroupMember
	require.NoError(t,
		db.Where("group_id = ? AND user_id = ?", groupID, userID).First(&m).Error,
		"failed to read back group member %s", userID)
	return m.Role
}

// --- Direct group manager granter ---

// TestGroupServiceUpdateMemberRole_ManagerPromotesToOwner_Rejected: a direct group manager
// must NOT promote a member to owner. Expected RED today (no cap → update succeeds).
func TestGroupServiceUpdateMemberRole_ManagerPromotesToOwner_Rejected(t *testing.T) {
	const (
		ownerID   = "group-owner-account"
		granterID = "group-manager-granter"
		targetID  = "role-cap-target"
	)

	db := newGroupRoleCapDB(t)
	groupID := seedGroupRoleCap(t, db, ownerID, nil, map[string]groupModels.GroupMemberRole{
		ownerID:   groupModels.GroupMemberRoleOwner,
		granterID: groupModels.GroupMemberRoleManager,
		targetID:  groupModels.GroupMemberRoleMember,
	})

	svc := groupServices.NewGroupService(db)
	err := svc.UpdateMemberRole(groupID, granterID, targetID, groupModels.GroupMemberRoleOwner)

	require.Error(t, err,
		"a group manager must not promote a member to owner (owner100 > manager50); "+
			"UpdateMemberRole must reject, but it returned nil")
	require.Equal(t, groupModels.GroupMemberRoleMember, readGroupMemberRole(t, db, groupID, targetID),
		"the rejected promotion must NOT persist — target role must remain member")
}

// TestGroupServiceUpdateMemberRole_ManagerSetsRoleAtOrBelowOwn_Allowed: a direct group
// manager may set manager (equal) and member (below). Green guard.
func TestGroupServiceUpdateMemberRole_ManagerSetsRoleAtOrBelowOwn_Allowed(t *testing.T) {
	const (
		ownerID   = "group-owner-account"
		granterID = "group-manager-granter"
		targetID  = "role-cap-target"
	)

	t.Run("manager_to_manager", func(t *testing.T) {
		db := newGroupRoleCapDB(t)
		groupID := seedGroupRoleCap(t, db, ownerID, nil, map[string]groupModels.GroupMemberRole{
			ownerID:   groupModels.GroupMemberRoleOwner,
			granterID: groupModels.GroupMemberRoleManager,
			targetID:  groupModels.GroupMemberRoleMember,
		})
		svc := groupServices.NewGroupService(db)
		err := svc.UpdateMemberRole(groupID, granterID, targetID, groupModels.GroupMemberRoleManager)
		require.NoError(t, err, "a manager assigning manager must be allowed (manager50 >= manager50)")
		require.Equal(t, groupModels.GroupMemberRoleManager, readGroupMemberRole(t, db, groupID, targetID),
			"the allowed update must persist — target role must be manager")
	})

	t.Run("manager_to_member", func(t *testing.T) {
		db := newGroupRoleCapDB(t)
		groupID := seedGroupRoleCap(t, db, ownerID, nil, map[string]groupModels.GroupMemberRole{
			ownerID:   groupModels.GroupMemberRoleOwner,
			granterID: groupModels.GroupMemberRoleManager,
			targetID:  groupModels.GroupMemberRoleManager,
		})
		svc := groupServices.NewGroupService(db)
		err := svc.UpdateMemberRole(groupID, granterID, targetID, groupModels.GroupMemberRoleMember)
		require.NoError(t, err, "a manager assigning member must be allowed (manager50 >= member10)")
		require.Equal(t, groupModels.GroupMemberRoleMember, readGroupMemberRole(t, db, groupID, targetID),
			"the allowed update must persist — target role must be member")
	})
}

// TestGroupServiceUpdateMemberRole_OwnerPromotesToOwner_Allowed: the group owner
// (requestingUserID == group.OwnerUserID) may promote a member to owner via the owner
// short-circuit. Green guard.
func TestGroupServiceUpdateMemberRole_OwnerPromotesToOwner_Allowed(t *testing.T) {
	const (
		ownerID  = "group-owner-account"
		targetID = "role-cap-target"
	)

	db := newGroupRoleCapDB(t)
	groupID := seedGroupRoleCap(t, db, ownerID, nil, map[string]groupModels.GroupMemberRole{
		ownerID:  groupModels.GroupMemberRoleOwner,
		targetID: groupModels.GroupMemberRoleMember,
	})

	svc := groupServices.NewGroupService(db)
	err := svc.UpdateMemberRole(groupID, ownerID, targetID, groupModels.GroupMemberRoleOwner)

	require.NoError(t, err,
		"the group owner must be able to promote a member to owner (owner short-circuit)")
	require.Equal(t, groupModels.GroupMemberRoleOwner, readGroupMemberRole(t, db, groupID, targetID),
		"the owner-driven promotion must persist — target role must be owner")
}

// --- Org-based manager granter (exercises the failed-lookup fallback to "manager") ---

// TestGroupServiceUpdateMemberRole_OrgBasedManagerPromotesToOwner_Rejected: the granter is
// a MANAGER of the parent organization but NOT a direct group member. CanUserManageGroup
// grants access via the org path; GetUserGroupRole errors (no group_members row), so the
// fix must fall back to "manager" and reject the owner promotion. Expected RED today.
func TestGroupServiceUpdateMemberRole_OrgBasedManagerPromotesToOwner_Rejected(t *testing.T) {
	const (
		ownerID     = "group-owner-account"
		orgManager  = "org-manager-granter"
		targetID    = "role-cap-target"
	)

	db := newGroupRoleCapDB(t)
	orgID := seedOrgManager(t, db, orgManager)
	groupID := seedGroupRoleCap(t, db, ownerID, &orgID, map[string]groupModels.GroupMemberRole{
		ownerID:  groupModels.GroupMemberRoleOwner,
		targetID: groupModels.GroupMemberRoleMember,
	})

	svc := groupServices.NewGroupService(db)
	err := svc.UpdateMemberRole(groupID, orgManager, targetID, groupModels.GroupMemberRoleOwner)

	require.Error(t, err,
		"an org-based manager (no group_members row → role lookup falls back to manager) "+
			"must not promote a group member to owner; UpdateMemberRole must reject")
	require.Equal(t, groupModels.GroupMemberRoleMember, readGroupMemberRole(t, db, groupID, targetID),
		"the rejected promotion must NOT persist — target role must remain member")
}

// TestGroupServiceUpdateMemberRole_OrgBasedManagerPromotesToManager_Allowed: same org-based
// manager, but assigning manager (fallback manager >= manager) must be allowed. Green guard
// on the fallback path.
func TestGroupServiceUpdateMemberRole_OrgBasedManagerPromotesToManager_Allowed(t *testing.T) {
	const (
		ownerID    = "group-owner-account"
		orgManager = "org-manager-granter"
		targetID   = "role-cap-target"
	)

	db := newGroupRoleCapDB(t)
	orgID := seedOrgManager(t, db, orgManager)
	groupID := seedGroupRoleCap(t, db, ownerID, &orgID, map[string]groupModels.GroupMemberRole{
		ownerID:  groupModels.GroupMemberRoleOwner,
		targetID: groupModels.GroupMemberRoleMember,
	})

	svc := groupServices.NewGroupService(db)
	err := svc.UpdateMemberRole(groupID, orgManager, targetID, groupModels.GroupMemberRoleManager)

	require.NoError(t, err,
		"an org-based manager assigning manager must be allowed (fallback manager50 >= manager50)")
	require.Equal(t, groupModels.GroupMemberRoleManager, readGroupMemberRole(t, db, groupID, targetID),
		"the allowed update must persist — target role must be manager")
}
