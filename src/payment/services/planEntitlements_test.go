package services

// Internal (package services) unit tests for derivePlanEntitlements — the pure
// projection from a plan's TYPED capability fields to the canonical entitlement
// strings. Internal because the helper is unexported by design (SSOT lives next
// to its consumers).
//
// RED: the skeleton returns nil, so every non-empty expectation fails. The
// zero-plan and nil-plan cases pass today (both want empty), pinning the
// zero-input contract.

import (
	"testing"

	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/payment/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func planWith(mutate func(p *models.SubscriptionPlan)) *models.SubscriptionPlan {
	p := &models.SubscriptionPlan{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Name:      "p",
	}
	if mutate != nil {
		mutate(p)
	}
	return p
}

func TestDerivePlanEntitlements_TableOverFieldCombos(t *testing.T) {
	cases := []struct {
		name string
		plan *models.SubscriptionPlan
		want []string
	}{
		{
			name: "zero plan yields no entitlements",
			plan: planWith(nil),
			want: []string{},
		},
		{
			name: "group management yields group_management + multiple_groups",
			plan: planWith(func(p *models.SubscriptionPlan) { p.GroupManagementEnabled = true }),
			want: []string{"group_management", "multiple_groups"},
		},
		{
			name: "network access yields network_access",
			plan: planWith(func(p *models.SubscriptionPlan) { p.NetworkAccessEnabled = true }),
			want: []string{"network_access"},
		},
		{
			name: "data persistence yields data_persistence",
			plan: planWith(func(p *models.SubscriptionPlan) { p.DataPersistenceEnabled = true }),
			want: []string{"data_persistence"},
		},
		{
			name: "positive command history retention yields command_history",
			plan: planWith(func(p *models.SubscriptionPlan) { p.CommandHistoryRetentionDays = 30 }),
			want: []string{"command_history"},
		},
		{
			name: "zero command history retention yields nothing",
			plan: planWith(func(p *models.SubscriptionPlan) { p.CommandHistoryRetentionDays = 0 }),
			want: []string{},
		},
		{
			name: "session supervision yields session_supervision",
			plan: planWith(func(p *models.SubscriptionPlan) { p.SessionSupervisionEnabled = true }),
			want: []string{"session_supervision"},
		},
		{
			name: "all capabilities yield the full canonical set",
			plan: planWith(func(p *models.SubscriptionPlan) {
				p.GroupManagementEnabled = true
				p.NetworkAccessEnabled = true
				p.DataPersistenceEnabled = true
				p.CommandHistoryRetentionDays = 7
				p.SessionSupervisionEnabled = true
			}),
			want: []string{
				"group_management", "multiple_groups",
				"network_access", "data_persistence",
				"command_history", "session_supervision",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := derivePlanEntitlements(tc.plan)
			assert.ElementsMatch(t, tc.want, got,
				"derivePlanEntitlements must emit exactly the canonical entitlement set for the typed fields")
			// Dropped capabilities must never be projected, regardless of fields.
			assert.NotContains(t, got, "api_access", "api_access is dropped and must never be projected")
			assert.NotContains(t, got, "advanced_terminals", "advanced_terminals must not be projected")
		})
	}
}

func TestDerivePlanEntitlements_NilPlanYieldsEmpty(t *testing.T) {
	got := derivePlanEntitlements(nil)
	assert.Empty(t, got, "a nil plan must yield an empty entitlement slice, not a panic")
}
