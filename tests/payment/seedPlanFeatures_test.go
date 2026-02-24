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
	assert.Equal(t, int64(23), count, "Should seed exactly 23 features")
}

func TestSeedPlanFeatures_NonEmptyDB_SkipsSeed(t *testing.T) {
	db := freshTestDB(t)

	// First seed
	initialization.SeedPlanFeatures(db)
	var countAfterFirst int64
	db.Model(&models.PlanFeature{}).Count(&countAfterFirst)
	require.Equal(t, int64(23), countAfterFirst)

	// Second seed — should skip (idempotent)
	initialization.SeedPlanFeatures(db)
	var countAfterSecond int64
	db.Model(&models.PlanFeature{}).Count(&countAfterSecond)
	assert.Equal(t, countAfterFirst, countAfterSecond, "Second seed should not add more features (idempotent)")
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
