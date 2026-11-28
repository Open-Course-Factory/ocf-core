package terminalTrainer

import (
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
			Key:         services.PortExposureFeatureKey,
			Name:        "Public port exposure",
			Description: "Let learners publish a port of their terminal session at a public URL, on plans that allow it. Off until the operator's Traefik and overlay are in place; switching it off stops every published route within one poll.",
			Enabled:     false,
			Category:    "modules",
			Module:      "terminals",
		},
		{
			Key:         services.LocalMachinesFeatureKey,
			Name:        "Local machines",
			Description: "Let scenarios run on a machine inside the learner's browser tab instead of a container, for plans and scenarios that allow it. Off: every new run goes to a container; runs in progress finish where they started.",
			Enabled:     false,
			Category:    "modules",
			Module:      "terminals",
		},
		{
			Key:         services.LocalMachineDesktopFeatureKey,
			Name:        "Local machines: desktop",
			Description: "Let scenarios show a desktop, not only a terminal, on a machine inside the learner's browser tab. Needs Local machines on too. Off: desktop scenarios are hidden from learners and cannot be launched; authors still edit them, and runs in progress finish.",
			Enabled:     false,
			Category:    "modules",
			Module:      "terminals",
		},
		{
			// Not a toggle: the value carries the curation. tt-backend answers
			// with every image it can run, including scenario base images that
			// were never meant to be offered when starting a terminal.
			//
			// Seeded empty on purpose. Which images to withhold differs per
			// deployment and the names belong to tt-backend's catalog and to
			// scenario content, so a built-in list would couple this module to
			// data it does not own. An administrator sets it in Platform
			// Settings.
			// A retention period, set by the administrator in Platform
			// Settings. Empty or unreadable means the built-in default.
			Key:         services.ExposedPortRetentionDaysKey,
			Name:        "Exposed ports: row retention (days)",
			Description: "How many days an exposed-port record is kept after its expiry before it is deleted. The audit trail keeps who/what/when under its own retention. Blank means 30.",
			Enabled:     true,
			Category:    "settings",
			Module:      "terminals",
			Value:       "30",
		},
		{
			Key:         services.UnlistedDistributionsKey,
			Name:        "Distributions hidden from the picker",
			Description: "Comma-separated distribution names to withhold from the terminal launcher, e.g. scenario base images. They stay launchable by name, so scenarios are unaffected. Empty withholds nothing.",
			Enabled:     true,
			Category:    "settings",
			Module:      "terminals",
			Value:       "",
		},
	}
}
