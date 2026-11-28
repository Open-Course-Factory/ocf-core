package terminalTrainer_tests

import (
	"testing"

	configModels "soli/formations/src/configuration/models"
	"soli/formations/src/terminalTrainer"
	"soli/formations/src/terminalTrainer/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The platform switch for machines in the learner's browser fails closed: a
// missing row (failed seed, deleted row) must never turn it on, unlike
// IsFeatureEnabled, which reads a missing row as on.
func TestLocalMachinesEnabled_FailsClosed(t *testing.T) {
	db := freshTestDB(t)
	assert.False(t, services.LocalMachinesEnabled(db), "no row: off")

	flag := configModels.Feature{Key: services.LocalMachinesFeatureKey, Name: "x", Enabled: false}
	require.NoError(t, db.Create(&flag).Error)
	assert.False(t, services.LocalMachinesEnabled(db), "row off: off")

	require.NoError(t, db.Model(&flag).Update("enabled", true).Error)
	assert.True(t, services.LocalMachinesEnabled(db), "row on: on")
}

func TestLocalMachinesFlag_IsDeclaredOff(t *testing.T) {
	for _, f := range terminalTrainer.NewTerminalTrainerModuleConfig().GetFeatures() {
		if f.Key == services.LocalMachinesFeatureKey {
			assert.False(t, f.Enabled, "seeded off: an operator turns it on")
			return
		}
	}
	t.Fatal("the local machines flag is not declared, so it is never seeded and never shown to the operator")
}

func TestLocalMachineDesktopFlag_IsDeclaredOff(t *testing.T) {
	for _, f := range terminalTrainer.NewTerminalTrainerModuleConfig().GetFeatures() {
		if f.Key == services.LocalMachineDesktopFeatureKey {
			assert.False(t, f.Enabled, "seeded off: the desktop ships after the terminal")
			return
		}
	}
	t.Fatal("the desktop flag is not declared, so it is never seeded and never shown to the operator")
}
