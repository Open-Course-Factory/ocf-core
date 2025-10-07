package integration

import (
	"testing"
	"time"

	paymentModels "soli/formations/src/payment/models"
	paymentServices "soli/formations/src/payment/services"
	terminalModels "soli/formations/src/terminalTrainer/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUsageMetrics_ConcurrentTerminals_OnlyCountsActive tests that concurrent_terminals metric
// only counts terminals with status='active', not stopped/expired ones
func TestUsageMetrics_ConcurrentTerminals_OnlyCountsActive(t *testing.T) {
	// Setup database
	db := setupIntegrationDB(t)
	plans := seedTestPlans(t, db)
	service := paymentServices.NewSubscriptionService(db)

	// Migrate terminal tables
	err := db.AutoMigrate(&terminalModels.Terminal{}, &terminalModels.UserTerminalKey{})
	require.NoError(t, err)

	// Create test user with subscription
	userID := "usage-test-user"
	subscription := createUserSubscription(t, db, userID, plans["Trainer"].ID)

	// Create user terminal key (required foreign key)
	userKey := &terminalModels.UserTerminalKey{
		UserID:      userID,
		APIKey:      "test-key-123",
		KeyName:     "Test Key",
		IsActive:    true,
		MaxSessions: 5,
	}
	err = db.Create(userKey).Error
	require.NoError(t, err)

	// Create 2 active terminals
	activeTerminal1 := &terminalModels.Terminal{
		SessionID:         "session-active-1",
		UserID:            userID,
		Status:            "active",
		ExpiresAt:         time.Now().Add(1 * time.Hour),
		UserTerminalKeyID: userKey.ID,
	}
	err = db.Create(activeTerminal1).Error
	require.NoError(t, err)

	activeTerminal2 := &terminalModels.Terminal{
		SessionID:         "session-active-2",
		UserID:            userID,
		Status:            "active",
		ExpiresAt:         time.Now().Add(1 * time.Hour),
		UserTerminalKeyID: userKey.ID,
	}
	err = db.Create(activeTerminal2).Error
	require.NoError(t, err)

	// Create 1 stopped terminal (should NOT be counted)
	stoppedTerminal := &terminalModels.Terminal{
		SessionID:         "session-stopped",
		UserID:            userID,
		Status:            "stopped",
		ExpiresAt:         time.Now().Add(1 * time.Hour),
		UserTerminalKeyID: userKey.ID,
	}
	err = db.Create(stoppedTerminal).Error
	require.NoError(t, err)

	// Create 1 expired terminal (should NOT be counted)
	expiredTerminal := &terminalModels.Terminal{
		SessionID:         "session-expired",
		UserID:            userID,
		Status:            "expired",
		ExpiresAt:         time.Now().Add(-1 * time.Hour),
		UserTerminalKeyID: userKey.ID,
	}
	err = db.Create(expiredTerminal).Error
	require.NoError(t, err)

	// Manually create usage metric with WRONG value (simulating out-of-sync state)
	// This simulates the bug where the metric wasn't decremented when terminals stopped
	wrongMetric := &paymentModels.UsageMetrics{
		UserID:         userID,
		SubscriptionID: subscription.ID,
		MetricType:     "concurrent_terminals",
		CurrentValue:   4, // WRONG: Should be 2, not 4!
		LimitValue:     3, // Trainer plan allows 3 concurrent
		PeriodStart:    time.Now().AddDate(0, 0, -1),
		PeriodEnd:      time.Now().AddDate(0, 1, 0),
		LastUpdated:    time.Now(),
	}
	err = db.Create(wrongMetric).Error
	require.NoError(t, err)

	t.Run("GetUserUsageMetrics should return real-time active terminal count", func(t *testing.T) {
		// Call the service method
		metrics, err := service.GetUserUsageMetrics(userID)
		require.NoError(t, err)
		require.NotNil(t, metrics)

		// Find the concurrent_terminals metric
		var concurrentTerminalsMetric *paymentModels.UsageMetrics
		for _, m := range *metrics {
			if m.MetricType == "concurrent_terminals" {
				concurrentTerminalsMetric = &m
				break
			}
		}

		require.NotNil(t, concurrentTerminalsMetric, "concurrent_terminals metric should exist")

		// CRITICAL TEST: Should return 2 (only active terminals), not 4 (stored wrong value)
		assert.Equal(t, int64(2), concurrentTerminalsMetric.CurrentValue,
			"Should count only active terminals (2), not stopped/expired ones")
		assert.Equal(t, int64(3), concurrentTerminalsMetric.LimitValue,
			"Limit should match subscription plan")
	})

	t.Run("Verify database still has wrong value but API returns correct count", func(t *testing.T) {
		// Check database value is still wrong
		var dbMetric paymentModels.UsageMetrics
		err := db.Where("user_id = ? AND metric_type = ?", userID, "concurrent_terminals").First(&dbMetric).Error
		require.NoError(t, err)
		assert.Equal(t, int64(4), dbMetric.CurrentValue, "Database should still have wrong value")

		// But GetUserUsageMetrics should return corrected value
		metrics, err := service.GetUserUsageMetrics(userID)
		require.NoError(t, err)

		for _, m := range *metrics {
			if m.MetricType == "concurrent_terminals" {
				assert.Equal(t, int64(2), m.CurrentValue, "API should return real-time count")
			}
		}
	})
}

// TestUsageMetrics_ConcurrentTerminals_ZeroActiveTerminals tests the case where all terminals are stopped
func TestUsageMetrics_ConcurrentTerminals_ZeroActiveTerminals(t *testing.T) {
	// Setup database
	db := setupIntegrationDB(t)
	plans := seedTestPlans(t, db)
	service := paymentServices.NewSubscriptionService(db)

	// Migrate terminal tables
	err := db.AutoMigrate(&terminalModels.Terminal{}, &terminalModels.UserTerminalKey{})
	require.NoError(t, err)

	// Create test user
	userID := "zero-terminals-user"
	subscription := createUserSubscription(t, db, userID, plans["Trial"].ID)

	// Create user terminal key
	userKey := &terminalModels.UserTerminalKey{
		UserID:      userID,
		APIKey:      "test-key-456",
		KeyName:     "Test Key 2",
		IsActive:    true,
		MaxSessions: 5,
	}
	err = db.Create(userKey).Error
	require.NoError(t, err)

	// Create only stopped terminals (no active ones)
	stoppedTerminal := &terminalModels.Terminal{
		SessionID:         "session-stopped-only",
		UserID:            userID,
		Status:            "stopped",
		ExpiresAt:         time.Now().Add(1 * time.Hour),
		UserTerminalKeyID: userKey.ID,
	}
	err = db.Create(stoppedTerminal).Error
	require.NoError(t, err)

	// Create metric with wrong value (1 instead of 0)
	wrongMetric := &paymentModels.UsageMetrics{
		UserID:         userID,
		SubscriptionID: subscription.ID,
		MetricType:     "concurrent_terminals",
		CurrentValue:   1, // WRONG: Should be 0!
		LimitValue:     1, // Trial plan
		PeriodStart:    time.Now().AddDate(0, 0, -1),
		PeriodEnd:      time.Now().AddDate(0, 1, 0),
		LastUpdated:    time.Now(),
	}
	err = db.Create(wrongMetric).Error
	require.NoError(t, err)

	t.Run("Should return 0 when all terminals are stopped", func(t *testing.T) {
		metrics, err := service.GetUserUsageMetrics(userID)
		require.NoError(t, err)

		for _, m := range *metrics {
			if m.MetricType == "concurrent_terminals" {
				assert.Equal(t, int64(0), m.CurrentValue,
					"Should return 0 when no active terminals exist")
			}
		}
	})
}
