package services

import (
	"fmt"

	"soli/formations/src/auth/access"
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
// This is the rule for the organization's SUBSCRIPTION. Role mappings apply it
// only to roles that run classes: see ValidateRolePlan.
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

// ValidateRolePlan is the rule for a plan mapped to a role inside an organization
// (OrganizationRolePlan, which resolveForOrg consults BEFORE the subscription).
//
// The role decides. A role that runs classes — teacher and above — must keep a
// plan that grants group management, or the school's managers silently lose
// their classrooms: same downgrade as the subscription door. Below that
// threshold the mapping is exactly where an individual plan belongs: a school
// holds a pool plan and maps its students to a seat plan that must NOT grant
// group management. Requiring it here made that model impossible to set up.
func ValidateRolePlan(role string, plan *models.SubscriptionPlan) error {
	if plan == nil {
		return fmt.Errorf("cannot map a role to a missing plan")
	}
	if !access.IsRoleAtLeast(role, access.RoleMinimumForClassrooms) {
		return nil
	}
	if !plan.GroupManagementEnabled {
		return fmt.Errorf(
			"plan %q is an individual plan and cannot be mapped to the %s role: "+
				"that role runs classes, and a mapping overrides the member's own plan, "+
				"so it must be a plan that grants group management",
			plan.Name, role)
	}
	return nil
}
