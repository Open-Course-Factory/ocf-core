package configuration_tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"soli/formations/src/configuration/models"
	configServices "soli/formations/src/configuration/services"
	"soli/formations/src/scenarios"
)

func scenarioFeature(t *testing.T, key string) models.FeatureDefinition {
	t.Helper()
	for _, feature := range scenarios.NewScenariosModuleConfig().GetFeatures() {
		if feature.Key == key {
			return feature
		}
	}
	require.Failf(t, "feature not declared", "the scenarios module must declare %q", key)
	return models.FeatureDefinition{}
}

// The Effects tab of the step editor is off until an operator turns it on.
// The flag gates the editor tab only: banners a step already declares keep
// playing for learners.
func TestScenariosModule_DeclaresStepEffectsFlagOff(t *testing.T) {
	assert.Equal(t, models.FeatureDefinition{
		Key:         "scenario_step_effects",
		Name:        "Scenario step effects",
		Description: "Show the Effects tab (intro/outro terminal banners) in the scenario step editor",
		Enabled:     false,
		Category:    "modules",
		Module:      "scenarios",
	}, scenarioFeature(t, "scenario_step_effects"))
}

// Seeding is create-only: a fresh database gets the declared default, and a
// row that already exists — enabled by hand in a dev database — keeps its
// state.
func TestScenariosModule_StepEffectsSeedingIsCreateOnly(t *testing.T) {
	seed := func(t *testing.T, db *gorm.DB) models.Feature {
		t.Helper()
		original := configServices.GlobalFeatureRegistry
		t.Cleanup(func() { configServices.GlobalFeatureRegistry = original })
		configServices.InitFeatureRegistry(db)
		configServices.GlobalFeatureRegistry.RegisterFeatures(scenarios.NewScenariosModuleConfig().GetFeatures())
		configServices.GlobalFeatureRegistry.SeedRegisteredFeatures()
		var stored models.Feature
		require.NoError(t, db.Where("key = ?", "scenario_step_effects").First(&stored).Error)
		return stored
	}
	openDB := func(t *testing.T) *gorm.DB {
		t.Helper()
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		require.NoError(t, err)
		require.NoError(t, db.AutoMigrate(&models.Feature{}))
		return db
	}

	t.Run("fresh database", func(t *testing.T) {
		assert.False(t, seed(t, openDB(t)).Enabled)
	})
	t.Run("existing row enabled by hand", func(t *testing.T) {
		db := openDB(t)
		require.NoError(t, db.Create(&models.Feature{Key: "scenario_step_effects", Name: "hand-made", Enabled: true}).Error)
		stored := seed(t, db)
		assert.True(t, stored.Enabled, "seeding never overwrites an existing row's state")
		assert.Equal(t, "hand-made", stored.Name, "nor its name")
		assert.Equal(t, "scenarios", stored.Module, "only an empty module is filled in")
	})
}
