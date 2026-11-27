package scenarios

import (
	"soli/formations/src/scenarios/services"

	configInterfaces "soli/formations/src/configuration/interfaces"
	"soli/formations/src/configuration/models"
)

// ScenariosModuleConfig implements the ModuleConfig interface
type ScenariosModuleConfig struct{}

// NewScenariosModuleConfig creates a new scenarios module configuration
func NewScenariosModuleConfig() configInterfaces.ModuleConfig {
	return &ScenariosModuleConfig{}
}

func (s *ScenariosModuleConfig) GetModuleName() string {
	return "scenarios"
}

func (s *ScenariosModuleConfig) GetFeatures() []models.FeatureDefinition {
	return []models.FeatureDefinition{
		{
			Key:         "scenarios",
			Name:        "Interactive Scenarios",
			Description: "Enable/disable interactive lab scenarios with step-by-step guidance, verification scripts, and CTF-style flag challenges",
			Enabled:     true,
			Category:    "modules",
			Module:      "scenarios",
		},
		{
			Key:         "scenario_conception",
			Name:        "Scenario Editor",
			Description: "Enable/disable the visual scenario editor for teachers to design and manage interactive lab scenarios",
			Enabled:     false,
			Category:    "modules",
			Module:      "scenarios",
		},
		{
			// Gates the editor tab only: banners a step already declares keep
			// playing for learners.
			Key:         "scenario_step_effects",
			Name:        "Scenario step effects",
			Description: "Show the Effects tab (intro/outro terminal banners) in the scenario step editor",
			Enabled:     false,
			Category:    "modules",
			Module:      "scenarios",
		},
		{
			// Set through PUT /local-machines/boot-defaults, which validates it.
			Key:         services.LocalMachineBootKey,
			Name:        "Machines in the browser: boot defaults",
			Description: "JSON object of how unkernel boots a machine in the learner's browser: jit, verbose, display, sound, append, console_hidden. A key left out keeps its built-in value; a scenario may override any key but append.",
			Enabled:     true,
			Category:    "settings",
			Module:      "scenarios",
			Value:       "",
		},
	}
}
