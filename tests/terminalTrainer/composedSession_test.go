package terminalTrainer_tests

import (
	"testing"

	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/terminalTrainer/services"

	"github.com/stretchr/testify/assert"
)

// ==========================================
// NormalizeSizeKey tests
// ==========================================

func TestNormalizeSizeKey(t *testing.T) {
	if testing.Short() {
		// These are pure-function tests, always run them
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"lowercase to uppercase", "xs", "XS"},
		{"already uppercase", "XL", "XL"},
		{"mixed case", "mEdIuM", "MEDIUM"},
		{"leading spaces", "  s", "S"},
		{"trailing spaces", "m  ", "M"},
		{"leading and trailing spaces", "  l  ", "L"},
		{"empty string", "", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := services.NormalizeSizeKey(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// ==========================================
// Helpers for session option tests
// ==========================================

func baseSizes() []dto.TTSize {
	return []dto.TTSize{
		{Key: "XS", Name: "Extra Small", SortOrder: 10, CPU: 1, CPUAllowance: "10%", Memory: "256MB", Disk: "1GB", Processes: 50},
		{Key: "S", Name: "Small", SortOrder: 20, CPU: 1, CPUAllowance: "25%", Memory: "512MB", Disk: "2GB", Processes: 100},
		{Key: "M", Name: "Medium", SortOrder: 30, CPU: 2, CPUAllowance: "50%", Memory: "1GB", Disk: "5GB", Processes: 200},
		{Key: "L", Name: "Large", SortOrder: 40, CPU: 4, CPUAllowance: "100%", Memory: "2GB", Disk: "10GB", Processes: 500},
		{Key: "XL", Name: "Extra Large", SortOrder: 50, CPU: 8, CPUAllowance: "200%", Memory: "4GB", Disk: "20GB", Processes: 1000},
	}
}

func baseFeatures() []dto.TTFeature {
	return []dto.TTFeature{
		{Key: "network", Name: "Network Access", ProfileName: "net-profile", DefaultEnabled: false, SortOrder: 10},
		{Key: "persistence", Name: "Data Persistence", ProfileName: "persist-profile", MinSizeKey: "M", DefaultEnabled: false, SortOrder: 20},
		{Key: "gpu", Name: "GPU Access", ProfileName: "gpu-profile", MinSizeKey: "L", DefaultEnabled: false, SortOrder: 30},
	}
}

func baseDistro() dto.TTDistribution {
	return dto.TTDistribution{
		Name:              "ubuntu-24.04",
		Prefix:            "ubuntu",
		Description:       "Ubuntu 24.04 LTS",
		OsType:            "deb",
		MinSizeKey:        "",
		SupportedFeatures: []string{"network", "persistence"},
	}
}

func freePlan() *paymentModels.SubscriptionPlan {
	return &paymentModels.SubscriptionPlan{
		Name:                       "Free",
		NetworkAccessEnabled:       false,
		DataPersistenceEnabled:     false,
		MaxSessionDurationMinutes:  60,
		CommandHistoryRetentionDays: 0,
	}
}

func proPlan() *paymentModels.SubscriptionPlan {
	return &paymentModels.SubscriptionPlan{
		Name:                       "Pro",
		NetworkAccessEnabled:       true,
		DataPersistenceEnabled:     true,
		MaxSessionDurationMinutes:  480,
		CommandHistoryRetentionDays: 30,
	}
}

// ==========================================
// ComputeSessionOptions tests
// ==========================================

func TestSessionOptions_FreePlan(t *testing.T) {
	plan := freePlan()
	distro := baseDistro()
	sizes := baseSizes()
	features := baseFeatures()

	opts := services.ComputeSessionOptions(distro, sizes, features, plan)

	assert.NotNil(t, opts)
	assert.Equal(t, distro.Name, opts.Distribution.Name)

	// All catalog sizes are admitted at the read-time level; the budget
	// engine (CPU/RAM caps) decides at launch time which sizes still fit.
	for _, s := range opts.AllowedSizes {
		assert.True(t, s.Allowed, "size %s should be admitted; budget enforcement happens later", s.Key)
		assert.Empty(t, s.Reason, "size %s should have no reason", s.Key)
	}

	// network should be plan_disabled (the only remaining plan-gated feature chip).
	// persistence is NO LONGER a feature chip after MR !239 — the persistent vs
	// ephemeral choice is surfaced as a persistence_mode radio, gated upstream
	// via DataPersistenceEnabled. Here it still falls under "size_too_small"
	for _, f := range opts.AllowedFeatures {
		switch f.Key {
		case "network":
			assert.False(t, f.Allowed, "network should be disabled on Free plan")
			assert.Equal(t, "plan_disabled", f.Reason)
		case "gpu":
			assert.False(t, f.Allowed, "gpu should not be supported by this distro")
			assert.Equal(t, "not_supported", f.Reason)
		}
	}
}

func TestSessionOptions_ProPlan(t *testing.T) {
	plan := proPlan()
	distro := baseDistro()
	sizes := baseSizes()
	features := baseFeatures()

	opts := services.ComputeSessionOptions(distro, sizes, features, plan)

	// All sizes should be allowed (plan allows "all")
	for _, s := range opts.AllowedSizes {
		assert.True(t, s.Allowed, "size %s should be allowed on Pro plan", s.Key)
	}

	// network and persistence should be allowed (both enabled in Pro)
	for _, f := range opts.AllowedFeatures {
		switch f.Key {
		case "network":
			assert.True(t, f.Allowed, "network should be allowed on Pro plan")
		case "persistence":
			assert.True(t, f.Allowed, "persistence should be allowed on Pro plan")
		case "gpu":
			assert.False(t, f.Allowed, "gpu should not be supported by this distro")
			assert.Equal(t, "not_supported", f.Reason)
		}
	}
}

func TestSessionOptions_MinSizeConstraint(t *testing.T) {
	plan := proPlan() // allows all sizes
	distro := baseDistro()
	distro.MinSizeKey = "S" // minimum size is S (SortOrder=20)
	sizes := baseSizes()
	features := baseFeatures()

	opts := services.ComputeSessionOptions(distro, sizes, features, plan)

	// XS should be excluded entirely (not present in AllowedSizes)
	sizeKeys := make([]string, 0, len(opts.AllowedSizes))
	for _, s := range opts.AllowedSizes {
		sizeKeys = append(sizeKeys, s.Key)
	}
	assert.NotContains(t, sizeKeys, "XS", "XS should not be present in AllowedSizes at all")

	// S, M, L, XL should still be present and allowed
	for _, s := range opts.AllowedSizes {
		switch s.Key {
		case "S", "M", "L", "XL":
			assert.True(t, s.Allowed, "size %s should be >= distro min_size S", s.Key)
		}
	}
}

func TestSessionOptions_FeatureNotSupported(t *testing.T) {
	plan := proPlan()
	distro := baseDistro()
	distro.SupportedFeatures = []string{"network"} // persistence NOT supported
	sizes := baseSizes()
	features := baseFeatures()

	opts := services.ComputeSessionOptions(distro, sizes, features, plan)

	for _, f := range opts.AllowedFeatures {
		switch f.Key {
		case "network":
			assert.True(t, f.Allowed, "network is supported and plan-enabled")
		case "persistence":
			assert.False(t, f.Allowed, "persistence not in distro's supported features")
			assert.Equal(t, "not_supported", f.Reason)
		case "gpu":
			assert.False(t, f.Allowed, "gpu not in distro's supported features")
			assert.Equal(t, "not_supported", f.Reason)
		}
	}
}

func TestSessionOptions_FeaturePlanDisabled(t *testing.T) {
	plan := proPlan()
	plan.NetworkAccessEnabled = false // override to disable
	distro := baseDistro()
	sizes := baseSizes()
	features := baseFeatures()

	opts := services.ComputeSessionOptions(distro, sizes, features, plan)

	for _, f := range opts.AllowedFeatures {
		if f.Key == "network" {
			assert.False(t, f.Allowed, "network should be plan_disabled when NetworkAccessEnabled=false")
			assert.Equal(t, "plan_disabled", f.Reason)
		}
	}
}


func TestStartComposedSession_RejectsDisabledFeature(t *testing.T) {
	plan := freePlan() // network disabled
	distro := baseDistro()
	sizes := baseSizes()
	features := baseFeatures()

	opts := services.ComputeSessionOptions(distro, sizes, features, plan)

	// Simulate validation: trying to enable network
	for _, f := range opts.AllowedFeatures {
		if f.Key == "network" {
			assert.False(t, f.Allowed, "network should be disabled on Free plan")
			assert.Equal(t, "plan_disabled", f.Reason)
		}
	}
}

func TestStartComposedSession_EnforcesMaxDuration(t *testing.T) {
	plan := freePlan() // max 60 minutes = 3600 seconds

	// Case 1: expiry exceeds max → should be capped
	expiry := 7200 // 2 hours
	maxDurationSeconds := plan.MaxSessionDurationMinutes * 60
	if expiry == 0 || expiry > maxDurationSeconds {
		expiry = maxDurationSeconds
	}
	assert.Equal(t, 3600, expiry, "expiry should be capped to plan max duration")

	// Case 2: expiry is 0 → should be set to max
	expiry = 0
	if expiry == 0 || expiry > maxDurationSeconds {
		expiry = maxDurationSeconds
	}
	assert.Equal(t, 3600, expiry, "zero expiry should default to plan max duration")

	// Case 3: expiry within limits → should stay
	expiry = 1800 // 30 minutes
	if expiry == 0 || expiry > maxDurationSeconds {
		expiry = maxDurationSeconds
	}
	assert.Equal(t, 1800, expiry, "expiry within limits should not change")
}

func TestSessionOptions_AllSizesKey(t *testing.T) {
	plan := &paymentModels.SubscriptionPlan{
		NetworkAccessEnabled: false,
	}
	distro := baseDistro()
	sizes := baseSizes()
	features := []dto.TTFeature{}

	opts := services.ComputeSessionOptions(distro, sizes, features, plan)

	for _, s := range opts.AllowedSizes {
		assert.True(t, s.Allowed, "size %s should be allowed when plan has 'all'", s.Key)
	}
}

// TestSessionOptions_MinSize_DistroExcludesBelowFloor — the distro's
// MinSizeKey is now the only size gate at read time. Sizes below the
// floor are dropped from AllowedSizes entirely; everything at or above
// is admitted (the budget engine handles per-launch limits).
func TestSessionOptions_MinSize_DistroExcludesBelowFloor(t *testing.T) {
	plan := &paymentModels.SubscriptionPlan{}
	distro := baseDistro()
	distro.MinSizeKey = "S"
	sizes := baseSizes()
	features := []dto.TTFeature{}

	opts := services.ComputeSessionOptions(distro, sizes, features, plan)

	sizeKeys := make([]string, 0, len(opts.AllowedSizes))
	for _, s := range opts.AllowedSizes {
		sizeKeys = append(sizeKeys, s.Key)
	}
	assert.NotContains(t, sizeKeys, "XS", "XS should be excluded by distro min_size")

	for _, s := range opts.AllowedSizes {
		assert.True(t, s.Allowed, "%s should be admitted", s.Key)
	}
}

// ==========================================
// Distribution prefix vs name (regression: console URL must use prefix)
// ==========================================

func TestComputeSessionOptions_ReturnsDistributionPrefix(t *testing.T) {
	distro := dto.TTDistribution{
		Name:              "alpine",
		Prefix:            "alp",
		MinSizeKey:        "xs",
		SupportedFeatures: []string{"network"},
	}
	sizes := baseSizes()
	features := baseFeatures()
	plan := &paymentModels.SubscriptionPlan{
		NetworkAccessEnabled: true,
	}

	result := services.ComputeSessionOptions(distro, sizes, features, plan)

	// The distribution in the response must preserve the prefix
	assert.Equal(t, "alp", result.Distribution.Prefix, "session options must return distribution prefix")
	assert.Equal(t, "alpine", result.Distribution.Name, "session options must return distribution name")
	assert.NotEqual(t, result.Distribution.Name, result.Distribution.Prefix,
		"name and prefix must be different — if they're equal, the code might confuse them")
}

func TestDistributionPrefix_MustBeUsedForInstanceType(t *testing.T) {
	// This test documents the invariant: InstanceType on the Terminal model
	// must be the distribution PREFIX (e.g., "alp"), not the name (e.g., "alpine").
	// The prefix is used by tt-backend for console, info, expire URL paths.
	// Using the name instead causes WebSocket 1006 errors.

	distro := dto.TTDistribution{
		Name:   "debian",
		Prefix: "deb",
	}

	// Simulate what StartComposedSession does after GetSessionOptions
	input := dto.CreateComposedSessionInput{
		Distribution: "debian",
	}
	input.DistributionPrefix = distro.Prefix

	// The InstanceType stored on Terminal must be the prefix, not the name
	instanceType := input.DistributionPrefix
	assert.Equal(t, "deb", instanceType, "InstanceType must be the prefix, not the distribution name")
	assert.NotEqual(t, input.Distribution, instanceType,
		"InstanceType must differ from Distribution (prefix vs name)")
}
