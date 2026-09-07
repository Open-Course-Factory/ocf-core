// tests/payment/subscriptionPlanOrgAssignability_test.go
//
// Since #458 a plan can only govern an organization when it grants group
// management, but the rule was enforced at assignment: an admin built a
// bespoke school plan and learnt of the constraint by hitting a refusal.
// The plan's own response now says so, from the same rule the door applies.
package payment_tests

import (
	"encoding/json"
	"testing"

	entityServices "soli/formations/src/entityManagement/services"
	"soli/formations/src/payment/dto"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createdPlanOutput creates a plan through the real generic service and
// converts it the way the create controller does before answering, so what is
// asserted is what the admin editor receives.
func createdPlanOutput(t *testing.T, input dto.CreateSubscriptionPlanInput) dto.SubscriptionPlanOutput {
	t.Helper()
	svc := entityServices.NewGenericService(sharedTestDB, nil)
	created, err := svc.CreateEntityWithUser(input, "SubscriptionPlan", "admin-1")
	require.NoError(t, err)
	answered, failed := svc.GetEntityFromResult("SubscriptionPlan", created)
	require.False(t, failed, "the created plan must convert to its output DTO")

	raw, err := json.Marshal(answered)
	require.NoError(t, err)
	var out dto.SubscriptionPlanOutput
	require.NoError(t, json.Unmarshal(raw, &out))
	return out
}

func TestSubscriptionPlan_CreateResponse_WarnsWhenNotOrgAssignable(t *testing.T) {
	_ = freshTestDB(t)
	registerSubscriptionPlanForScoping(t)
	withPaymentHooksRegistered(t)

	out := createdPlanOutput(t, dto.CreateSubscriptionPlanInput{
		Name: "École ESITECH 2026", PriceAmount: 9900, Currency: "eur", BillingInterval: "month",
		MaxCPU: 4000, MaxMemoryMB: 4096,
		GroupManagementEnabled: false,
	})

	assert.False(t, out.OrgAssignable)
	assert.Contains(t, out.OrgAssignabilityNote, "cannot be assigned to an organization",
		"the response must say the plan is individual-only before an admin hits the refusal")
}

func TestSubscriptionPlan_CreateResponse_SilentWhenOrgAssignable(t *testing.T) {
	_ = freshTestDB(t)
	registerSubscriptionPlanForScoping(t)
	withPaymentHooksRegistered(t)

	out := createdPlanOutput(t, dto.CreateSubscriptionPlanInput{
		Name: "École", PriceAmount: 9900, Currency: "eur", BillingInterval: "month",
		MaxCPU: 4000, MaxMemoryMB: 4096,
		GroupManagementEnabled: true,
	})

	assert.True(t, out.OrgAssignable)
	assert.NotContains(t, out.OrgAssignabilityNote, "cannot be assigned",
		"an assignable plan must not carry the refusal wording")
}
