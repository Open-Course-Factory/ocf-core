package services

import (
	"errors"
	"fmt"

	"soli/formations/src/payment/models"

	"gorm.io/gorm"
)

// FreePlanName is the name the free default plan must carry.
//
// This is a constraint, not a preference: the free plan is selected BY NAME in
// several places, and the startup seed recreates a plan with this name when none
// exists. Renaming the row (e.g. to a customer-facing "Découverte") therefore
// breaks signup auto-assignment and then silently spawns a duplicate free plan.
// Giving the plan a customer-facing name requires replacing the name lookup with
// a typed marker first; until then, this constant is the single place the literal
// should appear.
const FreePlanName = "Trial"

// FindFreePlan returns the active free default plan, or an error if it is absent.
func FindFreePlan(db *gorm.DB) (*models.SubscriptionPlan, error) {
	var plan models.SubscriptionPlan
	err := db.Where("name = ? AND price_amount = 0 AND is_active = ?", FreePlanName, true).
		First(&plan).Error
	if err != nil {
		return nil, fmt.Errorf("could not find active %s plan: %w", FreePlanName, err)
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
	err := db.Scopes(ScopeEntitling).Where("user_id = ?", userID).First(&existing).Error
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
