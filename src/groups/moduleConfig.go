package groups

import (
	configInterfaces "soli/formations/src/configuration/interfaces"
	"soli/formations/src/configuration/models"
)

// GroupsModuleConfig implements the ModuleConfig interface
type GroupsModuleConfig struct{}

// NewGroupsModuleConfig creates a new groups module configuration
func NewGroupsModuleConfig() configInterfaces.ModuleConfig {
	return &GroupsModuleConfig{}
}

func (g *GroupsModuleConfig) GetModuleName() string {
	return "groups"
}

func (g *GroupsModuleConfig) GetFeatures() []models.FeatureDefinition {
	return []models.FeatureDefinition{
		{
			Key:         "class_groups",
			Name:        "Class Groups Management",
			Description: "Enable/disable class groups and team management features including member management, group sharing, and role-based access control",
			Enabled:     true,
			Category:    "modules",
			Module:      "groups",
		},
	}
}
