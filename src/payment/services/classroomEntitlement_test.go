package services

// Internal unit tests for the classroom-entitlement rule — the single owner of
// "may this user run classrooms?" (#453).
//
// The rule itself is one flag, so the interesting coverage is not the happy path
// but the two ways the five previous implementations went wrong: treating an
// absent plan as permission, and answering from something other than the plan
// that actually applies.

import (
	"testing"

	"soli/formations/src/payment/models"

	"github.com/stretchr/testify/assert"
)

func TestClassroomEntitlementFor_RuleTable(t *testing.T) {
	cases := []struct {
		name       string
		plan       *models.SubscriptionPlan
		wantAllow  bool
		wantReason string
	}{
		{
			name:       "nil plan is refused, never assumed",
			plan:       nil,
			wantAllow:  false,
			wantReason: ClassroomDeniedNoPlan,
		},
		{
			name:       "zero plan does not grant classrooms",
			plan:       planWith(nil),
			wantAllow:  false,
			wantReason: ClassroomDeniedPlanLacksGroupManagement,
		},
		{
			name:       "group management grants classrooms",
			plan:       planWith(func(p *models.SubscriptionPlan) { p.GroupManagementEnabled = true }),
			wantAllow:  true,
			wantReason: "",
		},
		{
			name: "other capabilities do not grant classrooms on their own",
			plan: planWith(func(p *models.SubscriptionPlan) {
				p.NetworkAccessEnabled = true
				p.DataPersistenceEnabled = true
				p.SessionSupervisionEnabled = true
				p.CommandHistoryRetentionDays = 30
			}),
			wantAllow:  false,
			wantReason: ClassroomDeniedPlanLacksGroupManagement,
		},
		{
			name: "bulk-purchasable seat plan does not itself grant classrooms",
			plan: planWith(func(p *models.SubscriptionPlan) {
				p.BulkPurchasable = true
				p.SeatUnit = models.SeatUnitLearnerDay
			}),
			wantAllow:  false,
			wantReason: ClassroomDeniedPlanLacksGroupManagement,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classroomEntitlementFor(tc.plan)
			assert.Equal(t, tc.wantAllow, got.Allowed)
			assert.Equal(t, tc.wantReason, got.Reason,
				"a refusal must carry a machine-readable code so clients can translate it")
		})
	}
}

// A refused verdict must be distinguishable between "no plan at all" and "a plan
// that does not grant it": the purchase screen says different things for each,
// and collapsing them is what produced the misleading "no active subscription"
// message for users who had one.
func TestClassroomEntitlementFor_RefusalsAreDistinguishable(t *testing.T) {
	noPlan := classroomEntitlementFor(nil)
	weakPlan := classroomEntitlementFor(planWith(nil))

	assert.NotEqual(t, noPlan.Reason, weakPlan.Reason)
	assert.Nil(t, noPlan.Plan, "there is no plan to report when none resolved")
	assert.NotNil(t, weakPlan.Plan,
		"the refusing plan must be reported so callers need not re-resolve to explain the refusal")
}

// The verdict carries the plan it was read from. Callers that need further detail
// must be able to use that plan rather than resolving again — a second resolution
// can return a different plan, and the verdict would then not describe it.
func TestClassroomEntitlementFor_CarriesTheDecidingPlan(t *testing.T) {
	plan := planWith(func(p *models.SubscriptionPlan) { p.GroupManagementEnabled = true })

	got := classroomEntitlementFor(plan)

	assert.True(t, got.Allowed)
	assert.Same(t, plan, got.Plan, "the verdict must carry the exact plan it decided from")
}
