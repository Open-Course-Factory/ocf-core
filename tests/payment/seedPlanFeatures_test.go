// tests/payment/seedPlanFeatures_test.go
// Tests for the SeedPlanFeatures function that populates the plan_features catalog.
package payment_tests

import (
	"testing"

	"soli/formations/src/initialization"
	"soli/formations/src/payment/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeedPlanFeatures_EmptyDB_SeedsAllFeatures(t *testing.T) {
	db := freshTestDB(t)

	initialization.SeedPlanFeatures(db)

	var count int64
	db.Model(&models.PlanFeature{}).Count(&count)
	assert.Equal(t, int64(25), count, "Should seed exactly 25 features")
}

func TestSeedPlanFeatures_RerunIsIdempotent(t *testing.T) {
	db := freshTestDB(t)

	// First seed
	initialization.SeedPlanFeatures(db)
	var countAfterFirst int64
	db.Model(&models.PlanFeature{}).Count(&countAfterFirst)
	require.Equal(t, int64(25), countAfterFirst)

	// Second seed — per-row FirstOrCreate must not add duplicates
	initialization.SeedPlanFeatures(db)
	var countAfterSecond int64
	db.Model(&models.PlanFeature{}).Count(&countAfterSecond)
	assert.Equal(t, countAfterFirst, countAfterSecond, "Second seed should not add more features (idempotent)")
}

// TestSeedPlanFeatures_TopsUpExistingDB verifies the seed adds newly-introduced
// features to a DB that was already populated with an older catalog — the case
// that motivated the switch from "skip if any rows exist" to per-row FirstOrCreate.
func TestSeedPlanFeatures_TopsUpExistingDB(t *testing.T) {
	db := freshTestDB(t)

	// Simulate an older deployment that only has one feature already seeded.
	existing := models.PlanFeature{
		Key: "unlimited_courses", DisplayNameEn: "Unlimited Courses", DisplayNameFr: "Formations illimitées",
		Category: "capabilities", ValueType: "boolean", DefaultValue: "false", IsActive: true,
	}
	require.NoError(t, db.Create(&existing).Error)

	initialization.SeedPlanFeatures(db)

	var count int64
	db.Model(&models.PlanFeature{}).Count(&count)
	assert.Equal(t, int64(25), count, "Seed should top up missing features without duplicating existing ones")
}

func TestSeedPlanFeatures_IncludesPersistenceFields(t *testing.T) {
	db := freshTestDB(t)
	initialization.SeedPlanFeatures(db)

	var enabled models.PlanFeature
	require.NoError(t, db.Where("key = ?", "persistent_sessions_enabled").First(&enabled).Error)
	assert.Equal(t, "terminal_limits", enabled.Category)
	assert.Equal(t, "boolean", enabled.ValueType)
	assert.Equal(t, "false", enabled.DefaultValue)
	assert.NotEmpty(t, enabled.DescriptionEn)
	assert.NotEmpty(t, enabled.DescriptionFr)

	var maxPersistent models.PlanFeature
	require.NoError(t, db.Where("key = ?", "max_persistent_sessions").First(&maxPersistent).Error)
	assert.Equal(t, "terminal_limits", maxPersistent.Category)
	assert.Equal(t, "number", maxPersistent.ValueType)
	assert.Equal(t, "0", maxPersistent.DefaultValue)
	assert.NotEmpty(t, maxPersistent.DescriptionEn)
	assert.NotEmpty(t, maxPersistent.DescriptionFr)
}

func TestSeedPlanFeatures_FeatureCategories_AllPresent(t *testing.T) {
	db := freshTestDB(t)
	initialization.SeedPlanFeatures(db)

	expectedCategories := []string{"capabilities", "machine_sizes", "terminal_limits", "course_limits"}

	for _, category := range expectedCategories {
		var count int64
		db.Model(&models.PlanFeature{}).Where("category = ?", category).Count(&count)
		assert.Greater(t, count, int64(0), "Category %q should have at least one feature", category)
	}

	// Verify no unexpected categories exist
	var distinctCategories []string
	db.Model(&models.PlanFeature{}).Distinct("category").Pluck("category", &distinctCategories)
	assert.ElementsMatch(t, expectedCategories, distinctCategories, "Only expected categories should exist")
}

func TestSeedPlanFeatures_FeatureKeys_NoDuplicates(t *testing.T) {
	db := freshTestDB(t)
	initialization.SeedPlanFeatures(db)

	var allKeys []string
	db.Model(&models.PlanFeature{}).Pluck("key", &allKeys)

	// Check for duplicates by comparing length with a set
	keySet := make(map[string]bool)
	for _, key := range allKeys {
		assert.False(t, keySet[key], "Duplicate feature key found: %s", key)
		keySet[key] = true
	}
}

func TestSeedPlanFeatures_BilingualNames_AllPopulated(t *testing.T) {
	db := freshTestDB(t)
	initialization.SeedPlanFeatures(db)

	var features []models.PlanFeature
	db.Find(&features)

	for _, feature := range features {
		assert.NotEmpty(t, feature.DisplayNameEn, "Feature %q should have an English display name", feature.Key)
		assert.NotEmpty(t, feature.DisplayNameFr, "Feature %q should have a French display name", feature.Key)
	}
}
