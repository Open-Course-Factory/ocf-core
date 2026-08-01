package services

import (
	orgModels "soli/formations/src/organizations/models"
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

	// ClassroomDeniedNotOrgMember: asked in the context of an organization the
	// user does not belong to.
	ClassroomDeniedNotOrgMember = "not_org_member"

	// ClassroomDeniedInsufficientOrgRole: the organization's plan grants
	// classrooms, but this member ranks below teacher — the student case in a
	// school, where everyone inherits the plan and only staff may run classes.
	ClassroomDeniedInsufficientOrgRole = "insufficient_org_role"

	// ClassroomDeniedPersonalOrg: personal organizations hold no groups at all.
	ClassroomDeniedPersonalOrg = "personal_organization"
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
// Two questions, depending on whether an organization is named:
//
//   - orgID != nil — "may this user run classes IN THIS organization?" Answers on
//     the organization's shape, the user's rank in it, and the plan that applies
//     there. All three, because in a school every member inherits a plan that
//     grants classrooms and only the role separates a teacher from a student.
//   - orgID == nil — "does this user's plan allow classrooms at all?" Plan only.
//     A trainer who has just bought Formateur and not yet created a team org
//     manages nothing, and must still be able to buy seats.
//
// Resolution failure and "no subscription" are deliberately the same verdict:
// GetUserEffectivePlan reports both as an error, and both mean the same thing
// here — no plan grants this. Failing closed is the only safe reading.
func (s *effectivePlanService) CanRunClassrooms(userID string, orgID *uuid.UUID) ClassroomEntitlement {
	if orgID != nil {
		if verdict, refused := s.refuseOnOrgContext(userID, *orgID); refused {
			return verdict
		}
	}

	result, err := s.GetUserEffectivePlan(userID, orgID)
	if err != nil || result == nil {
		return ClassroomEntitlement{Reason: ClassroomDeniedNoPlan}
	}
	return classroomEntitlementFor(result.Plan)
}

// refuseOnOrgContext checks the organization-shaped preconditions, returning
// refused=true with the verdict when one fails.
//
// These run BEFORE plan resolution so a refusal names the gate that actually
// closed. Resolving first would collapse "you are not a member" into "no plan",
// because resolveForOrg rejects non-members itself — and a user told to upgrade
// their plan when the real problem is their role will upgrade and still be stuck.
func (s *effectivePlanService) refuseOnOrgContext(userID string, orgID uuid.UUID) (ClassroomEntitlement, bool) {
	var org orgModels.Organization
	if err := s.db.First(&org, "id = ?", orgID).Error; err != nil {
		return ClassroomEntitlement{Reason: ClassroomDeniedNotOrgMember}, true
	}
	if org.IsPersonalOrg() {
		return ClassroomEntitlement{Reason: ClassroomDeniedPersonalOrg}, true
	}

	var member orgModels.OrganizationMember
	err := s.db.
		Where("organization_id = ? AND user_id = ? AND is_active = ?", orgID, userID, true).
		First(&member).Error
	if err != nil {
		return ClassroomEntitlement{Reason: ClassroomDeniedNotOrgMember}, true
	}

	// CanManageGroups owns the role threshold; restating it here as a comparison
	// would be the second copy that starts the drift.
	if !member.CanManageGroups() {
		return ClassroomEntitlement{Reason: ClassroomDeniedInsufficientOrgRole}, true
	}

	return ClassroomEntitlement{}, false
}
