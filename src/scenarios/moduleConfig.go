package scenarios

import (
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
	}
}
