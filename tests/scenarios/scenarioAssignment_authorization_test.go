package scenarios_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/entityManagement/hooks"
	entityManagementModels "soli/formations/src/entityManagement/models"
	groupModels "soli/formations/src/groups/models"
	orgModels "soli/formations/src/organizations/models"
	scenarioHooks "soli/formations/src/scenarios/hooks"
	"soli/formations/src/scenarios/models"
)

// ============================================================================
// BeforeUpdate Tests — Group-Scoped Assignments
// ============================================================================

func TestScenarioAssignmentAuth_BeforeUpdate_GroupManagerCanUpdate(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.AutoMigrate(&orgModels.Organization{}, &orgModels.OrganizationMember{}))

	hook := scenarioHooks.NewScenarioAssignmentAuthorizationHook(db)

	ownerID := "group-owner-update-001"

	// Create group with owner
	groupID, err := uuid.NewV7()
	require.NoError(t, err)
	group := &groupModels.ClassGroup{
		Name:        "Update Test Group",
		DisplayName: "Update Test Group",
		OwnerUserID: ownerID,
		MaxMembers:  50,
		IsActive:    true,
	}
	group.ID = groupID
	require.NoError(t, db.Omit("Metadata").Create(group).Error)

	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID:   groupID,
		UserID:    ownerID,
		Role:      groupModels.GroupMemberRoleOwner,
		InvitedBy: ownerID,
		JoinedAt:  time.Now(),
		IsActive:  true,
	}).Error)

	// Create scenario and assignment
	scenario := &models.Scenario{
		Name: "update-auth-test", Title: "Update Auth Test",
		InstanceType: "ubuntu:22.04", CreatedByID: ownerID,
	}
	require.NoError(t, db.Create(scenario).Error)

	assignment := &models.ScenarioAssignment{
		BaseModel:   entityManagementModels.BaseModel{ID: uuid.New()},
		ScenarioID:  scenario.ID,
		GroupID:      &groupID,
		Scope:       "group",
		CreatedByID: ownerID,
		IsActive:    true,
	}
	require.NoError(t, db.Create(assignment).Error)

	ctx := &hooks.HookContext{
		EntityName: "ScenarioAssignment",
		HookType:   hooks.BeforeUpdate,
		EntityID:   assignment.ID,
		OldEntity:  assignment,
		NewEntity:  map[string]any{"is_active": false},
		UserID:     ownerID,
		UserRoles:  []string{"Member"},
	}

	err = hook.Execute(ctx)
	assert.NoError(t, err, "Group manager (owner) should be able to update assignment")
}

func TestScenarioAssignmentAuth_BeforeUpdate_NonManagerBlocked(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.AutoMigrate(&orgModels.Organization{}, &orgModels.OrganizationMember{}))

	hook := scenarioHooks.NewScenarioAssignmentAuthorizationHook(db)

	ownerID := "group-owner-update-002"
	memberID := "group-member-update-002"

	// Create group with owner and regular member
	groupID, err := uuid.NewV7()
	require.NoError(t, err)
	group := &groupModels.ClassGroup{
		Name: "Update Block Test Group", DisplayName: "Update Block Test Group",
		OwnerUserID: ownerID, MaxMembers: 50, IsActive: true,
	}
	group.ID = groupID
	require.NoError(t, db.Omit("Metadata").Create(group).Error)

	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: groupID, UserID: ownerID, Role: groupModels.GroupMemberRoleOwner,
		InvitedBy: ownerID, JoinedAt: time.Now(), IsActive: true,
	}).Error)

	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: groupID, UserID: memberID, Role: groupModels.GroupMemberRoleMember,
		InvitedBy: ownerID, JoinedAt: time.Now(), IsActive: true,
	}).Error)

	// Create scenario and assignment
	scenario := &models.Scenario{
		Name: "update-block-test", Title: "Update Block Test",
		InstanceType: "ubuntu:22.04", CreatedByID: ownerID,
	}
	require.NoError(t, db.Create(scenario).Error)

	assignment := &models.ScenarioAssignment{
		BaseModel:   entityManagementModels.BaseModel{ID: uuid.New()},
		ScenarioID:  scenario.ID,
		GroupID:      &groupID,
		Scope:       "group",
		CreatedByID: ownerID,
		IsActive:    true,
	}
	require.NoError(t, db.Create(assignment).Error)

	ctx := &hooks.HookContext{
		EntityName: "ScenarioAssignment",
		HookType:   hooks.BeforeUpdate,
		EntityID:   assignment.ID,
		OldEntity:  assignment,
		NewEntity:  map[string]any{"is_active": false},
		UserID:     memberID,
		UserRoles:  []string{"Member"},
	}

	err = hook.Execute(ctx)
	assert.Error(t, err, "Non-manager should be blocked from updating assignment")
	assert.Contains(t, err.Error(), "permission", "Error should mention permission denial")
}

func TestScenarioAssignmentAuth_BeforeUpdate_AdminCanUpdate(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.AutoMigrate(&orgModels.Organization{}, &orgModels.OrganizationMember{}))

	hook := scenarioHooks.NewScenarioAssignmentAuthorizationHook(db)

	ownerID := "group-owner-update-003"
	adminID := "platform-admin-update-003"

	// Create group with owner
	groupID, err := uuid.NewV7()
	require.NoError(t, err)
	group := &groupModels.ClassGroup{
		Name: "Admin Update Test Group", DisplayName: "Admin Update Test Group",
		OwnerUserID: ownerID, MaxMembers: 50, IsActive: true,
	}
	group.ID = groupID
	require.NoError(t, db.Omit("Metadata").Create(group).Error)

	// Create scenario and assignment
	scenario := &models.Scenario{
		Name: "admin-update-test", Title: "Admin Update Test",
		InstanceType: "ubuntu:22.04", CreatedByID: ownerID,
	}
	require.NoError(t, db.Create(scenario).Error)

	assignment := &models.ScenarioAssignment{
		BaseModel:   entityManagementModels.BaseModel{ID: uuid.New()},
		ScenarioID:  scenario.ID,
		GroupID:      &groupID,
		Scope:       "group",
		CreatedByID: ownerID,
		IsActive:    true,
	}
	require.NoError(t, db.Create(assignment).Error)

	ctx := &hooks.HookContext{
		EntityName: "ScenarioAssignment",
		HookType:   hooks.BeforeUpdate,
		EntityID:   assignment.ID,
		OldEntity:  assignment,
		NewEntity:  map[string]any{"is_active": false},
		UserID:     adminID,
		UserRoles:  []string{"Administrator"},
	}

	err = hook.Execute(ctx)
	assert.NoError(t, err, "Platform admin should bypass authorization and update any assignment")
}

// ============================================================================
// Organization-Level Authorization Tests — BeforeCreate
// ============================================================================

func TestScenarioAssignmentAuth_BeforeCreate_OrgManagerCanCreate(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.AutoMigrate(&orgModels.Organization{}, &orgModels.OrganizationMember{}))

	hook := scenarioHooks.NewScenarioAssignmentAuthorizationHook(db)

	orgOwnerID := "org-owner-create-001"

	// Create organization with owner membership
	orgID, err := uuid.NewV7()
	require.NoError(t, err)
	org := &orgModels.Organization{
		Name: "Create Auth Test Org", DisplayName: "Create Auth Test Org",
		OwnerUserID: orgOwnerID, OrganizationType: orgModels.OrgTypeTeam,
		MaxMembers: 100, IsActive: true,
	}
	org.ID = orgID
	require.NoError(t, db.Omit("Metadata").Create(org).Error)

	require.NoError(t, db.Omit("Metadata").Create(&orgModels.OrganizationMember{
		OrganizationID: orgID, UserID: orgOwnerID, Role: orgModels.OrgRoleOwner,
		JoinedAt: time.Now(), IsActive: true,
	}).Error)

	// Create scenario
	scenario := &models.Scenario{
		Name: "org-create-test", Title: "Org Create Test",
		InstanceType: "ubuntu:22.04", CreatedByID: orgOwnerID,
	}
	require.NoError(t, db.Create(scenario).Error)

	// Org owner should be able to create org-scoped assignment
	assignment := &models.ScenarioAssignment{
		ScenarioID:     scenario.ID,
		OrganizationID: &orgID,
		Scope:          "org",
		IsActive:       true,
	}

	ctx := &hooks.HookContext{
		EntityName: "ScenarioAssignment",
		HookType:   hooks.BeforeCreate,
		NewEntity:  assignment,
		UserID:     orgOwnerID,
		UserRoles:  []string{"Member"},
	}

	err = hook.Execute(ctx)
	assert.NoError(t, err, "Org owner should be able to create org-scoped assignment")
	assert.Equal(t, orgOwnerID, assignment.CreatedByID, "CreatedByID should be set from authenticated user")
}

func TestScenarioAssignmentAuth_BeforeCreate_NonOrgManagerBlocked(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.AutoMigrate(&orgModels.Organization{}, &orgModels.OrganizationMember{}))

	hook := scenarioHooks.NewScenarioAssignmentAuthorizationHook(db)

	orgOwnerID := "org-owner-create-002"
	orgMemberID := "org-member-create-002"

	// Create organization with owner and regular member
	orgID, err := uuid.NewV7()
	require.NoError(t, err)
	org := &orgModels.Organization{
		Name: "Create Block Test Org", DisplayName: "Create Block Test Org",
		OwnerUserID: orgOwnerID, OrganizationType: orgModels.OrgTypeTeam,
		MaxMembers: 100, IsActive: true,
	}
	org.ID = orgID
	require.NoError(t, db.Omit("Metadata").Create(org).Error)

	require.NoError(t, db.Omit("Metadata").Create(&orgModels.OrganizationMember{
		OrganizationID: orgID, UserID: orgOwnerID, Role: orgModels.OrgRoleOwner,
		JoinedAt: time.Now(), IsActive: true,
	}).Error)

	require.NoError(t, db.Omit("Metadata").Create(&orgModels.OrganizationMember{
		OrganizationID: orgID, UserID: orgMemberID, Role: orgModels.OrgRoleMember,
		JoinedAt: time.Now(), IsActive: true,
	}).Error)

	// Create scenario
	scenario := &models.Scenario{
		Name: "org-create-block-test", Title: "Org Create Block Test",
		InstanceType: "ubuntu:22.04", CreatedByID: orgOwnerID,
	}
	require.NoError(t, db.Create(scenario).Error)

	// Regular org member should be blocked from creating org-scoped assignment
	assignment := &models.ScenarioAssignment{
		ScenarioID:     scenario.ID,
		OrganizationID: &orgID,
		Scope:          "org",
		IsActive:       true,
	}

	ctx := &hooks.HookContext{
		EntityName: "ScenarioAssignment",
		HookType:   hooks.BeforeCreate,
		NewEntity:  assignment,
		UserID:     orgMemberID,
		UserRoles:  []string{"Member"},
	}

	err = hook.Execute(ctx)
	assert.Error(t, err, "Non-manager org member should be blocked from creating org-scoped assignment")
	assert.Contains(t, err.Error(), "permission", "Error should mention permission denial")
}

// ============================================================================
// Organization-Level Authorization Tests — BeforeDelete
// ============================================================================

func TestScenarioAssignmentAuth_BeforeDelete_OrgManagerCanDelete(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.AutoMigrate(&orgModels.Organization{}, &orgModels.OrganizationMember{}))

	hook := scenarioHooks.NewScenarioAssignmentAuthorizationHook(db)

	orgOwnerID := "org-owner-delete-001"

	// Create organization with owner
	orgID, err := uuid.NewV7()
	require.NoError(t, err)
	org := &orgModels.Organization{
		Name: "Delete Auth Test Org", DisplayName: "Delete Auth Test Org",
		OwnerUserID: orgOwnerID, OrganizationType: orgModels.OrgTypeTeam,
		MaxMembers: 100, IsActive: true,
	}
	org.ID = orgID
	require.NoError(t, db.Omit("Metadata").Create(org).Error)

	require.NoError(t, db.Omit("Metadata").Create(&orgModels.OrganizationMember{
		OrganizationID: orgID, UserID: orgOwnerID, Role: orgModels.OrgRoleOwner,
		JoinedAt: time.Now(), IsActive: true,
	}).Error)

	// Create scenario and assignment
	scenario := &models.Scenario{
		Name: "org-delete-test", Title: "Org Delete Test",
		InstanceType: "ubuntu:22.04", CreatedByID: orgOwnerID,
	}
	require.NoError(t, db.Create(scenario).Error)

	assignment := &models.ScenarioAssignment{
		BaseModel:      entityManagementModels.BaseModel{ID: uuid.New()},
		ScenarioID:     scenario.ID,
		OrganizationID: &orgID,
		Scope:          "org",
		CreatedByID:    orgOwnerID,
		IsActive:       true,
	}
	require.NoError(t, db.Create(assignment).Error)

	// Note: in DeleteEntityWithUser, the loaded entity goes into NewEntity
	ctx := &hooks.HookContext{
		EntityName: "ScenarioAssignment",
		HookType:   hooks.BeforeDelete,
		EntityID:   assignment.ID,
		NewEntity:  assignment,
		UserID:     orgOwnerID,
		UserRoles:  []string{"Member"},
	}

	err = hook.Execute(ctx)
	assert.NoError(t, err, "Org owner should be able to delete org-scoped assignment")
}

func TestScenarioAssignmentAuth_BeforeDelete_NonOrgManagerBlocked(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.AutoMigrate(&orgModels.Organization{}, &orgModels.OrganizationMember{}))

	hook := scenarioHooks.NewScenarioAssignmentAuthorizationHook(db)

	orgOwnerID := "org-owner-delete-002"
	orgMemberID := "org-member-delete-002"

	// Create organization with owner and regular member
	orgID, err := uuid.NewV7()
	require.NoError(t, err)
	org := &orgModels.Organization{
		Name: "Delete Block Test Org", DisplayName: "Delete Block Test Org",
		OwnerUserID: orgOwnerID, OrganizationType: orgModels.OrgTypeTeam,
		MaxMembers: 100, IsActive: true,
	}
	org.ID = orgID
	require.NoError(t, db.Omit("Metadata").Create(org).Error)

	require.NoError(t, db.Omit("Metadata").Create(&orgModels.OrganizationMember{
		OrganizationID: orgID, UserID: orgOwnerID, Role: orgModels.OrgRoleOwner,
		JoinedAt: time.Now(), IsActive: true,
	}).Error)

	require.NoError(t, db.Omit("Metadata").Create(&orgModels.OrganizationMember{
		OrganizationID: orgID, UserID: orgMemberID, Role: orgModels.OrgRoleMember,
		JoinedAt: time.Now(), IsActive: true,
	}).Error)

	// Create scenario and assignment
	scenario := &models.Scenario{
		Name: "org-delete-block-test", Title: "Org Delete Block Test",
		InstanceType: "ubuntu:22.04", CreatedByID: orgOwnerID,
	}
	require.NoError(t, db.Create(scenario).Error)

	assignment := &models.ScenarioAssignment{
		BaseModel:      entityManagementModels.BaseModel{ID: uuid.New()},
		ScenarioID:     scenario.ID,
		OrganizationID: &orgID,
		Scope:          "org",
		CreatedByID:    orgOwnerID,
		IsActive:       true,
	}
	require.NoError(t, db.Create(assignment).Error)

	ctx := &hooks.HookContext{
		EntityName: "ScenarioAssignment",
		HookType:   hooks.BeforeDelete,
		EntityID:   assignment.ID,
		NewEntity:  assignment,
		UserID:     orgMemberID,
		UserRoles:  []string{"Member"},
	}

	err = hook.Execute(ctx)
	assert.Error(t, err, "Non-manager org member should be blocked from deleting org-scoped assignment")
	assert.Contains(t, err.Error(), "permission", "Error should mention permission denial")
}

// ============================================================================
// Organization-Level Authorization Tests — BeforeUpdate
// ============================================================================

func TestScenarioAssignmentAuth_BeforeUpdate_OrgManagerCanUpdate(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.AutoMigrate(&orgModels.Organization{}, &orgModels.OrganizationMember{}))

	hook := scenarioHooks.NewScenarioAssignmentAuthorizationHook(db)

	orgManagerID := "org-manager-update-001"

	// Create organization with a manager member
	orgID, err := uuid.NewV7()
	require.NoError(t, err)
	org := &orgModels.Organization{
		Name: "Update Auth Test Org", DisplayName: "Update Auth Test Org",
		OwnerUserID: "org-owner-update-001", OrganizationType: orgModels.OrgTypeTeam,
		MaxMembers: 100, IsActive: true,
	}
	org.ID = orgID
	require.NoError(t, db.Omit("Metadata").Create(org).Error)

	require.NoError(t, db.Omit("Metadata").Create(&orgModels.OrganizationMember{
		OrganizationID: orgID, UserID: orgManagerID, Role: orgModels.OrgRoleManager,
		JoinedAt: time.Now(), IsActive: true,
	}).Error)

	// Create scenario and assignment
	scenario := &models.Scenario{
		Name: "org-update-test", Title: "Org Update Test",
		InstanceType: "ubuntu:22.04", CreatedByID: "org-owner-update-001",
	}
	require.NoError(t, db.Create(scenario).Error)

	assignment := &models.ScenarioAssignment{
		BaseModel:      entityManagementModels.BaseModel{ID: uuid.New()},
		ScenarioID:     scenario.ID,
		OrganizationID: &orgID,
		Scope:          "org",
		CreatedByID:    "org-owner-update-001",
		IsActive:       true,
	}
	require.NoError(t, db.Create(assignment).Error)

	ctx := &hooks.HookContext{
		EntityName: "ScenarioAssignment",
		HookType:   hooks.BeforeUpdate,
		EntityID:   assignment.ID,
		OldEntity:  assignment,
		NewEntity:  map[string]any{"is_active": false},
		UserID:     orgManagerID,
		UserRoles:  []string{"Member"},
	}

	err = hook.Execute(ctx)
	assert.NoError(t, err, "Org manager should be able to update org-scoped assignment")
}

func TestScenarioAssignmentAuth_BeforeUpdate_NonOrgManagerBlocked(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.AutoMigrate(&orgModels.Organization{}, &orgModels.OrganizationMember{}))

	hook := scenarioHooks.NewScenarioAssignmentAuthorizationHook(db)

	orgOwnerID := "org-owner-update-002"
	orgMemberID := "org-member-update-002"

	// Create organization with owner and regular member
	orgID, err := uuid.NewV7()
	require.NoError(t, err)
	org := &orgModels.Organization{
		Name: "Update Block Test Org", DisplayName: "Update Block Test Org",
		OwnerUserID: orgOwnerID, OrganizationType: orgModels.OrgTypeTeam,
		MaxMembers: 100, IsActive: true,
	}
	org.ID = orgID
	require.NoError(t, db.Omit("Metadata").Create(org).Error)

	require.NoError(t, db.Omit("Metadata").Create(&orgModels.OrganizationMember{
		OrganizationID: orgID, UserID: orgOwnerID, Role: orgModels.OrgRoleOwner,
		JoinedAt: time.Now(), IsActive: true,
	}).Error)

	require.NoError(t, db.Omit("Metadata").Create(&orgModels.OrganizationMember{
		OrganizationID: orgID, UserID: orgMemberID, Role: orgModels.OrgRoleMember,
		JoinedAt: time.Now(), IsActive: true,
	}).Error)

	// Create scenario and assignment
	scenario := &models.Scenario{
		Name: "org-update-block-test", Title: "Org Update Block Test",
		InstanceType: "ubuntu:22.04", CreatedByID: orgOwnerID,
	}
	require.NoError(t, db.Create(scenario).Error)

	assignment := &models.ScenarioAssignment{
		BaseModel:      entityManagementModels.BaseModel{ID: uuid.New()},
		ScenarioID:     scenario.ID,
		OrganizationID: &orgID,
		Scope:          "org",
		CreatedByID:    orgOwnerID,
		IsActive:       true,
	}
	require.NoError(t, db.Create(assignment).Error)

	ctx := &hooks.HookContext{
		EntityName: "ScenarioAssignment",
		HookType:   hooks.BeforeUpdate,
		EntityID:   assignment.ID,
		OldEntity:  assignment,
		NewEntity:  map[string]any{"is_active": false},
		UserID:     orgMemberID,
		UserRoles:  []string{"Member"},
	}

	err = hook.Execute(ctx)
	assert.Error(t, err, "Non-manager org member should be blocked from updating org-scoped assignment")
	assert.Contains(t, err.Error(), "permission", "Error should mention permission denial")
}

// ============================================================================
// Admin Bypass for Organization-Scoped Operations
// ============================================================================

func TestScenarioAssignmentAuth_BeforeCreate_AdminBypassesOrgCheck(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.AutoMigrate(&orgModels.Organization{}, &orgModels.OrganizationMember{}))

	hook := scenarioHooks.NewScenarioAssignmentAuthorizationHook(db)

	adminID := "platform-admin-org-001"
	orgID := uuid.New()

	scenario := &models.Scenario{
		Name: "admin-org-create-test", Title: "Admin Org Create Test",
		InstanceType: "ubuntu:22.04", CreatedByID: "some-creator",
	}
	require.NoError(t, db.Create(scenario).Error)

	// Admin should be able to create org-scoped assignment without being an org member
	assignment := &models.ScenarioAssignment{
		ScenarioID:     scenario.ID,
		OrganizationID: &orgID,
		Scope:          "org",
		IsActive:       true,
	}

	ctx := &hooks.HookContext{
		EntityName: "ScenarioAssignment",
		HookType:   hooks.BeforeCreate,
		NewEntity:  assignment,
		UserID:     adminID,
		UserRoles:  []string{"Administrator"},
	}

	err := hook.Execute(ctx)
	assert.NoError(t, err, "Platform admin should bypass org-level checks for create")
}

func TestScenarioAssignmentAuth_BeforeDelete_AdminBypassesOrgCheck(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.AutoMigrate(&orgModels.Organization{}, &orgModels.OrganizationMember{}))

	hook := scenarioHooks.NewScenarioAssignmentAuthorizationHook(db)

	adminID := "platform-admin-org-002"
	orgID := uuid.New()

	scenario := &models.Scenario{
		Name: "admin-org-delete-test", Title: "Admin Org Delete Test",
		InstanceType: "ubuntu:22.04", CreatedByID: "some-creator",
	}
	require.NoError(t, db.Create(scenario).Error)

	assignment := &models.ScenarioAssignment{
		BaseModel:      entityManagementModels.BaseModel{ID: uuid.New()},
		ScenarioID:     scenario.ID,
		OrganizationID: &orgID,
		Scope:          "org",
		CreatedByID:    "some-creator",
		IsActive:       true,
	}
	require.NoError(t, db.Create(assignment).Error)

	ctx := &hooks.HookContext{
		EntityName: "ScenarioAssignment",
		HookType:   hooks.BeforeDelete,
		EntityID:   assignment.ID,
		NewEntity:  assignment,
		UserID:     adminID,
		UserRoles:  []string{"Administrator"},
	}

	err := hook.Execute(ctx)
	assert.NoError(t, err, "Platform admin should bypass org-level checks for delete")
}
