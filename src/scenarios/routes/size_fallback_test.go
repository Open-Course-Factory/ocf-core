package scenarioController

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"soli/formations/src/scenarios/models"
	terminalDto "soli/formations/src/terminalTrainer/dto"
)

// --- Size fallback tests (resolveDistribution with sizes catalog) ---

func TestResolveDistribution_ValidSize_ReturnsAsIs(t *testing.T) {
	scenario := models.Scenario{OsType: "deb", InstanceType: "M"}
	distributions := []terminalDto.TTDistribution{
		{Name: "debian", OsType: "deb", DefaultSizeKey: "S"},
	}
	sizes := []terminalDto.TTSize{
		{Key: "XS", SortOrder: 1},
		{Key: "S", SortOrder: 2},
		{Key: "M", SortOrder: 3},
	}
	distName, size, _, err := resolveDistribution(scenario, distributions, sizes)
	assert.NoError(t, err)
	assert.Equal(t, "debian", distName)
	assert.Equal(t, "M", size)
}

func TestResolveDistribution_UnknownSize_FallsBackToDistDefault(t *testing.T) {
	scenario := models.Scenario{OsType: "deb", InstanceType: "BOGUS"}
	distributions := []terminalDto.TTDistribution{
		{Name: "debian", OsType: "deb", DefaultSizeKey: "S"},
	}
	sizes := []terminalDto.TTSize{
		{Key: "XS", SortOrder: 1},
		{Key: "S", SortOrder: 2},
		{Key: "M", SortOrder: 3},
	}
	distName, size, _, err := resolveDistribution(scenario, distributions, sizes)
	assert.NoError(t, err)
	assert.Equal(t, "debian", distName)
	assert.Equal(t, "S", size)
}

func TestResolveDistribution_UnknownSize_NoDistDefault_FallsBackToSmallest(t *testing.T) {
	scenario := models.Scenario{OsType: "deb", InstanceType: "BOGUS"}
	distributions := []terminalDto.TTDistribution{
		{Name: "debian", OsType: "deb", DefaultSizeKey: ""},
	}
	sizes := []terminalDto.TTSize{
		{Key: "M", SortOrder: 3},
		{Key: "XS", SortOrder: 1},
		{Key: "S", SortOrder: 2},
	}
	distName, size, _, err := resolveDistribution(scenario, distributions, sizes)
	assert.NoError(t, err)
	assert.Equal(t, "debian", distName)
	assert.Equal(t, "XS", size)
}

func TestResolveDistribution_EmptyCatalog_PassThrough(t *testing.T) {
	scenario := models.Scenario{OsType: "deb", InstanceType: "BOGUS"}
	distributions := []terminalDto.TTDistribution{
		{Name: "debian", OsType: "deb", DefaultSizeKey: "S"},
	}
	distName, size, _, err := resolveDistribution(scenario, distributions, nil)
	assert.NoError(t, err)
	assert.Equal(t, "debian", distName)
	assert.Equal(t, "BOGUS", size)
}

func TestResolveDistribution_CaseInsensitiveMatch(t *testing.T) {
	scenario := models.Scenario{OsType: "deb", InstanceType: "m"}
	distributions := []terminalDto.TTDistribution{
		{Name: "debian", OsType: "deb", DefaultSizeKey: "S"},
	}
	sizes := []terminalDto.TTSize{
		{Key: "XS", SortOrder: 1},
		{Key: "S", SortOrder: 2},
		{Key: "M", SortOrder: 3},
	}
	distName, size, _, err := resolveDistribution(scenario, distributions, sizes)
	assert.NoError(t, err)
	assert.Equal(t, "debian", distName)
	assert.Equal(t, "M", size)
}
