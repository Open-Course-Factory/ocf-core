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

// TestCheckUsageLimit_ConcurrentTerminals_RealTimeCount tests that CheckUsageLimit
// uses real-time active terminal count instead of stored metric value
func TestCheckUsageLimit_ConcurrentTerminals_RealTimeCount(t *testing.T) {
	// Setup
	db := setupIntegrationDB(t)
	plans := seedTestPlans(t, db)
	service := paymentServices.NewSubscriptionService(db)

	// Migrate terminal tables
	err := db.AutoMigrate(&terminalModels.Terminal{}, &terminalModels.UserTerminalKey{})
	require.NoError(t, err)

	userID := "middleware-test-user"
	subscription := createUserSubscription(t, db, userID, plans["Trainer"].ID) // Max 3 concurrent

	// Create user terminal key
	userKey := &terminalModels.UserTerminalKey{
		UserID:      userID,
		APIKey:      "middleware-test-key",
		KeyName:     "Test Key",
		IsActive:    true,
		MaxSessions: 5,
	}
	err = db.Create(userKey).Error
	require.NoError(t, err)

	t.Run("Should allow creation when only 1 active terminal exists (limit is 3)", func(t *testing.T) {
		// Create 1 active terminal
		activeTerminal := &terminalModels.Terminal{
			SessionID:         "middleware-session-1",
			UserID:            userID,
			Status:            "active",
			ExpiresAt:         time.Now().Add(1 * time.Hour),
			UserTerminalKeyID: userKey.ID,
		}
		err = db.Create(activeTerminal).Error
		require.NoError(t, err)

		// Create wrong metric (showing 3 terminals, but only 1 is active)
		wrongMetric := &paymentModels.UsageMetrics{
			UserID:         userID,
			SubscriptionID: subscription.ID,
			MetricType:     "concurrent_terminals",
			CurrentValue:   3, // WRONG! Should be 1
			LimitValue:     3, // Trainer plan
			PeriodStart:    time.Now().AddDate(0, 0, -1),
			PeriodEnd:      time.Now().AddDate(0, 1, 0),
			LastUpdated:    time.Now(),
		}
		err = db.Create(wrongMetric).Error
		require.NoError(t, err)

		// Check if creating a new terminal is allowed (increment=1)
		check, err := service.CheckUsageLimit(userID, "concurrent_terminals", 1)
		require.NoError(t, err)
		require.NotNil(t, check)

		// Should be ALLOWED because real count is 1, not 3
		assert.True(t, check.Allowed, "Should allow terminal creation when real count (1) + increment (1) <= limit (3)")
		assert.Equal(t, int64(1), check.CurrentUsage, "Should report real active terminal count (1), not stored wrong value (3)")
		assert.Equal(t, int64(3), check.Limit, "Limit should be 3 for Trainer plan")
		assert.Equal(t, int64(2), check.RemainingUsage, "Should have 2 slots remaining (3 - 1)")
	})

	t.Run("Should block when real active count reaches limit", func(t *testing.T) {
		// Add 2 more active terminals (total: 3 active, which is the limit)
		for i := 2; i <= 3; i++ {
			terminal := &terminalModels.Terminal{
				SessionID:         "middleware-session-" + string(rune('0'+i)),
				UserID:            userID,
				Status:            "active",
				ExpiresAt:         time.Now().Add(1 * time.Hour),
				UserTerminalKeyID: userKey.ID,
			}
			err = db.Create(terminal).Error
			require.NoError(t, err)
		}

		// Try to create another terminal (increment=1)
		check, err := service.CheckUsageLimit(userID, "concurrent_terminals", 1)
		require.NoError(t, err)
		require.NotNil(t, check)

		// Should be BLOCKED because 3 + 1 > 3
		assert.False(t, check.Allowed, "Should block when active count (3) + increment (1) > limit (3)")
		assert.Equal(t, int64(3), check.CurrentUsage, "Should show 3 active terminals")
		assert.Equal(t, int64(0), check.RemainingUsage, "No remaining slots")
		assert.Contains(t, check.Message, "Usage limit exceeded")
	})

	t.Run("Should allow after stopping terminals", func(t *testing.T) {
		// Stop 2 terminals (leaving only 1 active)
		// First, get 2 terminals to stop
		var terminalsToStop []terminalModels.Terminal
		err := db.Where("user_id = ? AND status = ?", userID, "active").
			Limit(2).
			Find(&terminalsToStop).Error
		require.NoError(t, err)
		require.Len(t, terminalsToStop, 2, "Should find 2 active terminals to stop")

		// Stop them
		for _, terminal := range terminalsToStop {
			err := db.Model(&terminal).Update("status", "stopped").Error
			require.NoError(t, err)
		}

		// Try to create another terminal
		check, err := service.CheckUsageLimit(userID, "concurrent_terminals", 1)
		require.NoError(t, err)

		// Should be ALLOWED because only 1 active now
		assert.True(t, check.Allowed, "Should allow after stopping terminals")
		assert.Equal(t, int64(1), check.CurrentUsage, "Should count only 1 active terminal")
		assert.Equal(t, int64(2), check.RemainingUsage, "Should have 2 remaining slots")
	})
}

// TestCheckUsageLimit_FirstTimeUser tests the case where user has no metrics yet
func TestCheckUsageLimit_FirstTimeUser(t *testing.T) {
	// Setup
	db := setupIntegrationDB(t)
	plans := seedTestPlans(t, db)
	service := paymentServices.NewSubscriptionService(db)

	// Migrate terminal tables
	err := db.AutoMigrate(&terminalModels.Terminal{}, &terminalModels.UserTerminalKey{})
	require.NoError(t, err)

	userID := "first-time-user"
	createUserSubscription(t, db, userID, plans["Solo"].ID) // Max 1 concurrent

	// Create user terminal key
	userKey := &terminalModels.UserTerminalKey{
		UserID:      userID,
		APIKey:      "first-time-key",
		KeyName:     "Test Key",
		IsActive:    true,
		MaxSessions: 5,
	}
	err = db.Create(userKey).Error
	require.NoError(t, err)

	t.Run("Should allow first terminal when no metrics exist", func(t *testing.T) {
		// No metrics exist yet, but user has 0 active terminals
		check, err := service.CheckUsageLimit(userID, "concurrent_terminals", 1)
		require.NoError(t, err)

		assert.True(t, check.Allowed, "Should allow first terminal")
		assert.Equal(t, int64(0), check.CurrentUsage, "Should count 0 active terminals")
		assert.Equal(t, int64(1), check.Limit, "Solo plan limit is 1")
	})

	t.Run("Should block second terminal for Solo plan", func(t *testing.T) {
		// Create 1 active terminal
		terminal := &terminalModels.Terminal{
			SessionID:         "first-time-session",
			UserID:            userID,
			Status:            "active",
			ExpiresAt:         time.Now().Add(1 * time.Hour),
			UserTerminalKeyID: userKey.ID,
		}
		err = db.Create(terminal).Error
		require.NoError(t, err)

		// Try to create second terminal
		check, err := service.CheckUsageLimit(userID, "concurrent_terminals", 1)
		require.NoError(t, err)

		assert.False(t, check.Allowed, "Should block second terminal on Solo plan")
		assert.Equal(t, int64(1), check.CurrentUsage, "Should count 1 active terminal")
		assert.Equal(t, int64(0), check.RemainingUsage, "No remaining slots on Solo plan")
	})
}
