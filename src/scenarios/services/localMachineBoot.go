package services

import (
	"encoding/json"
	"log"

	configRepositories "soli/formations/src/configuration/repositories"
	"soli/formations/src/scenarios/models"

	"gorm.io/gorm"
)

// LocalMachineBootKey is the Platform Settings entry holding the
// administrator's models.LocalMachineBoot as JSON.
const LocalMachineBootKey = "local_machine_boot"

// LocalMachineBootDefaults is the platform's boot for machines in the
// learner's browser: what the administrator stored over the built-in values.
// Nothing stored, or a value edited by hand into something unreadable, gives
// the built-in values.
func LocalMachineBootDefaults(db *gorm.DB) models.LocalMachineBoot {
	builtIn := models.BuiltInLocalMachineBoot()
	feature, err := configRepositories.NewFeatureRepository(db).GetFeatureByKey(LocalMachineBootKey)
	if err != nil {
		return builtIn
	}
	stored, err := models.ParseLocalMachineBoot(feature.Value)
	if err != nil {
		log.Printf("ignoring the stored %s: %v", LocalMachineBootKey, err)
		return builtIn
	}
	return stored.Over(builtIn)
}

// SaveLocalMachineBootDefaults stores the administrator's boot; unset fields
// keep following the built-in values.
func SaveLocalMachineBootDefaults(db *gorm.DB, boot models.LocalMachineBoot) error {
	value, err := json.Marshal(boot)
	if err != nil {
		return err
	}
	return configRepositories.NewFeatureRepository(db).UpdateFeatureValue(LocalMachineBootKey, string(value))
}

// LocalMachineBootFor is how a run's machine in the learner's browser boots:
// each key the scenario sets over the platform's defaults, except append,
// which only the platform sets — even should a row carry one past the hooks.
func LocalMachineBootFor(db *gorm.DB, scenario models.Scenario) models.LocalMachineBoot {
	defaults := LocalMachineBootDefaults(db)
	overrides := models.DecodeScenarioMachineBoot(scenario.MachineBoot)
	if overrides == nil {
		return defaults
	}
	overrides.Append = nil
	return overrides.Over(defaults)
}
