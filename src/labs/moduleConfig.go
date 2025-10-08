package labs

import (
	configInterfaces "soli/formations/src/configuration/interfaces"
	"soli/formations/src/configuration/models"
)

// LabsModuleConfig implements the ModuleConfig interface
type LabsModuleConfig struct{}

// NewLabsModuleConfig creates a new labs module configuration
func NewLabsModuleConfig() configInterfaces.ModuleConfig {
	return &LabsModuleConfig{}
}

func (l *LabsModuleConfig) GetModuleName() string {
	return "labs"
}

func (l *LabsModuleConfig) GetFeatures() []models.FeatureDefinition {
	return []models.FeatureDefinition{
		{
			Key:         "labs",
			Name:        "Lab Sessions",
			Description: "Enable/disable lab environment and session management including SSH connections and machine provisioning",
			Enabled:     true,
			Category:    "modules",
			Module:      "labs",
		},
	}
}
