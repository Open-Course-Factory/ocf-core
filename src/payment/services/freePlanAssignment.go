package services

import (
	"errors"
	"fmt"

	"soli/formations/src/payment/models"

	"gorm.io/gorm"
)

// FreePlanName is the customer-facing name the free default plan is SEEDED with.
//
// It is a label, not an identifier. The plan is found through the typed
// SubscriptionPlan.IsDefaultFree marker, precisely so that renaming it — which
// marketing may do again — cannot break signup auto-assignment or make the seed
// recreate a plan the lookup can no longer see.
const FreePlanName = "Découverte"

// LegacyFreePlanName is the name the free plan carried before the offer was
// named. Only the startup migrations still reference it, to find the row that
// predates the marker.
const LegacyFreePlanName = "Trial"

// FindFreePlan returns the active free default plan, or an error if it is absent.
func FindFreePlan(db *gorm.DB) (*models.SubscriptionPlan, error) {
	var plan models.SubscriptionPlan
	err := db.Where("is_default_free = ? AND price_amount = 0 AND is_active = ?", true, true).
		First(&plan).Error
	if err != nil {
		return nil, fmt.Errorf("could not find the active default free plan: %w", err)
	}
	return &plan, nil
}

// EnsureFreeTrialAssigned gives userID the free default plan unless they already
// hold a subscription that entitles them. It reports whether a subscription was
// created, so callers can distinguish "assigned" from "already had one" without
// re-querying.
//
// The "already has one" test uses the canonical entitling predicate rather than
// status='active'. That distinction is the bug this function was extracted to
// fix: a user in dunning (past_due) holds a perfectly real subscription, and
// checking only for 'active' handed them a second one on top of it.
func EnsureFreeTrialAssigned(db *gorm.DB, userID string) (bool, error) {
	var existing models.UserSubscription
	err := db.Scopes(models.ScopeEntitling).Where("user_id = ?", userID).First(&existing).Error
	switch {
	case err == nil:
		return false, nil
	case !errors.Is(err, gorm.ErrRecordNotFound):
		return false, fmt.Errorf("failed to check existing subscription for user %s: %w", userID, err)
	}

	plan, err := FindFreePlan(db)
	if err != nil {
		return false, err
	}

	if _, err := NewSubscriptionService(db).CreateUserSubscription(userID, plan.ID); err != nil {
		return false, fmt.Errorf("failed to create %s subscription for user %s: %w", FreePlanName, userID, err)
	}
	return true, nil
}
