package terminalTrainer

import (
	"strings"

	configInterfaces "soli/formations/src/configuration/interfaces"
	"soli/formations/src/configuration/models"
	"soli/formations/src/terminalTrainer/services"
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
		{
			// Not a toggle: the value carries the curation. tt-backend answers
			// with every image it can run, including scenario base images that
			// were never meant to be offered when starting a terminal.
			Key:         services.UnlistedDistributionsKey,
			Name:        "Distributions hidden from the picker",
			Description: "Comma-separated distribution names to withhold from the terminal launcher. They stay launchable by name, so scenarios are unaffected. Empty lists everything.",
			Enabled:     true,
			Category:    "settings",
			Module:      "terminals",
			Value:       strings.Join(services.DefaultUnlistedDistributions(), ","),
		},
	}
}
