package terminalTrainer

import (
	configInterfaces "soli/formations/src/configuration/interfaces"
	"soli/formations/src/configuration/models"
)

// TerminalTrainerModuleConfig implements the ModuleConfig interface
type TerminalTrainerModuleConfig struct{}

// NewTerminalTrainerModuleConfig creates a new terminal trainer module configuration
func NewTerminalTrainerModuleConfig() configInterfaces.ModuleConfig {
	return &TerminalTrainerModuleConfig{}
}

func (t *TerminalTrainerModuleConfig) GetModuleName() string {
	return "terminals"
}

func (t *TerminalTrainerModuleConfig) GetFeatures() []models.FeatureDefinition {
	return []models.FeatureDefinition{
		{
			Key:         "terminals",
			Name:        "Terminal Trainer",
			Description: "Enable/disable interactive terminal training sessions with sharing and collaboration features",
			Enabled:     true,
			Category:    "modules",
			Module:      "terminals",
		},
	}
}
