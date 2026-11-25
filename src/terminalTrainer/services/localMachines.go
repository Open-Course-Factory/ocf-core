package services

import (
	configRepositories "soli/formations/src/configuration/repositories"

	"gorm.io/gorm"
)

// LocalMachinesFeatureKey is the platform switch for scenario runs on a
// machine in the learner's browser tab instead of an Incus container.
const LocalMachinesFeatureKey = "local_machines"

// LocalMachinesEnabled reads the platform switch, failing closed: a missing
// row (failed seed, deleted row) must not turn it on, so this does not use
// IsFeatureEnabled, which reads a missing row as on.
func LocalMachinesEnabled(db *gorm.DB) bool {
	feature, err := configRepositories.NewFeatureRepository(db).GetFeatureByKey(LocalMachinesFeatureKey)
	return err == nil && feature.Enabled
}
