package config

// DefaultRetentionDays is how long a departed member's data is kept before the
// daily erasure job removes it, for organizations that have not set their own
// retention_days.
//
// This is configuration, not a legal figure: the organization is the data
// controller and sets the real period on Organization.RetentionDays. The
// platform default only covers organizations that never chose one.
func DefaultRetentionDays() int {
	return getIntEnv("OCF_DEFAULT_RETENTION_DAYS", 365)
}

// ErasureMaxPerRun caps how many members one run of the erasure job may erase.
// Erasure is irreversible, so an unexpected spike (a bad retention edit, a
// clock jump) is logged and refused rather than executed.
func ErasureMaxPerRun() int {
	return getIntEnv("OCF_ERASURE_MAX_PER_RUN", 50)
}
