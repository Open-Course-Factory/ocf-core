package integration

import (
	"strings"
	"testing"
	"time"

	"soli/formations/src/payment/models"
	"soli/formations/src/payment/repositories"
	"soli/formations/src/payment/services"
	terminalModels "soli/formations/src/terminalTrainer/models"

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

	// Auto-migrate all payment models, plus the tables that QuotaService
	// touches when delegating quota decisions:
	//   - OrganizationSubscription: looked up by EffectivePlanService when
	//     resolving the effective plan (returns 0 rows in personal-quota
	//     tests, but the table must exist).
	//   - Terminal: live recalc for concurrent_terminals via
	//     CountUserOccupiedSlots.
	err = db.AutoMigrate(
		&models.SubscriptionPlan{},
		&models.UserSubscription{},
		&models.OrganizationSubscription{},
		&models.UsageMetrics{},
		&models.Invoice{},
		&models.PaymentMethod{},
		&models.BillingAddress{},
		&terminalModels.Terminal{},
	)
	require.NoError(t, err, "Failed to migrate test database")

	return db
}

// seedTestPlans creates the 4 terminal pricing plans with updated size naming (XS, S, M, L, XL)
func seedTestPlans(t *testing.T, db *gorm.DB) map[string]*models.SubscriptionPlan {
	// Helper to create string pointer
	strPtr := func(s string) *string { return &s }

	// Use test name as suffix to make Stripe price IDs unique per test
	testSuffix := "_" + strings.ReplaceAll(t.Name(), "/", "_")

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
			StripePriceID:             strPtr("price_test_trial" + testSuffix),
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
			StripePriceID:             strPtr("price_test_solo" + testSuffix),
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
			StripePriceID:             strPtr("price_test_trainer" + testSuffix),
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
			StripePriceID:             strPtr("price_test_organization" + testSuffix),
		},
	}

	for name, plan := range plans {
		err := db.Create(plan).Error
		require.NoError(t, err, "Failed to create %s plan", name)
	}

	return plans
}

// startTerminals creates n active terminal rows for the given user.
//
// This replaces the legacy pattern of calling
// service.IncrementUsage(userID, "concurrent_terminals", n) to fake
// terminal creation. Since #311 the SSOT for concurrent_terminals is
// the live count from the terminals table (see
// terminalModels.CountUserOccupiedSlots / OccupiesSlotScope), so tests
// that exercise the quota path must create real terminal rows. The
// usage_metrics row remains for other (non-live) metric types.
//
// Returns the created terminal IDs so callers can stop / delete a
// specific subset.
func startTerminals(t *testing.T, db *gorm.DB, userID string, n int) []uuid.UUID {
	t.Helper()
	// Create a single user terminal key (FK target). Reuse if already
	// present so subsequent calls don't conflict on api_key.
	var key terminalModels.UserTerminalKey
	err := db.Where("user_id = ?", userID).First(&key).Error
	if err != nil {
		key = terminalModels.UserTerminalKey{
			UserID:      userID,
			APIKey:      "test-key-" + userID,
			KeyName:     "Test Key",
			IsActive:    true,
			MaxSessions: 100,
		}
		require.NoError(t, db.Create(&key).Error)
	}

	ids := make([]uuid.UUID, 0, n)
	for i := 0; i < n; i++ {
		term := &terminalModels.Terminal{
			SessionID:         "session-" + userID + "-" + uuid.NewString(),
			UserID:            userID,
			Status:            "active",
			ExpiresAt:         time.Now().Add(1 * time.Hour),
			UserTerminalKeyID: key.ID,
		}
		require.NoError(t, db.Create(term).Error)
		ids = append(ids, term.ID)
	}
	return ids
}

// stopTerminals soft-deletes n terminal rows for the given user. Used
// to simulate the user stopping (releasing) terminals so the live quota
// recalc reflects the change.
func stopTerminals(t *testing.T, db *gorm.DB, userID string, n int) {
	t.Helper()
	var ids []uuid.UUID
	require.NoError(t, db.
		Model(&terminalModels.Terminal{}).
		Where("user_id = ? AND deleted_at IS NULL", userID).
		Limit(n).
		Pluck("id", &ids).Error)
	for _, id := range ids {
		require.NoError(t, db.Delete(&terminalModels.Terminal{}, "id = ?", id).Error)
	}
}

// createUserSubscription creates an active subscription for a user
func createUserSubscription(t *testing.T, db *gorm.DB, userID string, planID uuid.UUID) *models.UserSubscription {
	customerID := "cus_test_" + userID
	subscriptionID := "sub_test_" + userID

	subscription := &models.UserSubscription{
		UserID:               userID,
		SubscriptionPlanID:   planID,
		Status:               "active",
		CurrentPeriodStart:   time.Now(),
		CurrentPeriodEnd:     time.Now().AddDate(0, 1, 0),
		StripeCustomerID:     &customerID,
		StripeSubscriptionID: &subscriptionID,
	}

	err := db.Create(subscription).Error
	require.NoError(t, err, "Failed to create subscription")

	// Preload the plan
	err = db.Preload("SubscriptionPlan").First(subscription, subscription.ID).Error
	require.NoError(t, err, "Failed to preload subscription plan")

	return subscription
}

// newQuotaService builds the canonical QuotaService wiring used across
// these integration tests. Quota checks are exercised through this
// service directly — the legacy subscriptionService.CheckUsageLimit
// wrapper has been removed (SSOT consolidation).
func newQuotaService(db *gorm.DB) services.QuotaService {
	return services.NewQuotaService(db, services.NewEffectivePlanService(db))
}

// TestIntegration_TrialPlan_FullFlow tests the complete flow for Trial plan.
//
// Since #311 (live-recalc SSOT for concurrent_terminals), creating a
// terminal is modeled by inserting a real Terminal row via the
// startTerminals helper, not by incrementing the materialized
// usage_metrics counter.
func TestIntegration_TrialPlan_FullFlow(t *testing.T) {
	// Setup
	db := setupIntegrationDB(t)
	plans := seedTestPlans(t, db)
	quotaSvc := newQuotaService(db)

	userID := "trial-user-integration"
	createUserSubscription(t, db, userID, plans["Trial"].ID)

	t.Run("First terminal creation - should succeed", func(t *testing.T) {
		// Check limit
		check, err := quotaSvc.CheckUserQuota(userID, nil, "concurrent_terminals", 1)
		assert.NoError(t, err)
		assert.True(t, check.Allowed, "First terminal should be allowed")
		assert.Equal(t, int64(0), check.CurrentUsage)
		assert.Equal(t, int64(1), check.Limit)

		// Simulate terminal creation (real row).
		startTerminals(t, db, userID, 1)
	})

	t.Run("Second terminal creation - should fail", func(t *testing.T) {
		// Check limit
		check, err := quotaSvc.CheckUserQuota(userID, nil, "concurrent_terminals", 1)
		assert.NoError(t, err)
		assert.False(t, check.Allowed, "Second terminal should NOT be allowed on Trial plan")
		assert.Equal(t, int64(1), check.CurrentUsage)
		assert.Contains(t, check.Message, "limit exceeded")
	})

	t.Run("Stop first terminal - should allow new terminal", func(t *testing.T) {
		// Simulate terminal stop (release slot).
		stopTerminals(t, db, userID, 1)

		// Verify can create new terminal
		check, err := quotaSvc.CheckUserQuota(userID, nil, "concurrent_terminals", 1)
		assert.NoError(t, err)
		assert.True(t, check.Allowed, "Should allow terminal after stopping previous one")
		assert.Equal(t, int64(0), check.CurrentUsage)
	})
}

// TestIntegration_TrainerPlan_MultipleTerminals tests creating multiple
// terminals against the Trainer plan limit.
//
// Real Terminal rows are inserted via startTerminals / stopTerminals
// since live-recalc is the SSOT for concurrent_terminals.
func TestIntegration_TrainerPlan_MultipleTerminals(t *testing.T) {
	// Setup
	db := setupIntegrationDB(t)
	plans := seedTestPlans(t, db)
	quotaSvc := newQuotaService(db)

	userID := "trainer-user-integration"
	createUserSubscription(t, db, userID, plans["Trainer"].ID)

	// Trainer plan allows 3 concurrent terminals
	t.Run("Create 3 terminals sequentially", func(t *testing.T) {
		for i := 1; i <= 3; i++ {
			check, err := quotaSvc.CheckUserQuota(userID, nil, "concurrent_terminals", 1)
			assert.NoError(t, err)
			assert.True(t, check.Allowed, "Terminal %d should be allowed", i)

			startTerminals(t, db, userID, 1)

			// Verify current usage
			check2, _ := quotaSvc.CheckUserQuota(userID, nil, "concurrent_terminals", 0)
			assert.Equal(t, int64(i), check2.CurrentUsage, "Current usage should be %d", i)
		}
	})

	t.Run("Fourth terminal should fail", func(t *testing.T) {
		check, err := quotaSvc.CheckUserQuota(userID, nil, "concurrent_terminals", 1)
		assert.NoError(t, err)
		assert.False(t, check.Allowed, "Fourth terminal should NOT be allowed")
		assert.Equal(t, int64(3), check.CurrentUsage)
	})

	t.Run("Stop 2 terminals and create 2 new ones", func(t *testing.T) {
		// Stop 2 terminals
		stopTerminals(t, db, userID, 2)

		// Verify current usage
		check, _ := quotaSvc.CheckUserQuota(userID, nil, "concurrent_terminals", 0)
		assert.Equal(t, int64(1), check.CurrentUsage)

		// Should be able to create 2 more
		for i := 0; i < 2; i++ {
			check, err := quotaSvc.CheckUserQuota(userID, nil, "concurrent_terminals", 1)
			assert.NoError(t, err)
			assert.True(t, check.Allowed)
			startTerminals(t, db, userID, 1)
		}

		// Should be at limit again
		check, _ = quotaSvc.CheckUserQuota(userID, nil, "concurrent_terminals", 1)
		assert.False(t, check.Allowed)
		assert.Equal(t, int64(3), check.CurrentUsage)
	})
}

// TestIntegration_OrganizationPlan_HighConcurrency tests organization plan with many terminals
func TestIntegration_OrganizationPlan_HighConcurrency(t *testing.T) {
	// Setup
	db := setupIntegrationDB(t)
	plans := seedTestPlans(t, db)
	quotaSvc := newQuotaService(db)

	userID := "org-user-integration"
	createUserSubscription(t, db, userID, plans["Organization"].ID)

	// Organization plan allows 10 concurrent terminals
	t.Run("Create 10 terminals", func(t *testing.T) {
		for i := 1; i <= 10; i++ {
			check, err := quotaSvc.CheckUserQuota(userID, nil, "concurrent_terminals", 1)
			assert.NoError(t, err)
			assert.True(t, check.Allowed, "Terminal %d/%d should be allowed", i, 10)

			startTerminals(t, db, userID, 1)
		}

		// Verify final state
		check, _ := quotaSvc.CheckUserQuota(userID, nil, "concurrent_terminals", 0)
		assert.Equal(t, int64(10), check.CurrentUsage)
		assert.Equal(t, int64(10), check.Limit)
	})

	t.Run("11th terminal should fail", func(t *testing.T) {
		check, err := quotaSvc.CheckUserQuota(userID, nil, "concurrent_terminals", 1)
		assert.NoError(t, err)
		assert.False(t, check.Allowed)
	})

	t.Run("Stop all and verify reset", func(t *testing.T) {
		// Stop all 10 terminals
		stopTerminals(t, db, userID, 10)

		// Verify reset to 0
		check, _ := quotaSvc.CheckUserQuota(userID, nil, "concurrent_terminals", 0)
		assert.Equal(t, int64(0), check.CurrentUsage)

		// Should be able to create new terminal
		check, err := quotaSvc.CheckUserQuota(userID, nil, "concurrent_terminals", 1)
		assert.NoError(t, err)
		assert.True(t, check.Allowed)
	})
}

// TestIntegration_PlanComparison tests different plan limits side by side
func TestIntegration_PlanComparison(t *testing.T) {
	// Setup
	db := setupIntegrationDB(t)
	plans := seedTestPlans(t, db)
	quotaSvc := newQuotaService(db)

	users := map[string]struct {
		planName     string
		maxTerminals int
	}{
		"user-trial":   {"Trial", 1},
		"user-solo":    {"Solo", 1},
		"user-trainer": {"Trainer", 3},
		"user-org":     {"Organization", 10},
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
				check, err := quotaSvc.CheckUserQuota(userID, nil, "concurrent_terminals", 1)
				assert.NoError(t, err)
				assert.True(t, check.Allowed, "%s: terminal %d/%d should be allowed", config.planName, i+1, config.maxTerminals)
				startTerminals(t, db, userID, 1)
			}

			// One more should fail
			check, err := quotaSvc.CheckUserQuota(userID, nil, "concurrent_terminals", 1)
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
	quotaSvc := newQuotaService(db)
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
		// Increment metric to limit (3 for Trainer plan).
		service.IncrementUsage(userID, "concurrent_terminals", 2)

		// Live recalc (SSOT for concurrent_terminals since #311) reads
		// from terminals table — create real rows so the quota check
		// reflects an "at limit" state.
		startTerminals(t, db, userID, 3)

		// Try to increment beyond (should fail check).
		check, _ := quotaSvc.CheckUserQuota(userID, nil, "concurrent_terminals", 1)
		assert.False(t, check.Allowed)

		// Current value of the materialized metric should still reflect
		// the accumulated increments (this subtest is about persistence
		// of the metric row, not the SSOT count).
		metric, _ := repo.GetUserUsageMetrics(userID, "concurrent_terminals")
		assert.Equal(t, int64(3), metric.CurrentValue)
	})
}

// TestIntegration_NoSubscription tests behavior without active subscription.
//
// QuotaService.CheckUserQuota returns an error when no effective plan can
// be resolved — middleware/controllers translate that error into an HTTP
// 4xx for the caller. This is the honest production contract; the legacy
// CheckUsageLimit wrapper used to fake a friendly UsageLimitCheck envelope
// with Allowed:false and a "No active subscription" message, but that
// wrapper has been removed as part of the SSOT consolidation.
func TestIntegration_NoSubscription(t *testing.T) {
	// Setup
	db := setupIntegrationDB(t)
	seedTestPlans(t, db)
	quotaSvc := newQuotaService(db)

	userID := "no-sub-user"

	t.Run("User without subscription cannot create terminals", func(t *testing.T) {
		check, err := quotaSvc.CheckUserQuota(userID, nil, "concurrent_terminals", 1)
		assert.Error(t, err, "no resolvable plan must surface as an error")
		assert.Nil(t, check, "QuotaService returns nil check when plan resolution fails")
	})
}

// TestIntegration_PlanUpgrade simulates upgrading from Trial to Trainer.
//
// Since #311 live recalc is the SSOT for concurrent_terminals: real
// Terminal rows drive the count, and plan.MaxConcurrentTerminals drives
// the limit (read off the user's effective plan, which now reflects
// UpgradeUserPlan's correctly-persisted SubscriptionPlanID — see #315).
func TestIntegration_PlanUpgrade(t *testing.T) {
	// Setup
	db := setupIntegrationDB(t)
	plans := seedTestPlans(t, db)
	service := services.NewSubscriptionService(db)
	quotaSvc := newQuotaService(db)

	userID := "upgrade-user"

	// Create user subscription at the start
	createUserSubscription(t, db, userID, plans["Trial"].ID)

	// Create 1 terminal (max for Trial)
	startTerminals(t, db, userID, 1)

	// Cannot create second on Trial plan
	check, _ := quotaSvc.CheckUserQuota(userID, nil, "concurrent_terminals", 1)
	assert.False(t, check.Allowed, "Trial plan should block 2nd terminal")
	assert.Equal(t, int64(1), check.Limit)

	// Upgrade to Trainer plan
	subscription, err := service.UpgradeUserPlan(userID, plans["Trainer"].ID, "")
	assert.NoError(t, err)
	assert.NotNil(t, subscription)

	// After upgrade - can create 2 more terminals (3 total)
	for i := 0; i < 2; i++ {
		check, err := quotaSvc.CheckUserQuota(userID, nil, "concurrent_terminals", 1)
		assert.NoError(t, err)
		assert.True(t, check.Allowed, "Should allow terminal %d after upgrade", i+2)
		assert.Equal(t, int64(3), check.Limit, "Limit should be updated to Trainer plan limit")
		startTerminals(t, db, userID, 1)
	}

	// Now at limit (3 terminals)
	check, _ = quotaSvc.CheckUserQuota(userID, nil, "concurrent_terminals", 1)
	assert.False(t, check.Allowed, "Should not allow 4th terminal")
	assert.Equal(t, int64(3), check.CurrentUsage)
	assert.Equal(t, int64(3), check.Limit)

	// Downgrade back to Trial (demonstrates limit updates work both ways)
	_, err = service.UpgradeUserPlan(userID, plans["Trial"].ID, "")
	assert.NoError(t, err)

	// Now limit is 1, but usage is still 3 - should not allow more
	check, _ = quotaSvc.CheckUserQuota(userID, nil, "concurrent_terminals", 1)
	assert.False(t, check.Allowed, "Should not allow more terminals when over limit after downgrade")
	assert.Equal(t, int64(3), check.CurrentUsage)
	assert.Equal(t, int64(1), check.Limit, "Limit should be updated to Trial plan limit")
}

// TestUpgradeUserPlan_PersistsNewPlanID is the kill-switch test for the
// latent bug in UpgradeUserPlan where tx.Model(subscription).Update(col, value)
// emitted UPDATE ... SET subscription_plan_id = <OLD ID> instead of <newPlanID>.
//
// The pre-#311 TestIntegration_PlanUpgrade did not catch this because it only
// asserted on metrics.limit_value (which UpgradeUserPlan writes correctly
// via its second Updates(map[...]) call), not on the subscription's
// SubscriptionPlanID column itself. Now that the quota path goes through
// QuotaService (live recalc against the resolved plan rather than the
// materialized limit_value), the integration test would have failed without
// this fix in place.
//
// This test asserts on the persisted column directly, so it stays a kill-switch
// regardless of how the rest of the quota path evolves.
func TestUpgradeUserPlan_PersistsNewPlanID(t *testing.T) {
	db := setupIntegrationDB(t)
	plans := seedTestPlans(t, db)
	service := services.NewSubscriptionService(db)

	userID := "persist-plan-id-user"
	createUserSubscription(t, db, userID, plans["Trial"].ID)

	trialID := plans["Trial"].ID
	trainerID := plans["Trainer"].ID
	require.NotEqual(t, trialID, trainerID, "test plan IDs must differ")

	// Upgrade from Trial to Trainer.
	returned, err := service.UpgradeUserPlan(userID, trainerID, "")
	require.NoError(t, err)
	require.NotNil(t, returned)

	// The returned subscription must reference the new plan.
	assert.Equal(t, trainerID, returned.SubscriptionPlanID,
		"UpgradeUserPlan return value must reference the new plan")

	// Reload the subscription from the DB and verify the column was
	// actually written. This is the assertion the legacy test was missing.
	reloaded, err := service.GetActiveUserSubscription(userID)
	require.NoError(t, err)
	assert.Equal(t, trainerID, reloaded.SubscriptionPlanID,
		"persisted subscription_plan_id must be the new plan ID; if this fails, "+
			"UpgradeUserPlan's GORM Update is writing the loaded struct value "+
			"(old plan ID) instead of the newPlanID argument")
}
