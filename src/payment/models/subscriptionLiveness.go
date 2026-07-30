package models

import "gorm.io/gorm"

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
// `trialing` is carried in both sets purely as defensiveness against Stripe's
// state machine reporting it — OCF sells no paid trials. Its removal is tracked
// separately (#439) and is a one-line change now that the sets live here; note
// it also appears in a partial UNIQUE INDEX on organization_subscriptions.
var (
	entitlingStatuses = []string{"active", "trialing", "past_due"}
	billableStatuses  = []string{"active", "trialing"}
)

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
// organization_subscriptions carry a `status` column, so one scope serves both.
func ScopeEntitling(tx *gorm.DB) *gorm.DB {
	return tx.Where("status IN ?", entitlingStatuses)
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
