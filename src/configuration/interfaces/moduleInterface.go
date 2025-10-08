package interfaces

import "soli/formations/src/configuration/models"

// ModuleConfig defines the interface that each module should implement
// to declare its features and configuration
type ModuleConfig interface {
	// GetModuleName returns the unique name of the module
	GetModuleName() string

	// GetFeatures returns the list of features this module provides
	GetFeatures() []models.FeatureDefinition
}
