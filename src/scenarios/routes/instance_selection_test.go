package scenarioController

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	entityManagementModels "soli/formations/src/entityManagement/models"
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

// --- CompatibleInstanceTypes tests ---

func makeScenarioInstanceType(scenarioID uuid.UUID, instanceType string, osType string, priority int) models.ScenarioInstanceType {
	return models.ScenarioInstanceType{
		BaseModel:    entityManagementModels.BaseModel{ID: uuid.New()},
		ScenarioID:   scenarioID,
		InstanceType: instanceType,
		OsType:       osType,
		Priority:     priority,
	}
}

func TestResolveDistribution_CompatibleInstanceTypes_MatchByName(t *testing.T) {
	scenarioID := uuid.New()
	scenario := models.Scenario{
		OsType: "deb",
		CompatibleInstanceTypes: []models.ScenarioInstanceType{
			makeScenarioInstanceType(scenarioID, "rogueLite", "", 0),
		},
	}
	scenario.BaseModel = entityManagementModels.BaseModel{ID: scenarioID}

	distributions := []terminalDto.TTDistribution{
		{Name: "debian", OsType: "deb", DefaultSizeKey: "s"},
		{Name: "rogueLite", OsType: "debian", DefaultSizeKey: "m"},
	}

	distName, size, _, err := resolveDistribution(scenario, distributions)
	assert.NoError(t, err)
	assert.Equal(t, "rogueLite", distName, "should match by CompatibleInstanceTypes name, not by OsType")
	assert.Equal(t, "m", size, "should use distribution's DefaultSizeKey when scenario has no InstanceType")
}

func TestResolveDistribution_CompatibleInstanceTypes_Priority(t *testing.T) {
	scenarioID := uuid.New()
	scenario := models.Scenario{
		CompatibleInstanceTypes: []models.ScenarioInstanceType{
			makeScenarioInstanceType(scenarioID, "ubuntu", "", 10),
			makeScenarioInstanceType(scenarioID, "rogueLite", "", 1),
		},
	}
	scenario.BaseModel = entityManagementModels.BaseModel{ID: scenarioID}

	distributions := []terminalDto.TTDistribution{
		{Name: "ubuntu", OsType: "deb", DefaultSizeKey: "s"},
		{Name: "rogueLite", OsType: "debian", DefaultSizeKey: "m"},
	}

	distName, size, _, err := resolveDistribution(scenario, distributions)
	assert.NoError(t, err)
	assert.Equal(t, "rogueLite", distName, "lower priority number should be selected first")
	assert.Equal(t, "m", size)
}

func TestResolveDistribution_CompatibleInstanceTypes_FallbackToOsType(t *testing.T) {
	scenarioID := uuid.New()
	scenario := models.Scenario{
		OsType: "deb",
		CompatibleInstanceTypes: []models.ScenarioInstanceType{
			makeScenarioInstanceType(scenarioID, "nonexistent", "", 0),
		},
	}
	scenario.BaseModel = entityManagementModels.BaseModel{ID: scenarioID}

	distributions := []terminalDto.TTDistribution{
		{Name: "debian", OsType: "deb", DefaultSizeKey: "s"},
	}

	distName, size, _, err := resolveDistribution(scenario, distributions)
	assert.NoError(t, err)
	assert.Equal(t, "debian", distName, "should fall back to OsType matching when no CompatibleInstanceTypes match")
	assert.Equal(t, "s", size)
}

func TestResolveDistribution_CompatibleInstanceTypes_Empty(t *testing.T) {
	scenario := models.Scenario{
		OsType:                  "apk",
		CompatibleInstanceTypes: []models.ScenarioInstanceType{},
	}

	distributions := []terminalDto.TTDistribution{
		{Name: "alpine", OsType: "apk", DefaultSizeKey: "xs"},
	}

	distName, size, _, err := resolveDistribution(scenario, distributions)
	assert.NoError(t, err)
	assert.Equal(t, "alpine", distName, "empty CompatibleInstanceTypes should use normal OsType matching")
	assert.Equal(t, "xs", size)
}

func TestResolveDistribution_CompatibleInstanceTypes_UsesScenarioSize(t *testing.T) {
	scenarioID := uuid.New()
	scenario := models.Scenario{
		InstanceType: "L",
		CompatibleInstanceTypes: []models.ScenarioInstanceType{
			makeScenarioInstanceType(scenarioID, "rogueLite", "", 0),
		},
	}
	scenario.BaseModel = entityManagementModels.BaseModel{ID: scenarioID}

	distributions := []terminalDto.TTDistribution{
		{Name: "rogueLite", OsType: "debian", DefaultSizeKey: "m"},
	}

	distName, size, _, err := resolveDistribution(scenario, distributions)
	assert.NoError(t, err)
	assert.Equal(t, "rogueLite", distName)
	assert.Equal(t, "L", size, "scenario's InstanceType should take precedence over distribution's DefaultSizeKey")
}
