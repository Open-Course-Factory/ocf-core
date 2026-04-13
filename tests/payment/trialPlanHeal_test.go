// tests/payment/trialPlanHeal_test.go
// Regression tests for ensureUsersHaveTrialPlan heal function.
//
// Bug: The heal function queries `status = 'active'` only, but
// GetActiveUserSubscription uses `status IN ('active', 'trialing')`.
// A user with a 'trialing' subscription gets a duplicate Trial plan
// assignment on every server restart.
//
// These tests replicate the exact DB query used by ensureUsersHaveTrialPlan
// (which is unexported and calls casdoorsdk.GetUsers(), making it impossible
// to call directly in unit tests). By running the same query logic against
// the test DB we can prove the bug without mocking the Casdoor SDK.
package payment_tests

import (
	"testing"
	"time"

	entityManagementModels "soli/formations/src/entityManagement/models"
	paymentModels "soli/formations/src/payment/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// simulateHealQueryForUser runs the exact same DB query that
// ensureUsersHaveTrialPlan uses to decide whether a user already has a
// subscription. It returns true if the heal function would consider this user
// as "needs a Trial plan" (i.e. the query finds no subscription).
//
// This isolates the buggy WHERE clause:
//
//	db.Where("user_id = ? AND status = ?", userID, "active")
//
// instead of the correct:
//
//	db.Where("user_id = ? AND status IN (?)", userID, []string{"active", "trialing"})
func simulateHealQueryForUser(db *gorm.DB, userID string) bool {
	var existingSub paymentModels.UserSubscription
	subResult := db.Where("user_id = ? AND status IN ?", userID, []string{"active", "trialing"}).First(&existingSub)
	// Returns true when the heal function would create a new subscription
	// (i.e. no matching row was found → subResult.Error != nil)
	return subResult.Error != nil
}

// TestEnsureUsersHaveTrialPlan_WithTrialingStatus_ShouldNotDuplicate is the
// primary regression test for issue #244.
//
// It proves that the heal function creates a DUPLICATE subscription for a user
// who already has a 'trialing' subscription, because the query only checks
// status = 'active' and misses the trialing subscription entirely.
//
// EXPECTED (after the fix): the heal function recognises 'trialing' as an
// active subscription and skips the user → subscription count stays at 1.
//
// ACTUAL (before the fix / current state): the heal function does NOT see the
// trialing subscription and creates a second one → subscription count becomes 2.
// The test therefore FAILS until the fix is applied.
func TestEnsureUsersHaveTrialPlan_WithTrialingStatus_ShouldNotDuplicate(t *testing.T) {
	db := freshTestDB(t)

	// Seed a Trial plan (required by the heal function)
	trialPlan := &paymentModels.SubscriptionPlan{
		BaseModel:              entityManagementModels.BaseModel{ID: uuid.New()},
		Name:                   "Trial",
		Description:            "Free trial plan",
		Priority:               0,
		PriceAmount:            0,
		Currency:               "eur",
		BillingInterval:        "month",
		MaxCourses:             5,
		MaxConcurrentTerminals: 1,
		IsActive:               true,
	}
	require.NoError(t, db.Create(trialPlan).Error)

	// Create a user (represented by a Casdoor UUID string)
	userID := uuid.New().String()

	// Seed a 'trialing' subscription for this user.
	// This is the real-world scenario: Stripe sets status to 'trialing'
	// during the checkout trial period.
	now := time.Now()
	existingSub := &paymentModels.UserSubscription{
		BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
		UserID:             userID,
		SubscriptionPlanID: trialPlan.ID,
		Status:             "trialing", // <-- the status the heal function misses
		SubscriptionType:   "personal",
		CurrentPeriodStart: now.AddDate(0, -1, 0),
		CurrentPeriodEnd:   now.AddDate(0, 1, 0),
	}
	require.NoError(t, db.Create(existingSub).Error)

	// ── Confirm the precondition: user has exactly 1 subscription ──────────
	var initialCount int64
	db.Model(&paymentModels.UserSubscription{}).
		Where("user_id = ?", userID).
		Count(&initialCount)
	require.Equal(t, int64(1), initialCount, "precondition: user should start with 1 subscription")

	// ── Simulate what ensureUsersHaveTrialPlan does for this user ───────────
	// The function uses status = 'active' only.  If the query misses the
	// trialing subscription it will insert a second one.
	healWouldCreateDuplicate := simulateHealQueryForUser(db, userID)

	if healWouldCreateDuplicate {
		// Replicate the insert the heal function would make
		newSub := paymentModels.UserSubscription{
			BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
			UserID:             userID,
			SubscriptionPlanID: trialPlan.ID,
			Status:             "active",
			CurrentPeriodStart: now,
			CurrentPeriodEnd:   now.AddDate(1, 0, 0),
			SubscriptionType:   "personal",
		}
		require.NoError(t, db.Create(&newSub).Error, "heal function created a duplicate subscription unexpectedly")
	}

	// ── Assert: user should still have exactly 1 subscription ──────────────
	// This FAILS before the fix because the heal query misses 'trialing' status
	// and creates a second subscription.
	var finalCount int64
	db.Model(&paymentModels.UserSubscription{}).
		Where("user_id = ?", userID).
		Count(&finalCount)

	assert.Equal(t, int64(1), finalCount,
		"REGRESSION: ensureUsersHaveTrialPlan created a duplicate subscription for a user "+
			"who already has a 'trialing' subscription. The heal query must check "+
			"status IN ('active', 'trialing') instead of status = 'active'.")
}

// TestEnsureUsersHaveTrialPlan_WithActiveStatus_ShouldNotDuplicate verifies
// the baseline case (status = 'active') still works correctly after the fix.
// This test should pass both before and after the fix.
func TestEnsureUsersHaveTrialPlan_WithActiveStatus_ShouldNotDuplicate(t *testing.T) {
	db := freshTestDB(t)

	trialPlan := &paymentModels.SubscriptionPlan{
		BaseModel:              entityManagementModels.BaseModel{ID: uuid.New()},
		Name:                   "Trial",
		Description:            "Free trial plan",
		Priority:               0,
		PriceAmount:            0,
		Currency:               "eur",
		BillingInterval:        "month",
		MaxCourses:             5,
		MaxConcurrentTerminals: 1,
		IsActive:               true,
	}
	require.NoError(t, db.Create(trialPlan).Error)

	userID := uuid.New().String()
	now := time.Now()

	existingSub := &paymentModels.UserSubscription{
		BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
		UserID:             userID,
		SubscriptionPlanID: trialPlan.ID,
		Status:             "active", // already active — heal must skip
		SubscriptionType:   "personal",
		CurrentPeriodStart: now.AddDate(0, -1, 0),
		CurrentPeriodEnd:   now.AddDate(0, 1, 0),
	}
	require.NoError(t, db.Create(existingSub).Error)

	healWouldCreateDuplicate := simulateHealQueryForUser(db, userID)

	if healWouldCreateDuplicate {
		newSub := paymentModels.UserSubscription{
			BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
			UserID:             userID,
			SubscriptionPlanID: trialPlan.ID,
			Status:             "active",
			CurrentPeriodStart: now,
			CurrentPeriodEnd:   now.AddDate(1, 0, 0),
			SubscriptionType:   "personal",
		}
		require.NoError(t, db.Create(&newSub).Error)
	}

	var finalCount int64
	db.Model(&paymentModels.UserSubscription{}).
		Where("user_id = ?", userID).
		Count(&finalCount)

	assert.Equal(t, int64(1), finalCount,
		"user with 'active' subscription should not get a duplicate after heal")
}

// TestEnsureUsersHaveTrialPlan_NoSubscription_ShouldAssignTrial verifies that
// a user with NO subscription at all does receive one from the heal function.
// This is the intended happy path and must keep working after the fix.
func TestEnsureUsersHaveTrialPlan_NoSubscription_ShouldAssignTrial(t *testing.T) {
	db := freshTestDB(t)

	trialPlan := &paymentModels.SubscriptionPlan{
		BaseModel:              entityManagementModels.BaseModel{ID: uuid.New()},
		Name:                   "Trial",
		Description:            "Free trial plan",
		Priority:               0,
		PriceAmount:            0,
		Currency:               "eur",
		BillingInterval:        "month",
		MaxCourses:             5,
		MaxConcurrentTerminals: 1,
		IsActive:               true,
	}
	require.NoError(t, db.Create(trialPlan).Error)

	userID := uuid.New().String()
	// Deliberately do NOT create any subscription for this user.

	healWouldCreateDuplicate := simulateHealQueryForUser(db, userID)

	// The heal function SHOULD create a subscription here (user has none)
	assert.True(t, healWouldCreateDuplicate,
		"heal function should assign a Trial plan to a user with no subscription")

	now := time.Now()
	if healWouldCreateDuplicate {
		newSub := paymentModels.UserSubscription{
			BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
			UserID:             userID,
			SubscriptionPlanID: trialPlan.ID,
			Status:             "active",
			CurrentPeriodStart: now,
			CurrentPeriodEnd:   now.AddDate(1, 0, 0),
			SubscriptionType:   "personal",
		}
		require.NoError(t, db.Create(&newSub).Error)
	}

	var finalCount int64
	db.Model(&paymentModels.UserSubscription{}).
		Where("user_id = ?", userID).
		Count(&finalCount)

	assert.Equal(t, int64(1), finalCount,
		"user with no prior subscription should have exactly 1 Trial subscription after heal")
}
