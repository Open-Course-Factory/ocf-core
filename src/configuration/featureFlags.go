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

// FeatureFlagsProvider is the interface for getting feature flags
type FeatureFlagsProvider interface {
	GetFeatureFlags() FeatureFlags
}

// GetFeatureFlags reads feature flags from environment variables
// Defaults to all features enabled if not specified
// This is the fallback method when database is not available
func GetFeatureFlags() FeatureFlags {
	return FeatureFlags{
		CoursesEnabled:   GetEnvBool("FEATURE_COURSES_ENABLED", false),
		LabsEnabled:      GetEnvBool("FEATURE_LABS_ENABLED", true),
		TerminalsEnabled: GetEnvBool("FEATURE_TERMINALS_ENABLED", true),
	}
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
