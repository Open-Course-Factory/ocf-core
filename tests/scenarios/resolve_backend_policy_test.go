package scenarios_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
	ttDto "soli/formations/src/terminalTrainer/dto"
	ttServices "soli/formations/src/terminalTrainer/services"
)

// policyTTService serves a fixed backend policy and a per-backend catalog, so
// Resolve can be watched choosing between hosts.
type policyTTService struct {
	*capturingTTService
	policy  ttServices.BackendPolicy
	catalog map[string][]ttDto.TTDistribution
}

func (m *policyTTService) BackendPolicyFor(*uuid.UUID, *paymentModels.SubscriptionPlan) (ttServices.BackendPolicy, error) {
	return m.policy, nil
}

func (m *policyTTService) GetDistributions(backend string) ([]ttDto.TTDistribution, error) {
	return m.catalog[backend], nil
}

func debianScenario() models.Scenario {
	return models.Scenario{
		Name:                    "needs-debian",
		InstanceType:            "S",
		CompatibleInstanceTypes: []models.ScenarioInstanceType{{InstanceType: "debian"}},
	}
}

func newPolicyResolver(t *testing.T) *services.ScenarioProvisioningService {
	t.Helper()
	tt := &policyTTService{
		capturingTTService: newCapturingTTService(),
		// "a" is the default but lacks the image; only "b" has it.
		policy:  ttServices.BackendPolicy{Allowed: []string{"a", "b"}, Default: "a"},
		catalog: map[string][]ttDto.TTDistribution{"b": {{Name: "debian", DefaultSizeKey: "S"}}},
	}
	return services.NewScenarioProvisioningService(freshTestDB(t), tt)
}

func TestResolve_Auto_TriesEveryAllowedBackend(t *testing.T) {
	got, err := newPolicyResolver(t).Resolve(debianScenario(), nil, nil, "")
	require.NoError(t, err)
	assert.Equal(t, "b", got.Backend)
}

func TestResolve_ExplicitBackend_IsNotSwapped(t *testing.T) {
	_, err := newPolicyResolver(t).Resolve(debianScenario(), nil, nil, "a")
	require.Error(t, err, "the chosen host lacks the image: say so rather than build elsewhere")
	assert.NotErrorIs(t, err, ttServices.ErrBackendNotAllowed)
}

func TestResolve_ExplicitBackend_OutsidePolicy_IsRefused(t *testing.T) {
	_, err := newPolicyResolver(t).Resolve(debianScenario(), nil, nil, "elsewhere")
	assert.ErrorIs(t, err, ttServices.ErrBackendNotAllowed)
}

// rankedTTService ranks backends in reverse policy order, standing in for a
// default host that is busier than the other.
type rankedTTService struct{ *policyTTService }

func (m *rankedTTService) RankBackends(candidates []string) []string {
	ranked := make([]string, 0, len(candidates))
	for i := len(candidates) - 1; i >= 0; i-- {
		ranked = append(ranked, candidates[i])
	}
	return ranked
}

func TestResolve_Auto_FollowsHostRanking(t *testing.T) {
	debian := []ttDto.TTDistribution{{Name: "debian", DefaultSizeKey: "S"}}
	tt := &rankedTTService{&policyTTService{
		capturingTTService: newCapturingTTService(),
		policy:             ttServices.BackendPolicy{Allowed: []string{"a", "b"}, Default: "a"},
		catalog:            map[string][]ttDto.TTDistribution{"a": debian, "b": debian},
	}}

	got, err := services.NewScenarioProvisioningService(freshTestDB(t), tt).Resolve(debianScenario(), nil, nil, "")
	require.NoError(t, err)
	assert.Equal(t, "b", got.Backend, "both hosts fit: the less busy one wins over the default")
}
