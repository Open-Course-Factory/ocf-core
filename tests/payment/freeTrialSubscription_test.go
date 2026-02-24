// tests/payment/freeTrialSubscription_test.go
// Integration test to verify that the free trial subscription fix works correctly
// This test ensures that:
// 1. Multiple free subscriptions can be created without unique constraint violations
// 2. Free subscriptions have NULL StripeSubscriptionID and StripeCustomerID
// 3. Paid subscriptions still enforce uniqueness on non-NULL Stripe IDs
package payment_tests

import (
	"strings"
	"testing"
	"time"

	"soli/formations/src/payment/models"
	"soli/formations/src/payment/repositories"
	"soli/formations/src/payment/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMultipleFreeSubscriptions_NoUniqueConstraintViolation verifies that
// multiple free subscriptions can be created without violating unique constraints
// This is a regression test for the bug where empty StripeSubscriptionID caused duplicates
func TestMultipleFreeSubscriptions_NoUniqueConstraintViolation(t *testing.T) {
	db := freshTestDB(t)

	// Create a free trial plan
	trialPlan := &models.SubscriptionPlan{
		Name:                   "Trial",
		Description:            "Free trial plan for testing",
		Priority:               0,
		PriceAmount:            0, // Free plan
		Currency:               "eur",
		BillingInterval:        "month",
		MaxCourses:             5,
		MaxConcurrentTerminals: 1,
		IsActive:               true,
	}
	err := db.Create(trialPlan).Error
	require.NoError(t, err, "Failed to create trial plan")

	// Clean up
	defer func() {
		db.Where("subscription_plan_id = ?", trialPlan.ID).Unscoped().Delete(&models.UserSubscription{})
		db.Unscoped().Delete(trialPlan)
	}()

	// Create subscription service
	subscriptionService := services.NewSubscriptionService(db)

	// Test: Create multiple free subscriptions for different users
	t.Run("Create multiple free subscriptions without error", func(t *testing.T) {
		userIDs := []string{
			uuid.New().String(),
			uuid.New().String(),
			uuid.New().String(),
		}

		var createdSubscriptions []*models.UserSubscription

		for i, userID := range userIDs {
			subscription, err := subscriptionService.CreateUserSubscription(userID, trialPlan.ID)
			assert.NoError(t, err, "Failed to create free subscription for user %d", i+1)
			assert.NotNil(t, subscription, "Subscription should not be nil for user %d", i+1)

			if subscription != nil {
				createdSubscriptions = append(createdSubscriptions, subscription)

				// Verify the subscription is active and free
				assert.Equal(t, "active", subscription.Status, "Free subscription should be active")
				assert.Equal(t, userID, subscription.UserID)
				assert.Equal(t, trialPlan.ID, subscription.SubscriptionPlanID)

				// CRITICAL: Verify StripeSubscriptionID and StripeCustomerID are NULL
				assert.Nil(t, subscription.StripeSubscriptionID, "Free subscription should have NULL StripeSubscriptionID")
				assert.Nil(t, subscription.StripeCustomerID, "Free subscription should have NULL StripeCustomerID")
			}
		}

		// Verify all subscriptions were created
		assert.Equal(t, len(userIDs), len(createdSubscriptions), "All free subscriptions should be created")

		// Verify in database that all have NULL Stripe IDs
		var dbSubscriptions []models.UserSubscription
		err := db.Where("subscription_plan_id = ?", trialPlan.ID).Find(&dbSubscriptions).Error
		require.NoError(t, err, "Failed to query subscriptions from DB")

		for _, sub := range dbSubscriptions {
			assert.Nil(t, sub.StripeSubscriptionID, "DB record should have NULL StripeSubscriptionID")
			assert.Nil(t, sub.StripeCustomerID, "DB record should have NULL StripeCustomerID")
		}
	})
}

// TestPaidSubscriptions_UniqueConstraintEnforced verifies that
// paid subscriptions with the same StripeSubscriptionID cannot be created
func TestPaidSubscriptions_UniqueConstraintEnforced(t *testing.T) {
	db := freshTestDB(t)

	// Create a paid plan
	paidPlan := &models.SubscriptionPlan{
		Name:                   "Pro",
		Description:            "Paid pro plan for testing",
		Priority:               10,
		PriceAmount:            1999, // €19.99
		Currency:               "eur",
		BillingInterval:        "month",
		MaxCourses:             -1, // Unlimited
		MaxConcurrentTerminals: 5,
		IsActive:               true,
	}
	err := db.Create(paidPlan).Error
	require.NoError(t, err, "Failed to create paid plan")

	// Clean up
	defer func() {
		db.Where("subscription_plan_id = ?", paidPlan.ID).Unscoped().Delete(&models.UserSubscription{})
		db.Unscoped().Delete(paidPlan)
	}()

	// Create repository
	repo := repositories.NewPaymentRepository(db)

	// Test: Create two subscriptions with the same Stripe ID should fail
	t.Run("Duplicate Stripe subscription ID should fail", func(t *testing.T) {
		stripeSubID := "sub_test_duplicate_" + uuid.New().String()[:8]
		stripeCustomerID := "cus_test_customer_" + uuid.New().String()[:8]

		user1ID := uuid.New().String()
		user2ID := uuid.New().String()

		// Create first subscription
		sub1 := &models.UserSubscription{
			UserID:               user1ID,
			SubscriptionPlanID:   paidPlan.ID,
			StripeSubscriptionID: &stripeSubID,
			StripeCustomerID:     &stripeCustomerID,
			Status:               "active",
			CurrentPeriodStart:   time.Now(),
			CurrentPeriodEnd:     time.Now().AddDate(0, 1, 0),
		}
		err := repo.CreateUserSubscription(sub1)
		assert.NoError(t, err, "First subscription should be created successfully")

		if err == nil {
			// Clean up after test
			defer db.Unscoped().Delete(sub1)
		}

		// Try to create second subscription with same Stripe ID
		sub2 := &models.UserSubscription{
			UserID:               user2ID,
			SubscriptionPlanID:   paidPlan.ID,
			StripeSubscriptionID: &stripeSubID, // Same Stripe ID
			StripeCustomerID:     &stripeCustomerID,
			Status:               "active",
			CurrentPeriodStart:   time.Now(),
			CurrentPeriodEnd:     time.Now().AddDate(0, 1, 0),
		}
		err = repo.CreateUserSubscription(sub2)
		assert.Error(t, err, "Second subscription with duplicate Stripe ID should fail")

		// Verify the error is about unique constraint
		// Different databases have different error messages:
		// - PostgreSQL: "duplicate key value violates unique constraint"
		// - SQLite: "UNIQUE constraint failed"
		if err != nil {
			errMsg := err.Error()
			hasUniqueError := strings.Contains(errMsg, "duplicate") || strings.Contains(errMsg, "UNIQUE constraint")
			assert.True(t, hasUniqueError, "Error should mention unique constraint violation, got: %s", errMsg)
		}
	})

	// Test: Two subscriptions with different Stripe IDs should succeed
	t.Run("Different Stripe subscription IDs should succeed", func(t *testing.T) {
		stripeSubID1 := "sub_test_unique_1_" + uuid.New().String()[:8]
		stripeSubID2 := "sub_test_unique_2_" + uuid.New().String()[:8]
		stripeCustomerID1 := "cus_test_customer_1_" + uuid.New().String()[:8]
		stripeCustomerID2 := "cus_test_customer_2_" + uuid.New().String()[:8]

		user1ID := uuid.New().String()
		user2ID := uuid.New().String()

		// Create first subscription
		sub1 := &models.UserSubscription{
			UserID:               user1ID,
			SubscriptionPlanID:   paidPlan.ID,
			StripeSubscriptionID: &stripeSubID1,
			StripeCustomerID:     &stripeCustomerID1,
			Status:               "active",
			CurrentPeriodStart:   time.Now(),
			CurrentPeriodEnd:     time.Now().AddDate(0, 1, 0),
		}
		err := repo.CreateUserSubscription(sub1)
		assert.NoError(t, err, "First unique subscription should be created")

		if err == nil {
			defer db.Unscoped().Delete(sub1)
		}

		// Create second subscription with different Stripe ID
		sub2 := &models.UserSubscription{
			UserID:               user2ID,
			SubscriptionPlanID:   paidPlan.ID,
			StripeSubscriptionID: &stripeSubID2, // Different Stripe ID
			StripeCustomerID:     &stripeCustomerID2,
			Status:               "active",
			CurrentPeriodStart:   time.Now(),
			CurrentPeriodEnd:     time.Now().AddDate(0, 1, 0),
		}
		err = repo.CreateUserSubscription(sub2)
		assert.NoError(t, err, "Second unique subscription should be created")

		if err == nil {
			defer db.Unscoped().Delete(sub2)
		}
	})
}

// TestFreeTrialCreation_E2E is an end-to-end test that simulates
// the complete user registration flow with free trial assignment
func TestFreeTrialCreation_E2E(t *testing.T) {
	db := freshTestDB(t)

	// Create a free trial plan (mimicking production setup)
	trialPlan := &models.SubscriptionPlan{
		Name:                   "Trial_E2E",
		Description:            "14-day free trial",
		Priority:               0,
		PriceAmount:            0, // Free
		Currency:               "eur",
		BillingInterval:        "month",
		TrialDays:              14,
		MaxCourses:             5,
		MaxConcurrentTerminals: 1,
		MaxConcurrentUsers:     1,
		IsActive:               true,
	}
	err := db.Create(trialPlan).Error
	require.NoError(t, err, "Failed to create trial plan")

	// Clean up
	defer func() {
		db.Where("subscription_plan_id = ?", trialPlan.ID).Unscoped().Delete(&models.UserSubscription{})
		db.Where("subscription_id IN (SELECT id FROM user_subscriptions WHERE subscription_plan_id = ?)", trialPlan.ID).Unscoped().Delete(&models.UsageMetrics{})
		db.Unscoped().Delete(trialPlan)
	}()

	subscriptionService := services.NewSubscriptionService(db)

	t.Run("Complete free trial assignment flow", func(t *testing.T) {
		// Simulate new user registration
		userID := uuid.New().String()

		// Step 1: Create subscription (what happens in assignFreeTrialPlan)
		subscription, err := subscriptionService.CreateUserSubscription(userID, trialPlan.ID)
		require.NoError(t, err, "Free trial creation should not fail")
		require.NotNil(t, subscription, "Subscription should not be nil")

		// Step 2: Verify subscription properties
		assert.Equal(t, "active", subscription.Status, "Free trial should be active immediately")
		assert.Equal(t, userID, subscription.UserID)
		assert.Equal(t, trialPlan.ID, subscription.SubscriptionPlanID)
		assert.Nil(t, subscription.StripeSubscriptionID, "Free trial should not have Stripe subscription ID")
		assert.Nil(t, subscription.StripeCustomerID, "Free trial should not have Stripe customer ID")

		// Step 3: Verify period dates (should be set even for free plans)
		assert.False(t, subscription.CurrentPeriodStart.IsZero(), "Period start should be set")
		assert.False(t, subscription.CurrentPeriodEnd.IsZero(), "Period end should be set")
		assert.True(t, subscription.CurrentPeriodEnd.After(subscription.CurrentPeriodStart), "Period end should be after start")

		// Step 4: Verify we can retrieve the active subscription
		activeSubscription, err := subscriptionService.GetActiveUserSubscription(userID)
		require.NoError(t, err, "Should be able to get active subscription")
		require.NotNil(t, activeSubscription)
		assert.Equal(t, subscription.ID, activeSubscription.ID)
	})

	t.Run("Second user gets independent free trial", func(t *testing.T) {
		// Create another user
		user2ID := uuid.New().String()

		// Create free trial for second user
		subscription2, err := subscriptionService.CreateUserSubscription(user2ID, trialPlan.ID)
		require.NoError(t, err, "Second free trial creation should not fail")
		require.NotNil(t, subscription2, "Second subscription should not be nil")

		// Verify both NULL fields
		assert.Nil(t, subscription2.StripeSubscriptionID, "Second free trial should have NULL Stripe subscription ID")
		assert.Nil(t, subscription2.StripeCustomerID, "Second free trial should have NULL Stripe customer ID")

		// Verify it's a separate subscription
		assert.Equal(t, "active", subscription2.Status)
		assert.Equal(t, user2ID, subscription2.UserID)
	})
}

// TestDatabaseConstraints_DirectQuery verifies the database-level constraints
func TestDatabaseConstraints_DirectQuery(t *testing.T) {
	db := freshTestDB(t)

	t.Run("Verify conditional unique index exists", func(t *testing.T) {
		// Query PostgreSQL to verify the index exists and is conditional
		var indexes []struct {
			IndexName string
			IndexDef  string
		}

		query := `
			SELECT indexname as index_name, indexdef as index_def
			FROM pg_indexes
			WHERE tablename = 'user_subscriptions'
			AND indexname LIKE '%stripe%sub%'
		`

		err := db.Raw(query).Scan(&indexes).Error
		if err != nil {
			t.Logf("Could not query indexes (might not be PostgreSQL): %v", err)
			t.Skip("Skipping index verification (not PostgreSQL)")
		}

		// Verify we have a conditional index
		foundConditionalIndex := false
		for _, idx := range indexes {
			t.Logf("Found index: %s - %s", idx.IndexName, idx.IndexDef)

			// Check if it's a conditional index (contains WHERE clause)
			if len(idx.IndexDef) > 0 &&
				containsSubstring(idx.IndexDef, "WHERE") &&
				containsSubstring(idx.IndexDef, "IS NOT NULL") {
				foundConditionalIndex = true
				assert.Contains(t, idx.IndexDef, "stripe_subscription_id", "Index should be on stripe_subscription_id")
			}
		}

		if len(indexes) > 0 {
			assert.True(t, foundConditionalIndex, "Should have a conditional unique index with WHERE IS NOT NULL")
		}
	})
}

// Helper function to check if a string contains a substring
func containsSubstring(s, substr string) bool {
	if len(substr) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
