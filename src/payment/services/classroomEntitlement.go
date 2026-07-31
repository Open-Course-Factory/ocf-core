package services

import (
	"soli/formations/src/payment/models"

	"github.com/google/uuid"
)

// Refusal codes for CanRunClassrooms. Machine-readable so the frontend can
// translate them; the code is the contract, the English gloss below is not.
const (
	// ClassroomDeniedNoPlan: no subscription resolved for this user in this
	// context, so there is no plan to grant anything.
	ClassroomDeniedNoPlan = "no_plan"

	// ClassroomDeniedPlanLacksGroupManagement: a plan resolved, but it does not
	// grant group management.
	ClassroomDeniedPlanLacksGroupManagement = "plan_lacks_group_management"
)

// ClassroomEntitlement is the verdict on "may this user run classrooms?" —
// create class groups, convert an organization to a team, buy seats for learners.
//
// The underlying rule is a single plan flag, GroupManagementEnabled. What made it
// drift across five call sites was never the flag: it was the question of WHICH
// plan to read it from, which each site answered for itself. The verdict therefore
// carries the plan it was read from, so a caller needing more detail cannot
// re-resolve and end up reading a different plan than the one that decided this.
type ClassroomEntitlement struct {
	Allowed bool

	// Reason carries a refusal code, empty when Allowed. Callers surfacing it to
	// users must translate it — it is not a display string.
	Reason string

	// Plan is the plan the verdict was read from, nil when none resolved.
	Plan *models.SubscriptionPlan
}

// classroomEntitlementFor applies the rule to an already-resolved plan.
//
// Split from CanRunClassrooms so callers that have just resolved a plan — the
// feature endpoints, which would otherwise resolve it twice per request — can
// reuse that resolution and still get the identical verdict. A second resolution
// is not merely wasteful: between the two calls it can legitimately return a
// different plan, and then the verdict would not describe the plan reported
// alongside it.
//
// A nil plan is not entitled. Absent must never read as yes: a user whose
// subscription cannot be resolved must not be invited into a classroom the
// backend will then refuse to run.
func classroomEntitlementFor(plan *models.SubscriptionPlan) ClassroomEntitlement {
	if plan == nil {
		return ClassroomEntitlement{Reason: ClassroomDeniedNoPlan}
	}
	if !plan.GroupManagementEnabled {
		return ClassroomEntitlement{Reason: ClassroomDeniedPlanLacksGroupManagement, Plan: plan}
	}
	return ClassroomEntitlement{Allowed: true, Plan: plan}
}

// ClassroomEntitlementFor is the exported accessor for classroomEntitlementFor,
// for packages that hold a resolved plan and need the same verdict.
func ClassroomEntitlementFor(plan *models.SubscriptionPlan) ClassroomEntitlement {
	return classroomEntitlementFor(plan)
}

// CanRunClassrooms — see EffectivePlanService.
//
// Resolution failure and "no subscription" are deliberately the same verdict:
// GetUserEffectivePlan reports both as an error, and both mean the same thing
// here — no plan grants this. Failing closed is the only safe reading.
func (s *effectivePlanService) CanRunClassrooms(userID string, orgID *uuid.UUID) ClassroomEntitlement {
	result, err := s.GetUserEffectivePlan(userID, orgID)
	if err != nil || result == nil {
		return ClassroomEntitlement{Reason: ClassroomDeniedNoPlan}
	}
	return classroomEntitlementFor(result.Plan)
}
