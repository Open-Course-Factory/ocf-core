package courses

import (
	configInterfaces "soli/formations/src/configuration/interfaces"
	"soli/formations/src/configuration/models"
)

// CoursesModuleConfig implements the ModuleConfig interface
type CoursesModuleConfig struct{}

// NewCoursesModuleConfig creates a new courses module configuration
func NewCoursesModuleConfig() configInterfaces.ModuleConfig {
	return &CoursesModuleConfig{}
}

func (c *CoursesModuleConfig) GetModuleName() string {
	return "courses"
}

func (c *CoursesModuleConfig) GetFeatures() []models.FeatureDefinition {
	return []models.FeatureDefinition{
		{
			Key:         "course_conception",
			Name:        "Course Generation",
			Description: "Enable/disable course generation and management features including Marp and Slidev engines",
			Enabled:     true,
			Category:    "modules",
			Module:      "courses",
		},
	}
}
