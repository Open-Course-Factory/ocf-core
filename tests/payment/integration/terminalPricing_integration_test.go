package integration

import (
	"testing"
	"time"

	"soli/formations/src/payment/models"
	"soli/formations/src/payment/repositories"
	"soli/formations/src/payment/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupIntegrationDB creates a test database with all required tables
func setupIntegrationDB(t *testing.T) *gorm.DB {
	// Use shared cache mode so all connections see the same in-memory database
	// This is critical for SQLite - each :memory: connection gets its own DB
	// With cache=shared, multiple connections share the same in-memory instance
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info), // Show queries for debugging
	})
	require.NoError(t, err, "Failed to open test database")

	// Auto-migrate all payment models
	err = db.AutoMigrate(
		&models.SubscriptionPlan{},
		&models.UserSubscription{},
		&models.UsageMetrics{},
		&models.Invoice{},
		&models.PaymentMethod{},
		&models.BillingAddress{},
	)
	require.NoError(t, err, "Failed to migrate test database")

	return db
}

// seedTestPlans creates the 4 terminal pricing plans with updated size naming (XS, S, M, L, XL)
func seedTestPlans(t *testing.T, db *gorm.DB) map[string]*models.SubscriptionPlan {
	plans := map[string]*models.SubscriptionPlan{
		"Trial": {
			Name:                      "Trial",
			PriceAmount:               0,
			Currency:                  "eur",
			BillingInterval:           "month",
			MaxSessionDurationMinutes: 60,
			MaxConcurrentTerminals:    1,
			AllowedMachineSizes:       []string{"XS"}, // XS size only
			NetworkAccessEnabled:      false,
			DataPersistenceEnabled:    false,
			DataPersistenceGB:         0,
			AllowedTemplates:          []string{"ubuntu-basic", "alpine-basic"},
			IsActive:                  true,
			RequiredRole:              "member",
		},
		"Solo": {
			Name:                      "Solo",
			PriceAmount:               900,
			Currency:                  "eur",
			BillingInterval:           "month",
			MaxSessionDurationMinutes: 480,
			MaxConcurrentTerminals:    1,
			AllowedMachineSizes:       []string{"XS", "S"}, // XS + S sizes
			NetworkAccessEnabled:      true,
			DataPersistenceEnabled:    true,
			DataPersistenceGB:         2,
			AllowedTemplates:          []string{"ubuntu-basic", "ubuntu-dev", "alpine-basic", "debian-basic", "python", "nodejs", "docker"},
			IsActive:                  true,
			RequiredRole:              "member",
		},
		"Trainer": {
			Name:                      "Trainer",
			PriceAmount:               1900,
			Currency:                  "eur",
			BillingInterval:           "month",
			MaxSessionDurationMinutes: 480,
			MaxConcurrentTerminals:    3,
			AllowedMachineSizes:       []string{"XS", "S", "M"}, // XS, S, M sizes
			NetworkAccessEnabled:      true,
			DataPersistenceEnabled:    true,
			DataPersistenceGB:         5,
			AllowedTemplates:          []string{"ubuntu-basic", "ubuntu-dev", "alpine-basic", "debian-basic", "python", "nodejs", "docker"},
			IsActive:                  true,
			RequiredRole:              "trainer",
		},
		"Organization": {
			Name:                      "Organization",
			PriceAmount:               4900,
			Currency:                  "eur",
			BillingInterval:           "month",
			MaxSessionDurationMinutes: 480,
			MaxConcurrentTerminals:    10,
			AllowedMachineSizes:       []string{"all"}, // All sizes: XS, S, M, L, XL
			NetworkAccessEnabled:      true,
			DataPersistenceEnabled:    true,
			DataPersistenceGB:         20,
			AllowedTemplates:          []string{"all"},
			IsActive:                  true,
			RequiredRole:              "organization",
		},
	}

	for name, plan := range plans {
		err := db.Create(plan).Error
		require.NoError(t, err, "Failed to create %s plan", name)
	}

	return plans
}

// createUserSubscription creates an active subscription for a user
func createUserSubscription(t *testing.T, db *gorm.DB, userID string, planID uuid.UUID) *models.UserSubscription {
	subscription := &models.UserSubscription{
		UserID:              userID,
		SubscriptionPlanID:  planID,
		Status:              "active",
		CurrentPeriodStart:  time.Now(),
		CurrentPeriodEnd:    time.Now().AddDate(0, 1, 0),
		StripeCustomerID:    "cus_test_" + userID,
		StripeSubscriptionID: "sub_test_" + userID,
	}

	err := db.Create(subscription).Error
	require.NoError(t, err, "Failed to create subscription")

	// Preload the plan
	err = db.Preload("SubscriptionPlan").First(subscription, subscription.ID).Error
	require.NoError(t, err, "Failed to preload subscription plan")

	return subscription
}

// TestIntegration_TrialPlan_FullFlow tests the complete flow for Trial plan
func TestIntegration_TrialPlan_FullFlow(t *testing.T) {
	// Setup
	db := setupIntegrationDB(t)
	plans := seedTestPlans(t, db)
	service := services.NewSubscriptionService(db)
	repo := repositories.NewPaymentRepository(db)

	userID := "trial-user-integration"
	subscription := createUserSubscription(t, db, userID, plans["Trial"].ID)

	t.Run("First terminal creation - should succeed", func(t *testing.T) {
		// Check limit
		check, err := service.CheckUsageLimit(userID, "concurrent_terminals", 1)
		assert.NoError(t, err)
		assert.True(t, check.Allowed, "First terminal should be allowed")
		assert.Equal(t, int64(0), check.CurrentUsage)
		assert.Equal(t, int64(1), check.Limit)

		// Simulate terminal creation
		err = service.IncrementUsage(userID, "concurrent_terminals", 1)
		assert.NoError(t, err)

		// Verify metric was created
		metric, err := repo.GetUserUsageMetrics(userID, "concurrent_terminals")
		assert.NoError(t, err)
		assert.Equal(t, int64(1), metric.CurrentValue)
		assert.Equal(t, subscription.ID, metric.SubscriptionID)
	})

	t.Run("Second terminal creation - should fail", func(t *testing.T) {
		// Check limit
		check, err := service.CheckUsageLimit(userID, "concurrent_terminals", 1)
		assert.NoError(t, err)
		assert.False(t, check.Allowed, "Second terminal should NOT be allowed on Trial plan")
		assert.Equal(t, int64(1), check.CurrentUsage)
		assert.Contains(t, check.Message, "limit exceeded")
	})

	t.Run("Stop first terminal - should allow new terminal", func(t *testing.T) {
		// Simulate terminal stop (decrement)
		err := service.IncrementUsage(userID, "concurrent_terminals", -1)
		assert.NoError(t, err)

		// Verify can create new terminal
		check, err := service.CheckUsageLimit(userID, "concurrent_terminals", 1)
		assert.NoError(t, err)
		assert.True(t, check.Allowed, "Should allow terminal after stopping previous one")
		assert.Equal(t, int64(0), check.CurrentUsage)
	})
}

// TestIntegration_TrainerPlan_MultipleTerminals tests creating multiple terminals
func TestIntegration_TrainerPlan_MultipleTerminals(t *testing.T) {
	// Setup
	db := setupIntegrationDB(t)
	plans := seedTestPlans(t, db)
	service := services.NewSubscriptionService(db)

	userID := "trainer-user-integration"
	createUserSubscription(t, db, userID, plans["Trainer"].ID)

	// Trainer plan allows 3 concurrent terminals
	t.Run("Create 3 terminals sequentially", func(t *testing.T) {
		for i := 1; i <= 3; i++ {
			check, err := service.CheckUsageLimit(userID, "concurrent_terminals", 1)
			assert.NoError(t, err)
			assert.True(t, check.Allowed, "Terminal %d should be allowed", i)

			err = service.IncrementUsage(userID, "concurrent_terminals", 1)
			assert.NoError(t, err)

			// Verify current usage
			check2, _ := service.CheckUsageLimit(userID, "concurrent_terminals", 0)
			assert.Equal(t, int64(i), check2.CurrentUsage, "Current usage should be %d", i)
		}
	})

	t.Run("Fourth terminal should fail", func(t *testing.T) {
		check, err := service.CheckUsageLimit(userID, "concurrent_terminals", 1)
		assert.NoError(t, err)
		assert.False(t, check.Allowed, "Fourth terminal should NOT be allowed")
		assert.Equal(t, int64(3), check.CurrentUsage)
	})

	t.Run("Stop 2 terminals and create 2 new ones", func(t *testing.T) {
		// Stop 2 terminals
		service.IncrementUsage(userID, "concurrent_terminals", -2)

		// Verify current usage
		check, _ := service.CheckUsageLimit(userID, "concurrent_terminals", 0)
		assert.Equal(t, int64(1), check.CurrentUsage)

		// Should be able to create 2 more
		for i := 0; i < 2; i++ {
			check, err := service.CheckUsageLimit(userID, "concurrent_terminals", 1)
			assert.NoError(t, err)
			assert.True(t, check.Allowed)
			service.IncrementUsage(userID, "concurrent_terminals", 1)
		}

		// Should be at limit again
		check, _ = service.CheckUsageLimit(userID, "concurrent_terminals", 1)
		assert.False(t, check.Allowed)
		assert.Equal(t, int64(3), check.CurrentUsage)
	})
}

// TestIntegration_OrganizationPlan_HighConcurrency tests organization plan with many terminals
func TestIntegration_OrganizationPlan_HighConcurrency(t *testing.T) {
	// Setup
	db := setupIntegrationDB(t)
	plans := seedTestPlans(t, db)
	service := services.NewSubscriptionService(db)

	userID := "org-user-integration"
	createUserSubscription(t, db, userID, plans["Organization"].ID)

	// Organization plan allows 10 concurrent terminals
	t.Run("Create 10 terminals", func(t *testing.T) {
		for i := 1; i <= 10; i++ {
			check, err := service.CheckUsageLimit(userID, "concurrent_terminals", 1)
			assert.NoError(t, err)
			assert.True(t, check.Allowed, "Terminal %d/%d should be allowed", i, 10)

			err = service.IncrementUsage(userID, "concurrent_terminals", 1)
			assert.NoError(t, err)
		}

		// Verify final state
		check, _ := service.CheckUsageLimit(userID, "concurrent_terminals", 0)
		assert.Equal(t, int64(10), check.CurrentUsage)
		assert.Equal(t, int64(10), check.Limit)
	})

	t.Run("11th terminal should fail", func(t *testing.T) {
		check, err := service.CheckUsageLimit(userID, "concurrent_terminals", 1)
		assert.NoError(t, err)
		assert.False(t, check.Allowed)
	})

	t.Run("Stop all and verify reset", func(t *testing.T) {
		// Stop all 10 terminals
		err := service.IncrementUsage(userID, "concurrent_terminals", -10)
		assert.NoError(t, err)

		// Verify reset to 0
		check, _ := service.CheckUsageLimit(userID, "concurrent_terminals", 0)
		assert.Equal(t, int64(0), check.CurrentUsage)

		// Should be able to create new terminal
		check, err = service.CheckUsageLimit(userID, "concurrent_terminals", 1)
		assert.NoError(t, err)
		assert.True(t, check.Allowed)
	})
}

// TestIntegration_PlanComparison tests different plan limits side by side
func TestIntegration_PlanComparison(t *testing.T) {
	// Setup
	db := setupIntegrationDB(t)
	plans := seedTestPlans(t, db)
	service := services.NewSubscriptionService(db)

	users := map[string]struct {
		planName string
		maxTerminals int
	}{
		"user-trial":  {"Trial", 1},
		"user-solo":   {"Solo", 1},
		"user-trainer": {"Trainer", 3},
		"user-org":    {"Organization", 10},
	}

	// Create subscriptions for each user
	for userID, config := range users {
		createUserSubscription(t, db, userID, plans[config.planName].ID)
	}

	// Test each user's limit
	for userID, config := range users {
		t.Run(config.planName+" plan limits", func(t *testing.T) {
			// Create up to max
			for i := 0; i < config.maxTerminals; i++ {
				check, err := service.CheckUsageLimit(userID, "concurrent_terminals", 1)
				assert.NoError(t, err)
				assert.True(t, check.Allowed, "%s: terminal %d/%d should be allowed", config.planName, i+1, config.maxTerminals)
				service.IncrementUsage(userID, "concurrent_terminals", 1)
			}

			// One more should fail
			check, err := service.CheckUsageLimit(userID, "concurrent_terminals", 1)
			assert.NoError(t, err)
			assert.False(t, check.Allowed, "%s: should not allow beyond limit", config.planName)
			assert.Equal(t, int64(config.maxTerminals), check.CurrentUsage)
		})
	}
}

// TestIntegration_UsageMetricsPersistence tests that metrics are properly persisted
func TestIntegration_UsageMetricsPersistence(t *testing.T) {
	// Setup
	db := setupIntegrationDB(t)
	plans := seedTestPlans(t, db)
	service := services.NewSubscriptionService(db)
	repo := repositories.NewPaymentRepository(db)

	userID := "metrics-user"
	subscription := createUserSubscription(t, db, userID, plans["Trainer"].ID)

	t.Run("Create metric and verify persistence", func(t *testing.T) {
		// Increment usage
		err := service.IncrementUsage(userID, "concurrent_terminals", 2)
		assert.NoError(t, err)

		// Retrieve metric directly from repository
		metric, err := repo.GetUserUsageMetrics(userID, "concurrent_terminals")
		assert.NoError(t, err)
		assert.Equal(t, int64(2), metric.CurrentValue)
		assert.Equal(t, int64(3), metric.LimitValue) // Trainer plan limit
		assert.Equal(t, subscription.ID, metric.SubscriptionID)
		assert.Equal(t, "concurrent_terminals", metric.MetricType)
	})

	t.Run("Update metric and verify changes", func(t *testing.T) {
		// Decrement
		err := service.IncrementUsage(userID, "concurrent_terminals", -1)
		assert.NoError(t, err)

		// Verify updated value
		metric, err := repo.GetUserUsageMetrics(userID, "concurrent_terminals")
		assert.NoError(t, err)
		assert.Equal(t, int64(1), metric.CurrentValue)
	})

	t.Run("Increment beyond limit should still update metric", func(t *testing.T) {
		// Increment to limit
		service.IncrementUsage(userID, "concurrent_terminals", 2)

		// Try to increment beyond (should fail check, but metric still updates if we force it)
		check, _ := service.CheckUsageLimit(userID, "concurrent_terminals", 1)
		assert.False(t, check.Allowed)

		// Current value should be at limit
		metric, _ := repo.GetUserUsageMetrics(userID, "concurrent_terminals")
		assert.Equal(t, int64(3), metric.CurrentValue)
	})
}

// TestIntegration_NoSubscription tests behavior without active subscription
func TestIntegration_NoSubscription(t *testing.T) {
	// Setup
	db := setupIntegrationDB(t)
	seedTestPlans(t, db)
	service := services.NewSubscriptionService(db)

	userID := "no-sub-user"

	t.Run("User without subscription cannot create terminals", func(t *testing.T) {
		check, err := service.CheckUsageLimit(userID, "concurrent_terminals", 1)
		assert.NoError(t, err)
		assert.False(t, check.Allowed)
		assert.Contains(t, check.Message, "No active subscription")
	})
}

// TestIntegration_PlanUpgrade simulates upgrading from Trial to Trainer
func TestIntegration_PlanUpgrade(t *testing.T) {
	// Setup
	db := setupIntegrationDB(t)
	plans := seedTestPlans(t, db)
	service := services.NewSubscriptionService(db)

	userID := "upgrade-user"

	// Create user subscription at the start
	createUserSubscription(t, db, userID, plans["Trial"].ID)

	// Create 1 terminal (max for Trial)
	err := service.IncrementUsage(userID, "concurrent_terminals", 1)
	assert.NoError(t, err)

	// Cannot create second on Trial plan
	check, _ := service.CheckUsageLimit(userID, "concurrent_terminals", 1)
	assert.False(t, check.Allowed, "Trial plan should block 2nd terminal")
	assert.Equal(t, int64(1), check.Limit)

	// Upgrade to Trainer plan
	subscription, err := service.UpgradeUserPlan(userID, plans["Trainer"].ID)
	assert.NoError(t, err)
	assert.NotNil(t, subscription)

	// After upgrade - can create 2 more terminals (3 total)
	for i := 0; i < 2; i++ {
		check, err := service.CheckUsageLimit(userID, "concurrent_terminals", 1)
		assert.NoError(t, err)
		assert.True(t, check.Allowed, "Should allow terminal %d after upgrade", i+2)
		assert.Equal(t, int64(3), check.Limit, "Limit should be updated to Trainer plan limit")
		service.IncrementUsage(userID, "concurrent_terminals", 1)
	}

	// Now at limit (3 terminals)
	check, _ = service.CheckUsageLimit(userID, "concurrent_terminals", 1)
	assert.False(t, check.Allowed, "Should not allow 4th terminal")
	assert.Equal(t, int64(3), check.CurrentUsage)
	assert.Equal(t, int64(3), check.Limit)

	// Downgrade back to Trial (demonstrates limit updates work both ways)
	_, err = service.UpgradeUserPlan(userID, plans["Trial"].ID)
	assert.NoError(t, err)

	// Now limit is 1, but usage is still 3 - should not allow more
	check, _ = service.CheckUsageLimit(userID, "concurrent_terminals", 1)
	assert.False(t, check.Allowed, "Should not allow more terminals when over limit after downgrade")
	assert.Equal(t, int64(3), check.CurrentUsage)
	assert.Equal(t, int64(1), check.Limit, "Limit should be updated to Trial plan limit")
}
