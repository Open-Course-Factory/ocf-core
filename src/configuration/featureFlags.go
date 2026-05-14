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
		CoursesEnabled:   getEnvBool("FEATURE_COURSES_ENABLED", true),
		LabsEnabled:      getEnvBool("FEATURE_LABS_ENABLED", true),
		TerminalsEnabled: getEnvBool("FEATURE_TERMINALS_ENABLED", true),
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

// getEnvBool reads a boolean from environment variable
func getEnvBool(key string, defaultValue bool) bool {
	val := strings.ToLower(os.Getenv(key))
	if val == "" {
		return defaultValue
	}
	return val == "true" || val == "1" || val == "yes"
}

// IsBudgetQuotasEnabled reports whether the CPU/RAM budget quota model is
// active for plan-gated terminal/scenario flows. When false (the default),
// every path that consults this flag must fall back to the legacy slot-based
// quota behaviour (MaxConcurrentTerminals + AllowedMachineSizes).
//
// The flag is read from the OCF_FEATURE_BUDGET_QUOTAS environment variable
// so the feature can be toggled per-deployment without a DB migration. The
// underlying SubscriptionPlan.QuotaModel = "budget" rows have no effect
// until this flag flips on — production rollout proceeds in two safe steps:
//
//  1. Run the backfill (CORE-3) so plans carry MaxCPU / MaxMemoryMB.
//  2. Flip OCF_FEATURE_BUDGET_QUOTAS=1 to start enforcing.
//
// CI tests that need to exercise the budget path set the env var explicitly.
func IsBudgetQuotasEnabled() bool {
	return getEnvBool("OCF_FEATURE_BUDGET_QUOTAS", false)
}
