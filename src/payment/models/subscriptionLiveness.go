package models

import (
	"time"

	"gorm.io/gorm"
)

// Subscription liveness — the single owner of "does this subscription still count?".
//
// Before this file the question was answered inline in 22 places with three
// different status sets, and they had drifted apart: some checked `active`
// alone, some `active, trialing`, some `active, trialing, past_due`. That is not
// merely duplication, it produced wrong behaviour — the free-plan assignment
// decided "this user already has a subscription" with `active` alone, so a user
// sitting in dunning was handed a SECOND subscription on top of the one they had.
//
// It lives in models rather than services because the repository layer needs it
// too, and services already imports repositories — the reverse edge would be an
// import cycle. Both layers can depend on models.
//
// There are deliberately TWO predicates, not one, because entitlement and
// billing genuinely disagree about dunning:
//
//   - Entitling: the holder still gets access. `past_due` belongs here — a
//     subscription in dunning keeps content and within-grace sessions working,
//     and access beyond the grace window is cut at session creation, not here.
//   - Billable: the subscription is cleanly paid. `past_due` is excluded, since
//     it is precisely the not-paid case. GetActiveSubscriptionByCustomerID and
//     GetRecoverableSubscriptionByCustomerID are the canonical illustration of
//     the pair: a successful invoice payment must be able to find and cure a
//     past_due subscription, which the billable lookup can never see.
//
// Entitling is always a superset of Billable; a test pins that, because the
// inverse would mean charging someone for access they do not have.
//
// `trialing` was removed in #439: OCF sells no paid trials, so the free Trial
// *plan* is the only "trial" and the status was never a product state — it was
// carried purely as defensiveness against Stripe's state machine reporting it.
// Removing it required more than editing these slices, because the status was
// also baked into a partial UNIQUE INDEX on organization_subscriptions and into
// existing rows; see MigrateUniqueActiveOrgSubscriptionIndex and
// MigrateTrialingStatusToActive.
//
// Defensiveness did not disappear with it, it moved somewhere honest: an
// unmodelled status now surfaces as a warning at the Stripe ingestion point
// (see IsKnownStatus) instead of silently landing in the database as a row that
// entitles nobody.
var (
	entitlingStatuses = []string{"active", "past_due"}
	billableStatuses  = []string{"active"}
)

// knownStatuses is every status OCF models. It is deliberately wider than the
// two predicates: cancelled and unpaid are perfectly well understood, they just
// do not entitle. The point of the set is to tell "understood and not entitling"
// apart from "we have never seen this before", which is the case that must be
// loud — a subscription silently stuck in an unrecognised status entitles
// nobody and looks, from the outside, exactly like a billing bug.
var knownStatuses = []string{
	"active", "past_due", "cancelled", "unpaid",
	"incomplete", "incomplete_expired", "paused", "replaced",
}

// IsKnownStatus reports whether OCF models this subscription status at all.
// A false result means the status should be surfaced, not swallowed.
func IsKnownStatus(status string) bool {
	return containsStatus(knownStatuses, status)
}

// EntitlingStatuses returns the statuses under which a subscription grants access.
// The slice is copied so callers cannot mutate the canonical definition.
//
// Prefer ScopeEntitling; use this when the column needs table qualification, as
// in a join where a bare `status` would be ambiguous.
func EntitlingStatuses() []string {
	return append([]string(nil), entitlingStatuses...)
}

// BillableStatuses returns the statuses under which a subscription counts as
// cleanly paid. Narrower than EntitlingStatuses: dunning is excluded.
func BillableStatuses() []string {
	return append([]string(nil), billableStatuses...)
}

// IsEntitling reports whether a subscription in this status grants its holder access.
func IsEntitling(status string) bool {
	return containsStatus(entitlingStatuses, status)
}

// IsBillable reports whether a subscription in this status is cleanly paid.
func IsBillable(status string) bool {
	return containsStatus(billableStatuses, status)
}

// ScopeEntitling is a GORM scope filtering any subscription table down to rows
// that currently grant access. Both user_subscriptions and
// organization_subscriptions carry `status` and `expires_at`, so one scope
// serves both — and must, since a column present on only one table would break
// the other's plan resolution outright.
//
// Two terms, because a subscription can stop entitling two different ways:
// its status changes (Stripe drives that), or its window closes (#440). The
// second is what a prepaid pack needs — it has no Stripe subscription to flip a
// status for it, so without a deadline it would grant access forever.
//
// NULL expires_at means no deadline, so every row that predates this and every
// ordinary recurring subscription is unaffected. That default is deliberate:
// treating absent as expired would have revoked access silently and en masse.
func ScopeEntitling(tx *gorm.DB) *gorm.DB {
	return tx.Where("status IN ?", entitlingStatuses).
		Where("expires_at IS NULL OR expires_at > ?", time.Now())
}

// ScopeBillable is the billing-side counterpart of ScopeEntitling.
func ScopeBillable(tx *gorm.DB) *gorm.DB {
	return tx.Where("status IN ?", billableStatuses)
}

func containsStatus(set []string, status string) bool {
	for _, s := range set {
		if s == status {
			return true
		}
	}
	return false
}
