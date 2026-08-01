package services

import (
	"fmt"

	"soli/formations/src/payment/models"
)

// ValidateOrgAssignablePlan is the single rule for whether a plan may govern an
// organization — whether as the organization's subscription or as a role mapping
// inside it.
//
// It exists because an organization's plan wins unconditionally over its members'
// own plans: resolveForOrg returns the org's plan with no priority comparison,
// deliberately, because a school's subscription is what decides for the school.
// That makes assigning the WRONG plan quietly destructive. A trainer whose
// organization was given a Solo plan could no longer create classes and silently
// dropped to Solo's budget, with nothing anywhere reporting a problem.
//
// The rule is GroupManagementEnabled, which today is exactly the set of plans
// meant for organizations — Formateur and École / OF. It is a proxy: "grants
// classrooms" and "is meant to govern an organization" are different questions
// that happen to coincide. The day an org-level plan without classroom features is
// wanted — a company buying a terminal budget for self-study — this should become
// an explicit OrgAssignable flag rather than being worked around by ticking a flag
// that means something else, which would quietly grant classrooms to that
// organization's members.
//
// Both doors must call this. Guarding only the organization subscription leaves
// OrganizationRolePlan, which resolveForOrg consults FIRST, as an equally easy way
// to make the same mistake.
func ValidateOrgAssignablePlan(plan *models.SubscriptionPlan) error {
	if plan == nil {
		return fmt.Errorf("cannot assign a missing plan to an organization")
	}
	if !plan.GroupManagementEnabled {
		return fmt.Errorf(
			"plan %q is an individual plan and cannot be assigned to an organization: "+
				"an organization's plan applies to all of its members and overrides their own, "+
				"so it must be a plan that grants group management",
			plan.Name)
	}
	return nil
}
