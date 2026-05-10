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

// TestUsageMetrics_ConcurrentTerminals_StoppedCountedExpiredNot tests that the
// concurrent_terminals metric counts BOTH 'active' and 'stopped' terminals
// (since stopped sessions still occupy a slot until DELETE), while excluding
// 'expired' sessions whose slot has already been released.
//
// Replaces the earlier TestUsageMetrics_ConcurrentTerminals_OnlyCountsActive
// which encoded the buggy "stopped is free" semantics — see fix(payment):
// count stopped terminals toward concurrent limit.
func TestUsageMetrics_ConcurrentTerminals_StoppedCountedExpiredNot(t *testing.T) {
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

	// Create 1 stopped terminal — MUST be counted (still occupies a slot).
	stoppedTerminal := &terminalModels.Terminal{
		SessionID:         "session-stopped",
		UserID:            userID,
		Status:            "stopped",
		ExpiresAt:         time.Now().Add(1 * time.Hour),
		UserTerminalKeyID: userKey.ID,
	}
	err = db.Create(stoppedTerminal).Error
	require.NoError(t, err)

	// Create 1 expired terminal — must NOT be counted (slot released).
	expiredTerminal := &terminalModels.Terminal{
		SessionID:         "session-expired",
		UserID:            userID,
		Status:            "expired",
		ExpiresAt:         time.Now().Add(-1 * time.Hour),
		UserTerminalKeyID: userKey.ID,
	}
	err = db.Create(expiredTerminal).Error
	require.NoError(t, err)

	// Manually create usage metric with WRONG value (simulating out-of-sync state).
	// Real-time recalc must overwrite the stored value.
	wrongMetric := &paymentModels.UsageMetrics{
		UserID:         userID,
		SubscriptionID: subscription.ID,
		MetricType:     "concurrent_terminals",
		CurrentValue:   4, // WRONG: real-time count is 3 (2 active + 1 stopped).
		LimitValue:     3, // Trainer plan allows 3 concurrent
		PeriodStart:    time.Now().AddDate(0, 0, -1),
		PeriodEnd:      time.Now().AddDate(0, 1, 0),
		LastUpdated:    time.Now(),
	}
	err = db.Create(wrongMetric).Error
	require.NoError(t, err)

	t.Run("GetUserUsageMetrics returns real-time count including stopped", func(t *testing.T) {
		metrics, err := service.GetUserUsageMetrics(userID)
		require.NoError(t, err)
		require.NotNil(t, metrics)

		var concurrentTerminalsMetric *paymentModels.UsageMetrics
		for _, m := range *metrics {
			if m.MetricType == "concurrent_terminals" {
				concurrentTerminalsMetric = &m
				break
			}
		}

		require.NotNil(t, concurrentTerminalsMetric, "concurrent_terminals metric should exist")

		// 2 active + 1 stopped = 3. Expired is excluded.
		assert.Equal(t, int64(3), concurrentTerminalsMetric.CurrentValue,
			"Should count active+stopped (3); expired and deleted are excluded")
		assert.Equal(t, int64(3), concurrentTerminalsMetric.LimitValue,
			"Limit should match subscription plan")
	})

	t.Run("Verify database still has wrong value but API returns recalculated count", func(t *testing.T) {
		var dbMetric paymentModels.UsageMetrics
		err := db.Where("user_id = ? AND metric_type = ?", userID, "concurrent_terminals").First(&dbMetric).Error
		require.NoError(t, err)
		assert.Equal(t, int64(4), dbMetric.CurrentValue, "Database should still have wrong stored value")

		metrics, err := service.GetUserUsageMetrics(userID)
		require.NoError(t, err)

		for _, m := range *metrics {
			if m.MetricType == "concurrent_terminals" {
				assert.Equal(t, int64(3), m.CurrentValue, "API should return real-time count (2 active + 1 stopped)")
			}
		}
	})
}

// TestUsageMetrics_ConcurrentTerminals_OnlyExpiredOrDeleted verifies that the
// real-time recalc returns 0 only when the user has no slot-occupying
// terminals — i.e. only 'expired' or 'deleted' rows remain. Stopped sessions
// are NOT in this set (they still occupy a slot until DELETE).
//
// Replaces the earlier TestUsageMetrics_ConcurrentTerminals_ZeroActiveTerminals
// which encoded the buggy "stopped is free" semantics.
func TestUsageMetrics_ConcurrentTerminals_OnlyExpiredOrDeleted(t *testing.T) {
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

	// Only expired terminals exist — no slot occupied.
	expiredTerminal := &terminalModels.Terminal{
		SessionID:         "session-expired-only",
		UserID:            userID,
		Status:            "expired",
		ExpiresAt:         time.Now().Add(-1 * time.Hour),
		UserTerminalKeyID: userKey.ID,
	}
	err = db.Create(expiredTerminal).Error
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

	t.Run("Should return 0 when only expired terminals remain", func(t *testing.T) {
		metrics, err := service.GetUserUsageMetrics(userID)
		require.NoError(t, err)

		for _, m := range *metrics {
			if m.MetricType == "concurrent_terminals" {
				assert.Equal(t, int64(0), m.CurrentValue,
					"Should return 0 when no slot-occupying terminals exist")
			}
		}
	})
}
