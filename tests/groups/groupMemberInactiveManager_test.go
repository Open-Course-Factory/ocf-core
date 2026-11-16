package groups_tests

// A deactivated group membership must grant nothing, like a deactivated
// organization membership: the organization cleanup stands a class roster down
// with is_active=false, and from then on a former class manager must no longer
// add, remove or re-rank members through the GroupMember hooks.
//
// Each case runs the real hook as a non-creator class manager, once with the
// row active (control) and once with it deactivated the way production does it
// (an UPDATE of the column, which the GORM bool-default pitfall cannot swallow).

import (
	"testing"
	"time"

	"soli/formations/src/auth/casdoor"
	authMocks "soli/formations/src/auth/mocks"
	"soli/formations/src/entityManagement/hooks"
	groupHooks "soli/formations/src/groups/hooks"
	groupModels "soli/formations/src/groups/models"
	groupServices "soli/formations/src/groups/services"
	scenarioHooks "soli/formations/src/scenarios/hooks"
	scenarioModels "soli/formations/src/scenarios/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const (
	inactiveCaseOwner   = "class-creator"
	inactiveCaseManager = "class-manager"
	inactiveCaseMember  = "class-member"
)

// seedManagedClass seeds a class whose creator is distinct from its manager,
// and deactivates the manager's row when managerActive is false.
func seedManagedClass(t *testing.T, managerActive bool) (*gorm.DB, uuid.UUID) {
	t.Helper()

	orig := casdoor.Enforcer
	casdoor.Enforcer = authMocks.NewMockEnforcer()
	t.Cleanup(func() { casdoor.Enforcer = orig })

	db := newGroupRoleCapDB(t)
	groupID := seedGroupRoleCap(t, db, inactiveCaseOwner, nil, map[string]groupModels.GroupMemberRole{
		inactiveCaseOwner:   groupModels.GroupMemberRoleOwner,
		inactiveCaseManager: groupModels.GroupMemberRoleManager,
		inactiveCaseMember:  groupModels.GroupMemberRoleMember,
	})
	if !managerActive {
		require.NoError(t, db.Model(&groupModels.GroupMember{}).
			Where("group_id = ? AND user_id = ?", groupID, inactiveCaseManager).
			Update("is_active", false).Error)
	}
	return db, groupID
}

func runAddMemberAsManager(t *testing.T, managerActive bool) error {
	db, groupID := seedManagedClass(t, managerActive)
	return groupHooks.NewGroupMemberValidationHook(db).Execute(&hooks.HookContext{
		EntityName: "GroupMember",
		HookType:   hooks.BeforeCreate,
		NewEntity: &groupModels.GroupMember{
			GroupID: groupID,
			UserID:  "newcomer",
			Role:    groupModels.GroupMemberRoleMember,
		},
		UserID:    inactiveCaseManager,
		UserRoles: platformMember,
	})
}

func runRemoveMemberAsManager(t *testing.T, managerActive bool) error {
	db, groupID := seedManagedClass(t, managerActive)
	return groupHooks.NewGroupMemberCleanupHook(db).Execute(&hooks.HookContext{
		EntityName: "GroupMember",
		HookType:   hooks.BeforeDelete,
		NewEntity: &groupModels.GroupMember{
			GroupID: groupID,
			UserID:  inactiveCaseMember,
			Role:    groupModels.GroupMemberRoleMember,
		},
		UserID:    inactiveCaseManager,
		UserRoles: platformMember,
	})
}

func runChangeRoleAsManager(t *testing.T, managerActive bool) error {
	db, groupID := seedManagedClass(t, managerActive)
	return groupHooks.NewGroupMemberRoleChangeHook(db).Execute(&hooks.HookContext{
		EntityName: "GroupMember",
		HookType:   hooks.BeforeUpdate,
		OldEntity: &groupModels.GroupMember{
			GroupID: groupID,
			UserID:  inactiveCaseMember,
			Role:    groupModels.GroupMemberRoleMember,
		},
		NewEntity: map[string]any{"role": groupModels.GroupMemberRoleManager},
		UserID:    inactiveCaseManager,
		UserRoles: platformMember,
	})
}

func TestGroupMemberHooks_ActiveManagerAddsMember_Allowed(t *testing.T) {
	require.NoError(t, runAddMemberAsManager(t, true))
}

func TestGroupMemberHooks_DeactivatedManagerAddsMember_Rejected(t *testing.T) {
	require.Error(t, runAddMemberAsManager(t, false),
		"a deactivated class manager must not add members to the class")
}

func TestGroupMemberHooks_ActiveManagerRemovesMember_Allowed(t *testing.T) {
	require.NoError(t, runRemoveMemberAsManager(t, true))
}

func TestGroupMemberHooks_DeactivatedManagerRemovesMember_Rejected(t *testing.T) {
	require.Error(t, runRemoveMemberAsManager(t, false),
		"a deactivated class manager must not remove members from the class")
}

func TestGroupMemberHooks_ActiveManagerChangesRole_Allowed(t *testing.T) {
	require.NoError(t, runChangeRoleAsManager(t, true))
}

func TestGroupMemberHooks_DeactivatedManagerChangesRole_Rejected(t *testing.T) {
	require.Error(t, runChangeRoleAsManager(t, false),
		"a deactivated class manager must not change roles in the class")
}

// The same rule reaches every caller of the membership lookup: editing the
// class itself and managing its scenario assignments.

func editClassAs(db *gorm.DB, groupID uuid.UUID, userID string) error {
	return groupHooks.NewGroupWriteAuthorizationHook(db).Execute(&hooks.HookContext{
		EntityName: "ClassGroup",
		HookType:   hooks.BeforeUpdate,
		EntityID:   groupID,
		UserID:     userID,
		UserRoles:  platformMember,
	})
}

func deactivateGroupRow(t *testing.T, db *gorm.DB, groupID uuid.UUID, userID string) {
	t.Helper()
	require.NoError(t, db.Model(&groupModels.GroupMember{}).
		Where("group_id = ? AND user_id = ?", groupID, userID).
		Update("is_active", false).Error)
}

func TestGroupWriteAuthorization_DeactivatedManagerEditsClass_Rejected(t *testing.T) {
	db, groupID := seedManagedClass(t, true)
	require.NoError(t, editClassAs(db, groupID, inactiveCaseManager), "control: an active manager edits the class")

	deactivateGroupRow(t, db, groupID, inactiveCaseManager)
	require.Error(t, editClassAs(db, groupID, inactiveCaseManager),
		"a deactivated class manager must not edit the class")
}

func TestScenarioAssignmentAuthorization_DeactivatedManager_Rejected(t *testing.T) {
	db, groupID := seedManagedClass(t, true)
	svc := groupServices.NewGroupService(db)
	assignment := &scenarioModels.ScenarioAssignment{GroupID: &groupID}

	ok, err := scenarioHooks.CanManageAssignment(db, svc, assignment, inactiveCaseManager)
	require.NoError(t, err)
	require.True(t, ok, "control: an active manager manages the class's assignments")

	deactivateGroupRow(t, db, groupID, inactiveCaseManager)
	ok, err = scenarioHooks.CanManageAssignment(db, svc, assignment, inactiveCaseManager)
	require.NoError(t, err)
	require.False(t, ok, "a deactivated class manager must not manage the class's assignments")
}

func TestGroupWriteAuthorization_CreatorWithDeactivatedRow_StillManages(t *testing.T) {
	db, groupID := seedManagedClass(t, true)
	deactivateGroupRow(t, db, groupID, inactiveCaseOwner)
	require.NoError(t, editClassAs(db, groupID, inactiveCaseOwner),
		"the class creator manages the class whatever their member row says")
}

func TestGroupWriteAuthorization_OrgManagerWithoutGroupRow_StillManages(t *testing.T) {
	orig := casdoor.Enforcer
	casdoor.Enforcer = authMocks.NewMockEnforcer()
	t.Cleanup(func() { casdoor.Enforcer = orig })

	db := newGroupRoleCapDB(t)
	orgID := seedOrgManager(t, db, "org-manager")
	groupID := seedGroupRoleCap(t, db, inactiveCaseOwner, &orgID, map[string]groupModels.GroupMemberRole{
		inactiveCaseOwner: groupModels.GroupMemberRoleOwner,
	})
	require.NoError(t, editClassAs(db, groupID, "org-manager"),
		"an organization manager manages the class through the organization")
}

// A re-added member holds an old deactivated row and a new active one; only
// the active row counts, whichever row the database returns first.
func TestGroupService_ReaddedMember_ActiveRowRoleWins(t *testing.T) {
	for _, tc := range []struct {
		name             string
		oldRole, newRole groupModels.GroupMemberRole
		wantManage       bool
	}{
		{"demoted on return", groupModels.GroupMemberRoleManager, groupModels.GroupMemberRoleMember, false},
		{"promoted on return", groupModels.GroupMemberRoleMember, groupModels.GroupMemberRoleManager, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := newGroupRoleCapDB(t)
			groupID := seedGroupRoleCap(t, db, inactiveCaseOwner, nil, map[string]groupModels.GroupMemberRole{
				"returning": tc.oldRole,
			})
			deactivateGroupRow(t, db, groupID, "returning")
			require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
				GroupID: groupID, UserID: "returning", Role: tc.newRole, JoinedAt: time.Now(), IsActive: true,
			}).Error)

			svc := groupServices.NewGroupService(db)
			inGroup, err := svc.IsUserInGroup(groupID, "returning")
			require.NoError(t, err)
			require.True(t, inGroup)
			role, err := svc.GetUserGroupRole(groupID, "returning")
			require.NoError(t, err)
			require.Equal(t, tc.newRole, role)
			canManage, err := svc.CanUserManageGroup(groupID, "returning")
			require.NoError(t, err)
			require.Equal(t, tc.wantManage, canManage)
		})
	}
}

// UpdateMemberRole has no HTTP caller (PATCH /group-members/:id goes through
// GroupMemberRoleChangeHook on the loaded row); a deactivated target reads as
// "not a member".
func TestGroupServiceUpdateMemberRole_DeactivatedTarget_NotAMember(t *testing.T) {
	db, groupID := seedManagedClass(t, true)
	deactivateGroupRow(t, db, groupID, inactiveCaseMember)
	err := groupServices.NewGroupService(db).UpdateMemberRole(groupID, inactiveCaseOwner, inactiveCaseMember, groupModels.GroupMemberRoleManager)
	require.ErrorContains(t, err, "not a member")
}

func TestGroupMemberValidation_ReaddDeactivatedMember_OneActiveRow(t *testing.T) {
	db, groupID := seedManagedClass(t, true)
	deactivateGroupRow(t, db, groupID, inactiveCaseMember)

	readd := func() error {
		return groupHooks.NewGroupMemberValidationHook(db).Execute(&hooks.HookContext{
			EntityName: "GroupMember",
			HookType:   hooks.BeforeCreate,
			NewEntity:  &groupModels.GroupMember{GroupID: groupID, UserID: inactiveCaseMember, Role: groupModels.GroupMemberRoleMember},
			UserID:     inactiveCaseOwner,
			UserRoles:  platformMember,
		})
	}
	require.NoError(t, readd(), "a deactivated member may be added back")
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: groupID, UserID: inactiveCaseMember, Role: groupModels.GroupMemberRoleMember, JoinedAt: time.Now(), IsActive: true,
	}).Error)
	require.Error(t, readd(), "once back, a second add is refused as already a member")

	var active int64
	require.NoError(t, db.Model(&groupModels.GroupMember{}).
		Where("group_id = ? AND user_id = ? AND is_active = ?", groupID, inactiveCaseMember, true).
		Count(&active).Error)
	require.EqualValues(t, 1, active)
}
