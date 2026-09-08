package config

import (
	"os"
	"strings"
)

// FeatureFlags contains global feature toggles for the application
type FeatureFlags struct {
	CoursesEnabled   bool
	LabsEnabled      bool
	TerminalsEnabled bool
}

// GetFeatureFlagsFromDB reads feature flags from the database
// Uses the feature keys: "course_conception", "labs", "terminals"
func GetFeatureFlagsFromDB(repo interface {
	IsFeatureEnabled(key string) bool
}) FeatureFlags {
	return FeatureFlags{
		CoursesEnabled:   repo.IsFeatureEnabled("course_conception"),
		LabsEnabled:      repo.IsFeatureEnabled("labs"),
		TerminalsEnabled: repo.IsFeatureEnabled("terminals"),
	}
}

// GetEnvBool reads a boolean from environment variable
func GetEnvBool(key string, defaultValue bool) bool {
	val := strings.ToLower(os.Getenv(key))
	if val == "" {
		return defaultValue
	}
	return val == "true" || val == "1" || val == "yes"
}
