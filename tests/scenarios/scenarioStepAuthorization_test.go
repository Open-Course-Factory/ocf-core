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

// =============================================================================
// ScenarioStep authorization
// =============================================================================

func TestScenarioStep_CreateAsOrgManager_Allowed(t *testing.T) {
	db := setupTestDB(t)
	hook := scenarioHooks.NewScenarioStepAuthorizationHook(db)

	orgOwnerID := "step-org-owner-001"
	orgManagerID := "step-org-manager-001"

	orgID := uuid.New()
	org := &orgModels.Organization{
		Name:             "Step Org Manager Allowed",
		DisplayName:      "Step Org Manager Allowed",
		OwnerUserID:      orgOwnerID,
		OrganizationType: orgModels.OrgTypeTeam,
		MaxMembers:       100,
		IsActive:         true,
	}
	org.ID = orgID
	require.NoError(t, db.Omit("Metadata").Create(org).Error)

	require.NoError(t, db.Omit("Metadata").Create(&orgModels.OrganizationMember{
		OrganizationID: orgID, UserID: orgManagerID, Role: orgModels.OrgRoleManager,
		JoinedAt: time.Now(), IsActive: true,
	}).Error)

	scenario := &models.Scenario{
		Name:           "step-create-org-manager",
		Title:          "Step Create Org Manager",
		InstanceType:   "ubuntu:22.04",
		CreatedByID:    orgOwnerID,
		OrganizationID: &orgID,
	}
	require.NoError(t, db.Create(scenario).Error)

	newStep := &models.ScenarioStep{
		ScenarioID: scenario.ID,
		Order:      1,
		Title:      "First step",
		StepType:   "info",
	}

	ctx := &hooks.HookContext{
		EntityName: "ScenarioStep",
		HookType:   hooks.BeforeCreate,
		NewEntity:  newStep,
		UserID:     orgManagerID,
		UserRoles:  []string{"Member"},
	}

	err := hook.Execute(ctx)
	assert.NoError(t, err, "Org manager should be allowed to create steps on org-scoped scenario")
}

func TestScenarioStep_CreateAsUnrelatedMember_Forbidden(t *testing.T) {
	db := setupTestDB(t)
	hook := scenarioHooks.NewScenarioStepAuthorizationHook(db)

	orgOwnerID := "step-org-owner-002"
	unrelatedID := "step-unrelated-002"

	orgID := uuid.New()
	org := &orgModels.Organization{
		Name:             "Step Unrelated Forbidden",
		DisplayName:      "Step Unrelated Forbidden",
		OwnerUserID:      orgOwnerID,
		OrganizationType: orgModels.OrgTypeTeam,
		MaxMembers:       100,
		IsActive:         true,
	}
	org.ID = orgID
	require.NoError(t, db.Omit("Metadata").Create(org).Error)

	require.NoError(t, db.Omit("Metadata").Create(&orgModels.OrganizationMember{
		OrganizationID: orgID, UserID: orgOwnerID, Role: orgModels.OrgRoleOwner,
		JoinedAt: time.Now(), IsActive: true,
	}).Error)

	scenario := &models.Scenario{
		Name:           "step-create-unrelated",
		Title:          "Step Create Unrelated",
		InstanceType:   "ubuntu:22.04",
		CreatedByID:    orgOwnerID,
		OrganizationID: &orgID,
	}
	require.NoError(t, db.Create(scenario).Error)

	newStep := &models.ScenarioStep{
		ScenarioID: scenario.ID,
		Order:      1,
		Title:      "Sneaky step",
		StepType:   "terminal",
	}

	ctx := &hooks.HookContext{
		EntityName: "ScenarioStep",
		HookType:   hooks.BeforeCreate,
		NewEntity:  newStep,
		UserID:     unrelatedID,
		UserRoles:  []string{"Member"},
	}

	err := hook.Execute(ctx)
	assert.Error(t, err, "Unrelated member must not be allowed to add steps to a scenario they do not own")
	assert.Contains(t, err.Error(), "permission")
}

func TestScenarioStep_CreateAsGroupManagerOfAssignedGroup_Allowed(t *testing.T) {
	db := setupTestDB(t)
	hook := scenarioHooks.NewScenarioStepAuthorizationHook(db)

	scenarioCreatorID := "step-scenario-creator-003"
	groupOwnerID := "step-group-owner-003"

	// Group owned by groupOwnerID
	groupID, err := uuid.NewV7()
	require.NoError(t, err)
	group := &groupModels.ClassGroup{
		Name:        "Step Group Manager Allowed",
		DisplayName: "Step Group Manager Allowed",
		OwnerUserID: groupOwnerID,
		MaxMembers:  50,
		IsActive:    true,
	}
	group.ID = groupID
	require.NoError(t, db.Omit("Metadata").Create(group).Error)

	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: groupID, UserID: groupOwnerID, Role: groupModels.GroupMemberRoleOwner,
		InvitedBy: groupOwnerID, JoinedAt: time.Now(), IsActive: true,
	}).Error)

	// Scenario owned by someone else, not org-scoped, but assigned to the group.
	scenario := &models.Scenario{
		Name:         "step-group-manager-assigned",
		Title:        "Step Group Manager Assigned",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  scenarioCreatorID,
	}
	require.NoError(t, db.Create(scenario).Error)

	assignment := &models.ScenarioAssignment{
		BaseModel:   entityManagementModels.BaseModel{ID: uuid.New()},
		ScenarioID:  scenario.ID,
		GroupID:     &groupID,
		Scope:       "group",
		CreatedByID: scenarioCreatorID,
		IsActive:    true,
	}
	require.NoError(t, db.Create(assignment).Error)

	newStep := &models.ScenarioStep{
		ScenarioID: scenario.ID,
		Order:      1,
		Title:      "Group-managed step",
		StepType:   "info",
	}

	ctx := &hooks.HookContext{
		EntityName: "ScenarioStep",
		HookType:   hooks.BeforeCreate,
		NewEntity:  newStep,
		UserID:     groupOwnerID,
		UserRoles:  []string{"Member"},
	}

	err = hook.Execute(ctx)
	assert.NoError(t, err, "Group manager of an assigned group should be allowed to add steps")
}

func TestScenarioStep_CreateAsAdmin_AllowedWithoutScenarioRelationship(t *testing.T) {
	db := setupTestDB(t)
	hook := scenarioHooks.NewScenarioStepAuthorizationHook(db)

	scenario := &models.Scenario{
		Name:         "step-admin-bypass",
		Title:        "Step Admin Bypass",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "some-other-user",
	}
	require.NoError(t, db.Create(scenario).Error)

	newStep := &models.ScenarioStep{
		ScenarioID: scenario.ID,
		Order:      1,
		Title:      "Admin step",
		StepType:   "info",
	}

	ctx := &hooks.HookContext{
		EntityName: "ScenarioStep",
		HookType:   hooks.BeforeCreate,
		NewEntity:  newStep,
		UserID:     "platform-admin",
		UserRoles:  []string{"Administrator"},
	}

	err := hook.Execute(ctx)
	assert.NoError(t, err, "Platform admin should bypass scenario-ownership check")
}

func TestScenarioStep_DeleteAsCreator_Allowed(t *testing.T) {
	db := setupTestDB(t)
	hook := scenarioHooks.NewScenarioStepAuthorizationHook(db)

	creatorID := "step-creator-delete-005"

	scenario := &models.Scenario{
		Name:         "step-creator-delete",
		Title:        "Step Creator Delete",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  creatorID,
	}
	require.NoError(t, db.Create(scenario).Error)

	step := &models.ScenarioStep{
		BaseModel:  entityManagementModels.BaseModel{ID: uuid.New()},
		ScenarioID: scenario.ID,
		Order:      1,
		Title:      "Soon-to-be-deleted",
		StepType:   "terminal",
	}
	require.NoError(t, db.Create(step).Error)

	ctx := &hooks.HookContext{
		EntityName: "ScenarioStep",
		HookType:   hooks.BeforeDelete,
		EntityID:   step.ID,
		NewEntity:  step,
		UserID:     creatorID,
		UserRoles:  []string{"Member"},
	}

	err := hook.Execute(ctx)
	assert.NoError(t, err, "Scenario creator should be allowed to delete a step on their own scenario")
}

func TestScenarioStep_UpdateAsUnrelatedMember_Forbidden(t *testing.T) {
	db := setupTestDB(t)
	hook := scenarioHooks.NewScenarioStepAuthorizationHook(db)

	creatorID := "step-creator-update-006"
	unrelatedID := "step-unrelated-update-006"

	scenario := &models.Scenario{
		Name:         "step-update-block",
		Title:        "Step Update Block",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  creatorID,
	}
	require.NoError(t, db.Create(scenario).Error)

	step := &models.ScenarioStep{
		BaseModel:  entityManagementModels.BaseModel{ID: uuid.New()},
		ScenarioID: scenario.ID,
		Order:      1,
		Title:      "Original title",
		StepType:   "terminal",
	}
	require.NoError(t, db.Create(step).Error)

	ctx := &hooks.HookContext{
		EntityName: "ScenarioStep",
		HookType:   hooks.BeforeUpdate,
		EntityID:   step.ID,
		OldEntity:  step,
		NewEntity:  map[string]any{"title": "hacked"},
		UserID:     unrelatedID,
		UserRoles:  []string{"Member"},
	}

	err := hook.Execute(ctx)
	assert.Error(t, err, "Unrelated member must not be able to update steps on a scenario they do not own")
	assert.Contains(t, err.Error(), "permission")
}

// =============================================================================
// ScenarioStepQuestion authorization (transitive via Step → Scenario)
// =============================================================================

func TestScenarioStepQuestion_CreateAsOrgManager_Allowed(t *testing.T) {
	db := setupTestDB(t)
	hook := scenarioHooks.NewScenarioStepQuestionAuthorizationHook(db)

	orgOwnerID := "stepq-org-owner-001"
	orgManagerID := "stepq-org-manager-001"

	orgID := uuid.New()
	org := &orgModels.Organization{
		Name:             "StepQ Org Manager Allowed",
		DisplayName:      "StepQ Org Manager Allowed",
		OwnerUserID:      orgOwnerID,
		OrganizationType: orgModels.OrgTypeTeam,
		MaxMembers:       100,
		IsActive:         true,
	}
	org.ID = orgID
	require.NoError(t, db.Omit("Metadata").Create(org).Error)

	require.NoError(t, db.Omit("Metadata").Create(&orgModels.OrganizationMember{
		OrganizationID: orgID, UserID: orgManagerID, Role: orgModels.OrgRoleManager,
		JoinedAt: time.Now(), IsActive: true,
	}).Error)

	scenario := &models.Scenario{
		Name:           "stepq-org-manager",
		Title:          "StepQ Org Manager",
		InstanceType:   "ubuntu:22.04",
		CreatedByID:    orgOwnerID,
		OrganizationID: &orgID,
	}
	require.NoError(t, db.Create(scenario).Error)

	step := &models.ScenarioStep{
		BaseModel:  entityManagementModels.BaseModel{ID: uuid.New()},
		ScenarioID: scenario.ID,
		Order:      1,
		Title:      "Quiz step",
		StepType:   "quiz",
	}
	require.NoError(t, db.Create(step).Error)

	newQuestion := &models.ScenarioStepQuestion{
		StepID:        step.ID,
		Order:         1,
		QuestionText:  "What is 2+2?",
		QuestionType:  "free_text",
		CorrectAnswer: "4",
		Points:        1,
	}

	ctx := &hooks.HookContext{
		EntityName: "ScenarioStepQuestion",
		HookType:   hooks.BeforeCreate,
		NewEntity:  newQuestion,
		UserID:     orgManagerID,
		UserRoles:  []string{"Member"},
	}

	err := hook.Execute(ctx)
	assert.NoError(t, err, "Org manager should be allowed to add a question to a step on an org-scoped scenario")
}

func TestScenarioStepQuestion_CreateAsUnrelatedMember_Forbidden(t *testing.T) {
	db := setupTestDB(t)
	hook := scenarioHooks.NewScenarioStepQuestionAuthorizationHook(db)

	creatorID := "stepq-creator-forbidden-002"
	unrelatedID := "stepq-unrelated-002"

	scenario := &models.Scenario{
		Name:         "stepq-forbidden",
		Title:        "StepQ Forbidden",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  creatorID,
	}
	require.NoError(t, db.Create(scenario).Error)

	step := &models.ScenarioStep{
		BaseModel:  entityManagementModels.BaseModel{ID: uuid.New()},
		ScenarioID: scenario.ID,
		Order:      1,
		Title:      "Quiz step",
		StepType:   "quiz",
	}
	require.NoError(t, db.Create(step).Error)

	newQuestion := &models.ScenarioStepQuestion{
		StepID:        step.ID,
		Order:         1,
		QuestionText:  "leak?",
		QuestionType:  "free_text",
		CorrectAnswer: "no",
		Points:        1,
	}

	ctx := &hooks.HookContext{
		EntityName: "ScenarioStepQuestion",
		HookType:   hooks.BeforeCreate,
		NewEntity:  newQuestion,
		UserID:     unrelatedID,
		UserRoles:  []string{"Member"},
	}

	err := hook.Execute(ctx)
	assert.Error(t, err, "Unrelated member must not be allowed to add questions to a scenario step they do not own")
	assert.Contains(t, err.Error(), "permission")
}

func TestScenarioStepQuestion_DeleteAsGroupManagerOfAssignedGroup_Allowed(t *testing.T) {
	db := setupTestDB(t)
	hook := scenarioHooks.NewScenarioStepQuestionAuthorizationHook(db)

	scenarioCreatorID := "stepq-creator-003"
	groupOwnerID := "stepq-group-owner-003"

	groupID, err := uuid.NewV7()
	require.NoError(t, err)
	group := &groupModels.ClassGroup{
		Name:        "StepQ Group Manager Allowed",
		DisplayName: "StepQ Group Manager Allowed",
		OwnerUserID: groupOwnerID,
		MaxMembers:  50,
		IsActive:    true,
	}
	group.ID = groupID
	require.NoError(t, db.Omit("Metadata").Create(group).Error)

	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: groupID, UserID: groupOwnerID, Role: groupModels.GroupMemberRoleOwner,
		InvitedBy: groupOwnerID, JoinedAt: time.Now(), IsActive: true,
	}).Error)

	scenario := &models.Scenario{
		Name:         "stepq-group-manager-assigned",
		Title:        "StepQ Group Manager Assigned",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  scenarioCreatorID,
	}
	require.NoError(t, db.Create(scenario).Error)

	require.NoError(t, db.Create(&models.ScenarioAssignment{
		BaseModel:   entityManagementModels.BaseModel{ID: uuid.New()},
		ScenarioID:  scenario.ID,
		GroupID:     &groupID,
		Scope:       "group",
		CreatedByID: scenarioCreatorID,
		IsActive:    true,
	}).Error)

	step := &models.ScenarioStep{
		BaseModel:  entityManagementModels.BaseModel{ID: uuid.New()},
		ScenarioID: scenario.ID,
		Order:      1,
		Title:      "Quiz step",
		StepType:   "quiz",
	}
	require.NoError(t, db.Create(step).Error)

	question := &models.ScenarioStepQuestion{
		BaseModel:     entityManagementModels.BaseModel{ID: uuid.New()},
		StepID:        step.ID,
		Order:         1,
		QuestionText:  "Old question",
		QuestionType:  "free_text",
		CorrectAnswer: "old",
		Points:        1,
	}
	require.NoError(t, db.Create(question).Error)

	ctx := &hooks.HookContext{
		EntityName: "ScenarioStepQuestion",
		HookType:   hooks.BeforeDelete,
		EntityID:   question.ID,
		NewEntity:  question,
		UserID:     groupOwnerID,
		UserRoles:  []string{"Member"},
	}

	err = hook.Execute(ctx)
	assert.NoError(t, err, "Group manager of an assigned group should be allowed to delete questions on the scenario's steps")
}
