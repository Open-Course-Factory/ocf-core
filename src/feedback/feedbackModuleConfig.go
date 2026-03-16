package feedback

import (
	configInterfaces "soli/formations/src/configuration/interfaces"
	"soli/formations/src/configuration/models"
)

// FeedbackModuleConfig implements the ModuleConfig interface
type FeedbackModuleConfig struct{}

// NewFeedbackModuleConfig creates a new feedback module configuration
func NewFeedbackModuleConfig() configInterfaces.ModuleConfig {
	return &FeedbackModuleConfig{}
}

func (f *FeedbackModuleConfig) GetModuleName() string {
	return "feedback"
}

func (f *FeedbackModuleConfig) GetFeatures() []models.FeatureDefinition {
	return []models.FeatureDefinition{
		{
			Key:         "feedback_recipients",
			Name:        "Feedback Email Recipients",
			Description: "JSON array of email addresses that receive user feedback",
			Enabled:     true,
			Category:    "configuration",
			Module:      "feedback",
			Value:       "[]",
		},
	}
}
