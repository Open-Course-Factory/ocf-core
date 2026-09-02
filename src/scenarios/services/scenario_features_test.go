package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/scenarios/models"
	terminalDto "soli/formations/src/terminalTrainer/dto"
)

// A scenario that configures a banner must arrive on a machine able to draw it.
// Asserted through resolveDistribution rather than on the helper alone, because
// the point of deriving it here is that every caller gets it without having to
// remember — the launch and the preview both read this return value.
func TestResolveDistribution_ScenarioWithABanner_AsksForTheRenderer(t *testing.T) {
	t.Setenv("FEATURE_SCENARIO_EFFECTS_ENABLED", "true")
	scenario := models.Scenario{
		OsType:       "deb",
		InstanceType: "M",
		Steps: []models.ScenarioStep{
			{IntroEffect: "beams", IntroText: "Niveau 2 débloqué"},
		},
	}
	distributions := []terminalDto.TTDistribution{{Name: "debian", OsType: "deb", DefaultSizeKey: "S"}}
	sizes := []terminalDto.TTSize{{Key: "M", SortOrder: 3}}

	_, _, features, err := resolveDistribution(scenario, distributions, sizes)

	require.NoError(t, err)
	assert.True(t, features["effects"],
		"a configured banner cannot be drawn without the renderer, so the machine has to be asked for it")
}

// The renderer is not requested for scenarios that never animate anything: it
// costs an install on any image that lacks it.
func TestResolveDistribution_ScenarioWithoutBanners_AsksForNothingExtra(t *testing.T) {
	scenario := models.Scenario{
		OsType:       "deb",
		InstanceType: "M",
		Steps:        []models.ScenarioStep{{Title: "plain step"}},
	}
	distributions := []terminalDto.TTDistribution{{Name: "debian", OsType: "deb", DefaultSizeKey: "S"}}
	sizes := []terminalDto.TTSize{{Key: "M", SortOrder: 3}}

	_, _, features, err := resolveDistribution(scenario, distributions, sizes)

	require.NoError(t, err)
	assert.False(t, features["effects"])
}

// With effects off — the default — a banner-carrying scenario must not drag the
// renderer along: installing it is the ~25s the switch exists to save.
func TestResolveDistribution_EffectsDisabled_DoesNotAskForTheRenderer(t *testing.T) {
	scenario := models.Scenario{
		OsType:       "deb",
		InstanceType: "M",
		Steps: []models.ScenarioStep{
			{IntroEffect: "beams", IntroText: "Niveau 2 débloqué"},
		},
	}
	distributions := []terminalDto.TTDistribution{{Name: "debian", OsType: "deb", DefaultSizeKey: "S"}}
	sizes := []terminalDto.TTSize{{Key: "M", SortOrder: 3}}

	_, _, features, err := resolveDistribution(scenario, distributions, sizes)

	require.NoError(t, err)
	assert.False(t, features["effects"])
}
