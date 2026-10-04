package terminalTrainer_tests

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/terminalTrainer/services"
)

// A feature's minimum size travels with it to the session options, so the
// launcher can say "needs size M" instead of only "size_too_small". It is the
// catalogue's own value, not a second copy kept in ocf-core.
func TestComputeSessionOptions_FeatureCarriesItsCatalogueMinSize(t *testing.T) {
	distro, sizes, features := debianLikeCatalog()
	features = append(features, dto.TTFeature{Key: "docker", Name: "Docker", MinSizeKey: "m", AlwaysAvailable: true})

	opts := services.ComputeSessionOptions(distro, sizes, features, networkEnabledPlan())

	docker := featureOption(t, opts, "docker")
	assert.Equal(t, "m", docker.MinSizeKey)

	raw, err := json.Marshal(docker)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"min_size_key":"m"`)

	raw, err = json.Marshal(featureOption(t, opts, "network"))
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "min_size_key", "a feature with no minimum says nothing")
}
