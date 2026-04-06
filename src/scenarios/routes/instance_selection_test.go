package scenarioController

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"soli/formations/src/scenarios/models"
	terminalDto "soli/formations/src/terminalTrainer/dto"
)

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
