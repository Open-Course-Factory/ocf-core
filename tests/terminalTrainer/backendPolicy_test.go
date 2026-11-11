package terminalTrainer_tests

import (
	"testing"

	organizationModels "soli/formations/src/organizations/models"
	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/terminalTrainer/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBackendPolicy_SourceAndDefault(t *testing.T) {
	cases := []struct {
		name        string
		org         *organizationModels.Organization
		plan        *paymentModels.SubscriptionPlan
		system      string
		wantAllowed []string
		wantDefault string
	}{
		{
			name:        "nothing configured → system default only",
			system:      "sys",
			wantAllowed: []string{"sys"},
			wantDefault: "sys",
		},
		{
			name:        "org settings win over plan settings",
			org:         &organizationModels.Organization{AllowedBackends: []string{"a", "b"}, DefaultBackend: "b"},
			plan:        &paymentModels.SubscriptionPlan{AllowedBackends: []string{"p"}, DefaultBackend: "p"},
			system:      "sys",
			wantAllowed: []string{"b", "a"},
			wantDefault: "b",
		},
		{
			name:        "org without settings → plan settings apply",
			org:         &organizationModels.Organization{},
			plan:        &paymentModels.SubscriptionPlan{AllowedBackends: []string{"p1", "p2"}, DefaultBackend: "p2"},
			system:      "sys",
			wantAllowed: []string{"p2", "p1"},
			wantDefault: "p2",
		},
		{
			name:        "default only → that backend alone",
			org:         &organizationModels.Organization{DefaultBackend: "a"},
			system:      "sys",
			wantAllowed: []string{"a"},
			wantDefault: "a",
		},
		{
			name:        "default outside the allowed list is never used",
			org:         &organizationModels.Organization{AllowedBackends: []string{"a", "b"}, DefaultBackend: "z"},
			system:      "sys",
			wantAllowed: []string{"a", "b"},
			wantDefault: "a",
		},
		{
			name:        "no org default → system default when it is allowed",
			org:         &organizationModels.Organization{AllowedBackends: []string{"a", "sys"}},
			system:      "sys",
			wantAllowed: []string{"sys", "a"},
			wantDefault: "sys",
		},
		{
			name:        "duplicates collapse",
			org:         &organizationModels.Organization{AllowedBackends: []string{"a", "b", "a"}, DefaultBackend: "a"},
			wantAllowed: []string{"a", "b"},
			wantDefault: "a",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := services.NewBackendPolicy(tc.org, tc.plan, tc.system)
			assert.Equal(t, tc.wantAllowed, p.Allowed)
			assert.Equal(t, tc.wantDefault, p.Default)
		})
	}
}

func TestBackendPolicy_Pick(t *testing.T) {
	p := services.NewBackendPolicy(
		&organizationModels.Organization{AllowedBackends: []string{"a", "b"}, DefaultBackend: "a"}, nil, "")

	got, err := p.Pick("")
	require.NoError(t, err)
	assert.Equal(t, "a", got, "no request → default")

	got, err = p.Pick("b")
	require.NoError(t, err)
	assert.Equal(t, "b", got)

	_, err = p.Pick("c")
	assert.ErrorIs(t, err, services.ErrBackendNotAllowed)
	assert.Contains(t, err.Error(), "not allowed", "the HTTP layer maps this phrase to 403")
}

func TestBackendPolicy_Candidates(t *testing.T) {
	p := services.NewBackendPolicy(
		&organizationModels.Organization{AllowedBackends: []string{"a", "b"}, DefaultBackend: "b"}, nil, "")

	got, err := p.Candidates("")
	require.NoError(t, err)
	assert.Equal(t, []string{"b", "a"}, got, "no request → every allowed backend, default first")

	got, err = p.Candidates("a")
	require.NoError(t, err)
	assert.Equal(t, []string{"a"}, got, "an explicit choice is never swapped for another host")

	_, err = p.Candidates("c")
	assert.ErrorIs(t, err, services.ErrBackendNotAllowed)
}

func TestSystemDefaultOf(t *testing.T) {
	assert.Equal(t, "b", services.SystemDefaultOf([]dto.BackendInfo{{ID: "a"}, {ID: "b", IsDefault: true}}))
	assert.Equal(t, "a", services.SystemDefaultOf([]dto.BackendInfo{{ID: "a"}, {ID: "b"}}),
		"no marked default → first listed, so the list and the start agree")
	assert.Equal(t, "", services.SystemDefaultOf(nil))
}
