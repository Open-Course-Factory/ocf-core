// tests/terminalTrainer/alwaysAvailableFeature_test.go
//
// A feature the image does not have to declare.
//
// tt-backend's catalog distinguishes two kinds of feature. Most attach a
// device or a profile and only work on images prepared for them, so an image
// lists them in supported_features. A few install their own software into the
// container at session start and therefore work anywhere — tt-backend marks
// those always_available, precisely so that every image does not have to be
// edited before the capability can be used.
//
// 'effects' (the scenario banner renderer) is one of those. ocf-core did not
// mirror the flag, so it judged every feature against the image's
// supported_features alone and refused to launch any scenario with an intro or
// outro banner: "feature 'effects' is not allowed: not_supported", seen in
// production on Debian, whose list is ["network"].
package terminalTrainer_tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	entityManagementModels "soli/formations/src/entityManagement/models"
	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/terminalTrainer/services"

	"github.com/google/uuid"
)

// featureOption returns the computed option for a feature key.
func featureOption(t *testing.T, opts *dto.SessionOptionsResponse, key string) *dto.SessionOptionFeature {
	t.Helper()
	for i := range opts.AllowedFeatures {
		if opts.AllowedFeatures[i].Key == key {
			return &opts.AllowedFeatures[i]
		}
	}
	t.Fatalf("feature %q not in response", key)
	return nil
}

// debianLikeCatalog mirrors production: an image declaring only "network",
// and a catalog holding both a device-backed feature and a self-installing one.
func debianLikeCatalog() (dto.TTDistribution, []dto.TTSize, []dto.TTFeature) {
	distro := dto.TTDistribution{Name: "Debian", OsType: "deb", SupportedFeatures: []string{"network"}}
	sizes := []dto.TTSize{
		{Key: "xs", SortOrder: 10},
		{Key: "s", SortOrder: 20},
	}
	features := []dto.TTFeature{
		{Key: "network", Name: "Network Access"},
		{Key: "effects", Name: "Terminal Effects", AlwaysAvailable: true},
	}
	return distro, sizes, features
}

// networkEnabledPlan is the plan these cases run under.
func networkEnabledPlan() *paymentModels.SubscriptionPlan {
	return &paymentModels.SubscriptionPlan{
		BaseModel:            entityManagementModels.BaseModel{ID: uuid.New()},
		Name:                 "FeatureCatalogTest",
		NetworkAccessEnabled: true,
	}
}

func TestComputeSessionOptions_AlwaysAvailableFeatureNeedsNoImageSupport(t *testing.T) {
	distro, sizes, features := debianLikeCatalog()

	opts := services.ComputeSessionOptions(distro, sizes, features, networkEnabledPlan())

	effects := featureOption(t, opts, "effects")
	require.True(t, effects.Allowed,
		"an always_available feature installs itself into the container, so an "+
			"image that never listed it must still offer it — otherwise every "+
			"scenario with a banner is unlaunchable. Reason given: %q", effects.Reason)
	assert.Empty(t, effects.Reason)
}

func TestComputeSessionOptions_OrdinaryFeatureStillNeedsImageSupport(t *testing.T) {
	distro, sizes, features := debianLikeCatalog()
	// An ordinary, device-backed feature this image does not declare.
	features = append(features, dto.TTFeature{Key: "docker", Name: "Docker"})

	opts := services.ComputeSessionOptions(distro, sizes, features, networkEnabledPlan())

	assert.True(t, featureOption(t, opts, "network").Allowed,
		"the image declares network")

	docker := featureOption(t, opts, "docker")
	assert.False(t, docker.Allowed,
		"a feature that needs image support must stay refused when the image "+
			"does not declare it — always_available is the exception, not the rule")
	assert.Equal(t, "not_supported", docker.Reason)
}

// A user can reach GET /terminals/session-options with a resolved effective
// plan whose Plan is nil — InjectEffectivePlan stores result.Plan verbatim, and
// the launch path guards against exactly that shape. featurePlanMapping's
// predicates dereference the plan, so an unguarded nil crashed the request
// instead of answering it.
func TestComputeSessionOptions_NilPlanRefusesPlanGatedFeaturesInsteadOfPanicking(t *testing.T) {
	distro, sizes, features := debianLikeCatalog()

	var opts *dto.SessionOptionsResponse
	require.NotPanics(t, func() {
		opts = services.ComputeSessionOptions(distro, sizes, features, nil)
	}, "no plan is an answerable state, not a crash")

	network := featureOption(t, opts, "network")
	assert.False(t, network.Allowed,
		"a feature gated on a plan predicate cannot be granted without a plan")
	assert.Equal(t, "plan_disabled", network.Reason)

	assert.True(t, featureOption(t, opts, "effects").Allowed,
		"a feature that needs no plan and no image support stays available")
}
