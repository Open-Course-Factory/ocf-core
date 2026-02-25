// tests/payment/planFeaturesValidationHook_test.go
// Tests for the PlanFeaturesValidationHook that validates feature keys
// against the PlanFeature catalog before creating/updating SubscriptionPlans.
package payment_tests

import (
	"testing"

	"soli/formations/src/entityManagement/hooks"
	entityManagementModels "soli/formations/src/entityManagement/models"
	paymentHooks "soli/formations/src/payment/hooks"
	"soli/formations/src/payment/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedTestPlanFeatures inserts a small set of active features into the test DB.
func seedTestPlanFeatures(t *testing.T) {
	t.Helper()
	features := []models.PlanFeature{
		{Key: "unlimited_courses", DisplayNameEn: "Unlimited Courses", DisplayNameFr: "Cours illimites", Category: "capabilities", ValueType: "boolean", IsActive: true},
		{Key: "advanced_labs", DisplayNameEn: "Advanced Labs", DisplayNameFr: "Laboratoires avances", Category: "capabilities", ValueType: "boolean", IsActive: true},
		{Key: "machine_size_xs", DisplayNameEn: "XS Machine", DisplayNameFr: "Machine XS", Category: "machine_sizes", ValueType: "boolean", IsActive: true},
		{Key: "network_access", DisplayNameEn: "Network Access", DisplayNameFr: "Acces reseau", Category: "terminal_limits", ValueType: "boolean", IsActive: true},
	}
	for _, f := range features {
		require.NoError(t, sharedTestDB.Create(&f).Error)
	}
}

// ==========================================
// GetName / GetEntityName / GetHookTypes
// ==========================================

func TestPlanFeaturesValidationHook_GetName_ReturnsPlanFeaturesValidation(t *testing.T) {
	db := freshTestDB(t)
	hook := paymentHooks.NewPlanFeaturesValidationHook(db)
	assert.Equal(t, "plan_features_validation", hook.GetName())
}

func TestPlanFeaturesValidationHook_GetEntityName_ReturnsSubscriptionPlan(t *testing.T) {
	db := freshTestDB(t)
	hook := paymentHooks.NewPlanFeaturesValidationHook(db)
	assert.Equal(t, "SubscriptionPlan", hook.GetEntityName())
}

func TestPlanFeaturesValidationHook_GetHookTypes_ReturnsBeforeCreateAndBeforeUpdate(t *testing.T) {
	db := freshTestDB(t)
	hook := paymentHooks.NewPlanFeaturesValidationHook(db)

	hookTypes := hook.GetHookTypes()
	assert.Len(t, hookTypes, 2)
	assert.Contains(t, hookTypes, hooks.BeforeCreate)
	assert.Contains(t, hookTypes, hooks.BeforeUpdate)
}

// ==========================================
// Execute — valid scenarios (should PASS)
// ==========================================

func TestPlanFeaturesValidationHook_ValidFeatures_Succeeds(t *testing.T) {
	db := freshTestDB(t)
	seedTestPlanFeatures(t)
	hook := paymentHooks.NewPlanFeaturesValidationHook(db)

	plan := &models.SubscriptionPlan{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Name:      "Test Plan",
		Features:  []string{"unlimited_courses", "advanced_labs", "machine_size_xs"},
	}

	ctx := &hooks.HookContext{
		EntityName: "SubscriptionPlan",
		HookType:   hooks.BeforeCreate,
		NewEntity:  plan,
	}

	err := hook.Execute(ctx)
	assert.NoError(t, err, "Valid features should not produce an error")
}

func TestPlanFeaturesValidationHook_EmptyFeatures_Skips(t *testing.T) {
	db := freshTestDB(t)
	seedTestPlanFeatures(t)
	hook := paymentHooks.NewPlanFeaturesValidationHook(db)

	plan := &models.SubscriptionPlan{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Name:      "Empty Features Plan",
		Features:  []string{},
	}

	ctx := &hooks.HookContext{
		EntityName: "SubscriptionPlan",
		HookType:   hooks.BeforeCreate,
		NewEntity:  plan,
	}

	err := hook.Execute(ctx)
	assert.NoError(t, err, "Empty features should skip validation")
}

func TestPlanFeaturesValidationHook_EmptyCatalog_Skips(t *testing.T) {
	db := freshTestDB(t)
	// Do NOT seed any features — catalog is empty
	hook := paymentHooks.NewPlanFeaturesValidationHook(db)

	plan := &models.SubscriptionPlan{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Name:      "Plan With Features But No Catalog",
		Features:  []string{"unlimited_courses"},
	}

	ctx := &hooks.HookContext{
		EntityName: "SubscriptionPlan",
		HookType:   hooks.BeforeCreate,
		NewEntity:  plan,
	}

	err := hook.Execute(ctx)
	assert.NoError(t, err, "Empty catalog should skip validation (bootstrap guard)")
}

// ==========================================
// Execute — invalid scenarios (should PASS)
// ==========================================

func TestPlanFeaturesValidationHook_InvalidFeatures_ReturnsError(t *testing.T) {
	db := freshTestDB(t)
	seedTestPlanFeatures(t)
	hook := paymentHooks.NewPlanFeaturesValidationHook(db)

	plan := &models.SubscriptionPlan{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Name:      "Bad Features Plan",
		Features:  []string{"totally_fake_feature", "another_bad_one"},
	}

	ctx := &hooks.HookContext{
		EntityName: "SubscriptionPlan",
		HookType:   hooks.BeforeCreate,
		NewEntity:  plan,
	}

	err := hook.Execute(ctx)
	assert.Error(t, err, "Invalid features should produce an error")
	assert.Contains(t, err.Error(), "totally_fake_feature")
	assert.Contains(t, err.Error(), "another_bad_one")
}

func TestPlanFeaturesValidationHook_MixedValidInvalid_ReturnsError(t *testing.T) {
	db := freshTestDB(t)
	seedTestPlanFeatures(t)
	hook := paymentHooks.NewPlanFeaturesValidationHook(db)

	plan := &models.SubscriptionPlan{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Name:      "Mixed Features Plan",
		Features:  []string{"unlimited_courses", "nonexistent_feature", "machine_size_xs"},
	}

	ctx := &hooks.HookContext{
		EntityName: "SubscriptionPlan",
		HookType:   hooks.BeforeCreate,
		NewEntity:  plan,
	}

	err := hook.Execute(ctx)
	assert.Error(t, err, "Mixed features should produce an error for the invalid one")
	assert.Contains(t, err.Error(), "nonexistent_feature")
	// Valid features should NOT appear in the error
	assert.NotContains(t, err.Error(), "unlimited_courses")
	assert.NotContains(t, err.Error(), "machine_size_xs")
}

// ==========================================
// Regression test — map[string]any input handling
// ==========================================

// TestPlanFeaturesValidationHook_BeforeUpdate_MapInput_Succeeds tests that the hook
// correctly handles map[string]any input during BeforeUpdate.
// The generic entity management framework passes ctx.NewEntity as map[string]any
// during PATCH updates (not as *models.SubscriptionPlan), so the hook must handle
// both types.
func TestPlanFeaturesValidationHook_BeforeUpdate_MapInput_Succeeds(t *testing.T) {
	db := freshTestDB(t)
	seedTestPlanFeatures(t)
	hook := paymentHooks.NewPlanFeaturesValidationHook(db)

	// Simulate the map[string]any that the generic framework passes during PATCH
	mapInput := map[string]any{
		"features": []string{"unlimited_courses", "machine_size_xs"},
	}

	ctx := &hooks.HookContext{
		EntityName: "SubscriptionPlan",
		HookType:   hooks.BeforeUpdate,
		NewEntity:  mapInput,
	}

	// This should validate the features from the map without panicking or erroring.
	err := hook.Execute(ctx)
	assert.NoError(t, err, "Hook should handle map[string]any input during BeforeUpdate without error")
}
