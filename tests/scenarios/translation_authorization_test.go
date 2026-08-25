package scenarios_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"soli/formations/src/entityManagement/hooks"
	scenarioHooks "soli/formations/src/scenarios/hooks"
	"soli/formations/src/scenarios/models"
)

// Translations are Member-writable so a trainer can translate their own
// scenarios. That makes an authorization hook on every write operation the only
// thing standing between a learner and someone else's content: an entity that
// grants Member write access and misses a hook is not weakly protected, it is
// unprotected.

func translatableStep(t *testing.T, db *gorm.DB, ownerID string) models.ScenarioStep {
	t.Helper()
	scenario := &models.Scenario{
		Name:         "translation-auth-" + ownerID,
		Title:        "Translation Auth",
		InstanceType: "debian",
		CreatedByID:  ownerID,
	}
	require.NoError(t, db.Create(scenario).Error)

	step := models.ScenarioStep{ScenarioID: scenario.ID, Order: 0, Title: "Step", StepType: "info"}
	require.NoError(t, db.Create(&step).Error)
	return step
}

func TestScenarioStepTranslation_CreateByStranger_IsRefused(t *testing.T) {
	db := setupTestDB(t)
	hook := scenarioHooks.NewScenarioStepTranslationAuthorizationHook(db)
	step := translatableStep(t, db, "owner-001")

	err := hook.Execute(&hooks.HookContext{
		EntityName: "ScenarioStepTranslation",
		HookType:   hooks.BeforeCreate,
		NewEntity:  &models.ScenarioStepTranslation{StepID: step.ID, Locale: "fr", Title: "Etape"},
		UserID:     "a-passing-learner",
		UserRoles:  []string{"Member"},
	})

	require.Error(t, err, "a member who cannot manage the scenario must not translate it")
}

func TestScenarioStepTranslation_CreateByCreator_IsAllowed(t *testing.T) {
	db := setupTestDB(t)
	hook := scenarioHooks.NewScenarioStepTranslationAuthorizationHook(db)
	step := translatableStep(t, db, "owner-002")

	err := hook.Execute(&hooks.HookContext{
		EntityName: "ScenarioStepTranslation",
		HookType:   hooks.BeforeCreate,
		NewEntity:  &models.ScenarioStepTranslation{StepID: step.ID, Locale: "fr", Title: "Etape"},
		UserID:     "owner-002",
		UserRoles:  []string{"Member"},
	})

	require.NoError(t, err)
}

// Update and delete are gated too. Guarding only create would leave a learner
// able to rewrite or remove someone else's translation, which is the same hole
// wearing a different verb.
func TestScenarioStepTranslation_UpdateByStranger_IsRefused(t *testing.T) {
	db := setupTestDB(t)
	hook := scenarioHooks.NewScenarioStepTranslationAuthorizationHook(db)
	step := translatableStep(t, db, "owner-003")

	existing := &models.ScenarioStepTranslation{StepID: step.ID, Locale: "fr", Title: "Etape"}
	require.NoError(t, db.Create(existing).Error)

	err := hook.Execute(&hooks.HookContext{
		EntityName: "ScenarioStepTranslation",
		HookType:   hooks.BeforeUpdate,
		OldEntity:  existing,
		NewEntity:  existing,
		UserID:     "a-passing-learner",
		UserRoles:  []string{"Member"},
	})

	require.Error(t, err)
}

func TestScenarioStepTranslation_DeleteByStranger_IsRefused(t *testing.T) {
	db := setupTestDB(t)
	hook := scenarioHooks.NewScenarioStepTranslationAuthorizationHook(db)
	step := translatableStep(t, db, "owner-004")

	existing := &models.ScenarioStepTranslation{StepID: step.ID, Locale: "fr", Title: "Etape"}
	require.NoError(t, db.Create(existing).Error)

	err := hook.Execute(&hooks.HookContext{
		EntityName: "ScenarioStepTranslation",
		HookType:   hooks.BeforeDelete,
		NewEntity:  existing,
		UserID:     "a-passing-learner",
		UserRoles:  []string{"Member"},
	})

	require.Error(t, err)
}

func TestScenarioStepTranslation_Admin_Bypasses(t *testing.T) {
	db := setupTestDB(t)
	hook := scenarioHooks.NewScenarioStepTranslationAuthorizationHook(db)
	step := translatableStep(t, db, "owner-005")

	err := hook.Execute(&hooks.HookContext{
		EntityName: "ScenarioStepTranslation",
		HookType:   hooks.BeforeCreate,
		NewEntity:  &models.ScenarioStepTranslation{StepID: step.ID, Locale: "fr", Title: "Etape"},
		UserID:     "platform-admin",
		UserRoles:  []string{"Administrator"},
	})

	require.NoError(t, err)
}

// The same rule guards a scenario's own translated fields.
func TestScenarioTranslation_CreateByStranger_IsRefused(t *testing.T) {
	db := setupTestDB(t)
	hook := scenarioHooks.NewScenarioTranslationAuthorizationHook(db)
	step := translatableStep(t, db, "owner-006")

	err := hook.Execute(&hooks.HookContext{
		EntityName: "ScenarioTranslation",
		HookType:   hooks.BeforeCreate,
		NewEntity:  &models.ScenarioTranslation{ScenarioID: step.ScenarioID, Locale: "fr", Title: "Scenario"},
		UserID:     "a-passing-learner",
		UserRoles:  []string{"Member"},
	})

	require.Error(t, err)
}

func TestScenarioTranslation_CreateByCreator_IsAllowed(t *testing.T) {
	db := setupTestDB(t)
	hook := scenarioHooks.NewScenarioTranslationAuthorizationHook(db)
	step := translatableStep(t, db, "owner-007")

	err := hook.Execute(&hooks.HookContext{
		EntityName: "ScenarioTranslation",
		HookType:   hooks.BeforeCreate,
		NewEntity:  &models.ScenarioTranslation{ScenarioID: step.ScenarioID, Locale: "fr", Title: "Scenario"},
		UserID:     "owner-007",
		UserRoles:  []string{"Member"},
	})

	require.NoError(t, err)
	assert.NotEqual(t, "", step.ScenarioID.String())
}
