package services

import (
	"errors"
	"fmt"

	"soli/formations/src/payment/models"
	"soli/formations/src/utils"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// This file owns one question: did a subscription's plan association actually
// resolve to a plan?
//
// It needs an owner because the answer is not obvious at the call site. GORM's
// Preload honours soft deletes, so a subscription pointing at a deleted plan
// comes back carrying the ZERO-VALUE SubscriptionPlan rather than failing. That
// struct looks like a plan to every consumer: it has a name (empty), an ID (nil)
// and limits (zero) — and QuotaService reads MaxCPU <= 0 as "no cap on this
// axis", which is the correct rule for a genuinely unlimited plan. So a deleted
// plan granted unlimited capacity, XL machines included (#481).
//
// The predicate was written twice before this: once inline in
// organizationSubscriptionService (#451), and not at all in EffectivePlanService,
// which is the service that actually owns "which plan applies" — so the same
// dangling reference failed closed on one path and fail-OPEN on the other. One
// rule, one function, applied wherever a plan association is consumed.

// ErrDanglingPlanReference reports a subscription whose plan no longer exists.
//
// A sentinel rather than a string match so callers can tell "this user has no
// plan" (an ordinary state — they get an upgrade prompt) from "this user's plan
// reference is broken" (an operator problem, and never an entitlement).
var ErrDanglingPlanReference = errors.New("subscription references a plan that no longer exists")

// planDidLoad reports whether a plan association actually resolved.
//
// The ID is the test, not the price or the name: a 0 EUR plan is a real plan,
// and an unlimited plan legitimately has zero limits. Only a nil ID means
// nothing came back.
func planDidLoad(p *models.SubscriptionPlan) bool {
	return p != nil && p.ID != uuid.Nil
}

// ensurePlanLoaded reports a dangling reference as an error, and logs which row
// carries it.
//
// `holder` names that row (e.g. "user subscription 0e2c…"), so the log line tells
// an operator what to repair rather than only that something is wrong.
func ensurePlanLoaded(p *models.SubscriptionPlan, holder string) error {
	if planDidLoad(p) {
		return nil
	}
	utils.Warn("Dangling plan reference: %s points at a plan that no longer exists; "+
		"refusing to resolve it (see ReportDanglingPlanReferences)", holder)
	return fmt.Errorf("%w: %s", ErrDanglingPlanReference, holder)
}

// DanglingPlanReport counts the rows whose plan reference cannot resolve.
type DanglingPlanReport struct {
	UserSubscriptions         int
	OrganizationSubscriptions int
	OrganizationRolePlans     int
}

// Any reports whether anything needs repairing.
func (r DanglingPlanReport) Any() bool {
	return r.UserSubscriptions+r.OrganizationSubscriptions+r.OrganizationRolePlans > 0
}

// ReportDanglingPlanReferences counts subscriptions and role mappings pointing
// at a plan that no longer exists.
//
// Failing closed makes a broken reference visible to the USER — they lose access
// — while telling the operator nothing. This is the other half: it names the
// breakage at startup, so the answer to "why can this customer not launch
// anything" is one log line rather than an investigation.
//
// Read-only by design: repairing means choosing a replacement plan, which is a
// commercial decision and not one a startup routine should make silently.
func ReportDanglingPlanReferences(db *gorm.DB) DanglingPlanReport {
	livePlanIDs := db.Model(&models.SubscriptionPlan{}).Select("id")

	count := func(model any, label string, scopes ...func(*gorm.DB) *gorm.DB) int {
		var n int64
		if err := db.Model(model).
			Scopes(scopes...).
			Where("subscription_plan_id NOT IN (?)", livePlanIDs).
			Count(&n).Error; err != nil {
			utils.Warn("Failed to count dangling plan references on %s: %v", label, err)
			return 0
		}
		return int(n)
	}

	// Only subscriptions that still entitle someone. A cancelled subscription
	// pointing at a retired plan is ordinary history, and reporting it every
	// startup is how a warning that matters gets tuned out. Role mappings carry no
	// status, so every one of them counts.
	return DanglingPlanReport{
		UserSubscriptions:         count(&models.UserSubscription{}, "user_subscriptions", models.ScopeEntitling),
		OrganizationSubscriptions: count(&models.OrganizationSubscription{}, "organization_subscriptions", models.ScopeEntitling),
		OrganizationRolePlans:     count(&models.OrganizationRolePlan{}, "organization_role_plans"),
	}
}
