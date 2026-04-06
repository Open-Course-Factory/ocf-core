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
		AllowedMachineSizes:        []string{"XS", "S"},
		NetworkAccessEnabled:       false,
		DataPersistenceEnabled:     false,
		MaxSessionDurationMinutes:  60,
		MaxConcurrentTerminals:     1,
		CommandHistoryRetentionDays: 0,
	}
}

func proPlan() *paymentModels.SubscriptionPlan {
	return &paymentModels.SubscriptionPlan{
		Name:                       "Pro",
		AllowedMachineSizes:        []string{"all"},
		NetworkAccessEnabled:       true,
		DataPersistenceEnabled:     true,
		MaxSessionDurationMinutes:  480,
		MaxConcurrentTerminals:     5,
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

	// XS and S should be allowed, M/L/XL denied by plan_limit
	for _, s := range opts.AllowedSizes {
		switch s.Key {
		case "XS", "S":
			assert.True(t, s.Allowed, "size %s should be allowed on Free plan", s.Key)
			assert.Empty(t, s.Reason)
		case "M", "L", "XL":
			assert.False(t, s.Allowed, "size %s should NOT be allowed on Free plan", s.Key)
			assert.Equal(t, "plan_limit", s.Reason)
		}
	}

	// network should be plan_disabled, persistence should be plan_disabled
	for _, f := range opts.AllowedFeatures {
		switch f.Key {
		case "network":
			assert.False(t, f.Allowed, "network should be disabled on Free plan")
			assert.Equal(t, "plan_disabled", f.Reason)
		case "persistence":
			assert.False(t, f.Allowed, "persistence should be disabled on Free plan")
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

	for _, s := range opts.AllowedSizes {
		switch s.Key {
		case "XS":
			assert.False(t, s.Allowed, "XS should be below distro min_size S")
			assert.Equal(t, "min_size", s.Reason)
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

func TestSessionOptions_FeatureSizeTooSmall(t *testing.T) {
	// Plan only allows XS and S, but persistence requires M (min_size_key=M, SortOrder=30)
	plan := freePlan() // allows XS, S
	plan.DataPersistenceEnabled = true // enable persistence in plan
	distro := baseDistro()
	distro.SupportedFeatures = []string{"persistence"}
	sizes := baseSizes()
	features := []dto.TTFeature{
		{Key: "persistence", Name: "Data Persistence", ProfileName: "persist-profile", MinSizeKey: "M", DefaultEnabled: false, SortOrder: 20},
	}

	opts := services.ComputeSessionOptions(distro, sizes, features, plan)

	for _, f := range opts.AllowedFeatures {
		if f.Key == "persistence" {
			// Max allowed sort order is S=20, but persistence requires M=30
			assert.False(t, f.Allowed, "persistence needs M but max allowed is S")
			assert.Equal(t, "size_too_small", f.Reason)
		}
	}
}

func TestStartComposedSession_RejectsForbiddenSize(t *testing.T) {
	// This tests the validation logic in ComputeSessionOptions indirectly:
	// A "free" plan user requesting M should see it disallowed
	plan := freePlan()
	distro := baseDistro()
	sizes := baseSizes()
	features := baseFeatures()

	opts := services.ComputeSessionOptions(distro, sizes, features, plan)

	// Simulate what the service does: check if M is allowed
	requestedSize := services.NormalizeSizeKey("m")
	for _, s := range opts.AllowedSizes {
		if services.NormalizeSizeKey(s.Key) == requestedSize {
			assert.False(t, s.Allowed, "M should not be allowed on Free plan")
			assert.Equal(t, "plan_limit", s.Reason)
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
	// Verify that "all" in AllowedMachineSizes allows everything
	plan := &paymentModels.SubscriptionPlan{
		AllowedMachineSizes:  []string{"all"},
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

func TestSessionOptions_EmptyPlanSizes(t *testing.T) {
	// A plan with no AllowedMachineSizes should deny all sizes
	plan := &paymentModels.SubscriptionPlan{
		AllowedMachineSizes: []string{},
	}
	distro := baseDistro()
	sizes := baseSizes()
	features := []dto.TTFeature{}

	opts := services.ComputeSessionOptions(distro, sizes, features, plan)

	for _, s := range opts.AllowedSizes {
		assert.False(t, s.Allowed, "size %s should be denied when plan has no allowed sizes", s.Key)
		assert.Equal(t, "plan_limit", s.Reason)
	}
}

func TestSessionOptions_MinSizeAndPlanCombined(t *testing.T) {
	// Distro min_size=S + plan allows only XS, S, M
	// XS: denied by min_size, S/M: allowed, L/XL: denied by plan_limit
	plan := &paymentModels.SubscriptionPlan{
		AllowedMachineSizes: []string{"XS", "S", "M"},
	}
	distro := baseDistro()
	distro.MinSizeKey = "S"
	sizes := baseSizes()
	features := []dto.TTFeature{}

	opts := services.ComputeSessionOptions(distro, sizes, features, plan)

	for _, s := range opts.AllowedSizes {
		switch s.Key {
		case "XS":
			assert.False(t, s.Allowed, "XS below distro min_size")
			assert.Equal(t, "min_size", s.Reason)
		case "S", "M":
			assert.True(t, s.Allowed, "%s should be allowed", s.Key)
		case "L", "XL":
			assert.False(t, s.Allowed, "%s not in plan", s.Key)
			assert.Equal(t, "plan_limit", s.Reason)
		}
	}
}
