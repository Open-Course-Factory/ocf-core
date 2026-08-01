package services

import (
	"fmt"
	"time"

	"soli/formations/src/payment/models"
)

// maxPackDurationDays bounds a prepaid pack. A pack is a short course, not a
// subscription with extra steps; without a ceiling "10 learners for 3650 days"
// buys a decade of access at pack rates.
const maxPackDurationDays = 366

// PackTerms is the resolved shape of a bulk purchase: how many people it covers,
// what Stripe is asked to charge for, and when the entitlement stops.
//
// It exists because those three were different numbers that nobody separated.
// The frontend multiplied learners by days and sent the product as "quantity";
// the backend then created that many licences, each running a full billing
// period. "10 learners for 3 days" was priced as 30 learner-days and delivered as
// 30 individually-assignable month-long seats — strictly better value than the
// monthly seat it was meant to undercut, which inverts the ladder
// seatPricingCoherence exists to police (#455).
type PackTerms struct {
	// Licences is how many people can hold a seat from this purchase.
	Licences int

	// BillingUnits is what Stripe charges for. For a learner-day pack it is
	// Licences × DurationDays, because the pack is priced per learner-day; for a
	// monthly seat it equals Licences.
	BillingUnits int

	// ExpiresAt is the entitlement deadline for every licence in the batch, or
	// nil for a product with no deadline of its own. ScopeEntitling already
	// excludes rows past it, so an expired pack stops entitling with no sweeper.
	ExpiresAt *time.Time

	// DurationDays is the pack length, zero for a monthly seat. Carried so the
	// checkout path can put it in Stripe metadata and the webhook can rebuild
	// these same terms.
	DurationDays int
}

// ResolvePackTerms is the single owner of "what did this purchase actually buy?".
//
// learners is the number of people to cover. durationDays is the pack length and
// is meaningful only for learner-day products.
//
// now is passed in rather than read here so the direct path and the webhook path
// can anchor a batch to the same instant, and so tests are not time-dependent.
func ResolvePackTerms(plan *models.SubscriptionPlan, learners, durationDays int, now time.Time) (PackTerms, error) {
	if plan == nil {
		return PackTerms{}, fmt.Errorf("cannot resolve pack terms without a plan")
	}
	if learners < 1 {
		return PackTerms{}, fmt.Errorf("a purchase must cover at least one learner")
	}

	if plan.EffectiveSeatUnit() != models.SeatUnitLearnerDay {
		// A monthly seat has no pack length. Accepting one silently would let a
		// caller believe they had bought a bounded product.
		if durationDays > 0 {
			return PackTerms{}, fmt.Errorf(
				"plan %q is sold per seat and per month, so it takes no duration in days", plan.Name)
		}
		return PackTerms{Licences: learners, BillingUnits: learners}, nil
	}

	if durationDays < 1 {
		return PackTerms{}, fmt.Errorf(
			"plan %q is sold per learner-day, so a duration in days is required", plan.Name)
	}
	if durationDays > maxPackDurationDays {
		return PackTerms{}, fmt.Errorf(
			"a prepaid pack cannot run longer than %d days; use a monthly seat plan instead",
			maxPackDurationDays)
	}

	expiresAt := now.AddDate(0, 0, durationDays)
	return PackTerms{
		Licences:     learners,
		BillingUnits: learners * durationDays,
		ExpiresAt:    &expiresAt,
		DurationDays: durationDays,
	}, nil
}
