package scenarioController

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"soli/formations/src/scenarios/models"
	terminalDto "soli/formations/src/terminalTrainer/dto"
)

func TestFindBestInstanceType_PrefersSmallestSize(t *testing.T) {
	scenario := models.Scenario{
		OsType:       "deb",
		InstanceType: "XS",
	}

	available := []terminalDto.InstanceType{
		{Name: "ubuntu-s", Prefix: "ubu-s", Size: "S", OsType: "deb"},
		{Name: "ubuntu-xs", Prefix: "ubu-xs", Size: "XS", OsType: "deb"},
	}

	result := findBestInstanceType(scenario, available)
	assert.Equal(t, "ubu-xs", result, "should pick the XS instance, not the S one")
}

func TestFindBestInstanceType_FallbackUnknownSize(t *testing.T) {
	scenario := models.Scenario{
		OsType:       "",
		InstanceType: "",
	}

	available := []terminalDto.InstanceType{
		{Name: "custom-box", Prefix: "cust", Size: "custom", OsType: "deb"},
	}

	result := findBestInstanceType(scenario, available)
	assert.Equal(t, "cust", result, "should fall back to first compatible match when size is unknown")
}

func TestFindBestInstanceType_NoMatch(t *testing.T) {
	scenario := models.Scenario{
		OsType:       "deb",
		InstanceType: "L",
	}

	available := []terminalDto.InstanceType{
		{Name: "tiny", Prefix: "tiny", Size: "XS", OsType: "deb"},
		{Name: "small", Prefix: "small", Size: "S", OsType: "deb"},
	}

	result := findBestInstanceType(scenario, available)
	assert.Equal(t, "", result, "should return empty when no instance meets the size requirement")
}

func TestFindBestInstanceType_EmptyRequiredSize(t *testing.T) {
	scenario := models.Scenario{
		OsType:       "deb",
		InstanceType: "", // no size requirement
	}

	available := []terminalDto.InstanceType{
		{Name: "large", Prefix: "lrg", Size: "L", OsType: "deb"},
		{Name: "medium", Prefix: "med", Size: "M", OsType: "deb"},
		{Name: "small", Prefix: "sml", Size: "S", OsType: "deb"},
	}

	result := findBestInstanceType(scenario, available)
	assert.Equal(t, "sml", result, "should return smallest available instance when no size is required")
}

func TestFindBestInstanceType_MultiSizeInstance(t *testing.T) {
	// Test instance that supports multiple sizes (pipe-separated)
	scenario := models.Scenario{
		OsType:       "deb",
		InstanceType: "M",
	}

	available := []terminalDto.InstanceType{
		{Name: "flex", Prefix: "flex", Size: "S|M|L", OsType: "deb"},
		{Name: "big", Prefix: "big", Size: "XL", OsType: "deb"},
	}

	result := findBestInstanceType(scenario, available)
	// "flex" supports M which is >= required M. "big" supports XL >= M.
	// Among compatible sizes: flex offers M (order 3), big offers XL (order 5).
	// Should pick flex with M since it's the smallest meeting the requirement.
	assert.Equal(t, "flex", result, "should pick the instance with the smallest compatible size")
}

func TestFindBestInstanceType_OsTypeMismatch(t *testing.T) {
	scenario := models.Scenario{
		OsType:       "rpm",
		InstanceType: "S",
	}

	available := []terminalDto.InstanceType{
		{Name: "debian", Prefix: "deb", Size: "S", OsType: "deb"},
		{Name: "fedora", Prefix: "fed", Size: "M", OsType: "rpm"},
	}

	result := findBestInstanceType(scenario, available)
	assert.Equal(t, "fed", result, "should skip OS type mismatches and pick the compatible one")
}

func TestInstanceMatchesScenario_BasicMatch(t *testing.T) {
	inst := terminalDto.InstanceType{
		OsType: "deb",
		Size:   "M",
	}

	assert.True(t, instanceMatchesScenario(inst, "deb", "M"))
	assert.True(t, instanceMatchesScenario(inst, "deb", "S"))  // M >= S
	assert.False(t, instanceMatchesScenario(inst, "deb", "L")) // M < L
	assert.False(t, instanceMatchesScenario(inst, "rpm", "M")) // wrong OS
	assert.True(t, instanceMatchesScenario(inst, "", "M"))      // no OS requirement
	assert.True(t, instanceMatchesScenario(inst, "", ""))        // no requirements
}

func TestInstanceMatchesScenario_PipeSeparatedSizes(t *testing.T) {
	inst := terminalDto.InstanceType{
		OsType: "deb",
		Size:   "S|M|L",
	}

	assert.True(t, instanceMatchesScenario(inst, "deb", "S"))
	assert.True(t, instanceMatchesScenario(inst, "deb", "M"))
	assert.True(t, instanceMatchesScenario(inst, "deb", "L"))
	assert.False(t, instanceMatchesScenario(inst, "deb", "XL")) // none of S|M|L >= XL
}

// --- resolveDistribution tests ---

func TestResolveDistribution_MatchesOsType(t *testing.T) {
	scenario := models.Scenario{OsType: "deb", InstanceType: "M"}
	distributions := []terminalDto.TTDistribution{
		{Name: "alpine", Prefix: "alp", OsType: "apk"},
		{Name: "debian", Prefix: "deb", OsType: "deb", MinSizeKey: "xs", DefaultSizeKey: "s"},
	}
	distName, size, _, err := resolveDistribution(scenario, distributions)
	assert.NoError(t, err)
	assert.Equal(t, "debian", distName)
	assert.Equal(t, "M", size)
}

func TestResolveDistribution_NoMatch(t *testing.T) {
	scenario := models.Scenario{OsType: "rpm", InstanceType: "M"}
	distributions := []terminalDto.TTDistribution{
		{Name: "debian", Prefix: "deb", OsType: "deb"},
	}
	_, _, _, err := resolveDistribution(scenario, distributions)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no compatible distribution")
}

func TestResolveDistribution_FeatureRequirement(t *testing.T) {
	scenario := models.Scenario{
		OsType:           "deb",
		InstanceType:     "M",
		RequiredFeatures: `["network"]`,
	}
	distributions := []terminalDto.TTDistribution{
		{Name: "debian-basic", OsType: "deb", SupportedFeatures: nil},
		{Name: "debian-full", OsType: "deb", SupportedFeatures: []string{"network", "persistence"}},
	}
	distName, _, features, err := resolveDistribution(scenario, distributions)
	assert.NoError(t, err)
	assert.Equal(t, "debian-full", distName)
	assert.True(t, features["network"])
}

func TestResolveDistribution_FeatureNotSupported(t *testing.T) {
	scenario := models.Scenario{
		OsType:           "deb",
		InstanceType:     "M",
		RequiredFeatures: `["gpu"]`,
	}
	distributions := []terminalDto.TTDistribution{
		{Name: "debian", OsType: "deb", SupportedFeatures: []string{"network"}},
	}
	_, _, _, err := resolveDistribution(scenario, distributions)
	assert.Error(t, err)
}

func TestResolveDistribution_MinSizeRespected(t *testing.T) {
	scenario := models.Scenario{OsType: "deb", InstanceType: "XS"}
	distributions := []terminalDto.TTDistribution{
		{Name: "debian-heavy", OsType: "deb", MinSizeKey: "M"},  // requires at least M
		{Name: "debian-light", OsType: "deb", MinSizeKey: "XS"}, // XS is fine
	}
	distName, _, _, err := resolveDistribution(scenario, distributions)
	assert.NoError(t, err)
	assert.Equal(t, "debian-light", distName)
}

func TestResolveDistribution_EmptyFeatures(t *testing.T) {
	scenario := models.Scenario{OsType: "apk", InstanceType: "S"}
	distributions := []terminalDto.TTDistribution{
		{Name: "alpine", OsType: "apk", SupportedFeatures: []string{"network"}},
	}
	distName, _, features, err := resolveDistribution(scenario, distributions)
	assert.NoError(t, err)
	assert.Equal(t, "alpine", distName)
	assert.Nil(t, features) // no features required
}

func TestResolveDistribution_DefaultSize(t *testing.T) {
	scenario := models.Scenario{OsType: "deb", InstanceType: ""}
	distributions := []terminalDto.TTDistribution{
		{Name: "debian", OsType: "deb", DefaultSizeKey: "M"},
	}
	distName, size, _, err := resolveDistribution(scenario, distributions)
	assert.NoError(t, err)
	assert.Equal(t, "debian", distName)
	assert.Equal(t, "M", size)
}

// --- distributionSupportsFeatures tests ---

func TestResolveDistribution_InvalidJSON(t *testing.T) {
	scenario := models.Scenario{
		OsType:           "deb",
		InstanceType:     "M",
		RequiredFeatures: "docker,python3", // invalid JSON
	}
	distributions := []terminalDto.TTDistribution{
		{Name: "debian", OsType: "deb"},
	}
	_, _, _, err := resolveDistribution(scenario, distributions)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid")
}

func TestDistributionSupportsFeatures_AllPresent(t *testing.T) {
	dist := terminalDto.TTDistribution{SupportedFeatures: []string{"network", "persistence"}}
	assert.True(t, distributionSupportsFeatures(dist, []string{"network"}))
	assert.True(t, distributionSupportsFeatures(dist, []string{"network", "persistence"}))
}

func TestDistributionSupportsFeatures_Missing(t *testing.T) {
	dist := terminalDto.TTDistribution{SupportedFeatures: []string{"network"}}
	assert.False(t, distributionSupportsFeatures(dist, []string{"persistence"}))
	assert.False(t, distributionSupportsFeatures(dist, []string{"network", "gpu"}))
}

func TestDistributionSupportsFeatures_Empty(t *testing.T) {
	dist := terminalDto.TTDistribution{SupportedFeatures: nil}
	assert.True(t, distributionSupportsFeatures(dist, nil))
	assert.True(t, distributionSupportsFeatures(dist, []string{}))
}
