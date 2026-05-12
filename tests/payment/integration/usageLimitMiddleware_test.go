package integration

import (
	"testing"
	"time"

	paymentModels "soli/formations/src/payment/models"
	terminalModels "soli/formations/src/terminalTrainer/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestQuotaCheck_ConcurrentTerminals_RealTimeCount asserts the
// concurrent_terminals quota path uses a real-time active terminal count
// rather than the stored usage_metrics value. The check now goes through
// QuotaService directly (the legacy CheckUsageLimit wrapper has been
// removed as part of the SSOT consolidation).
func TestQuotaCheck_ConcurrentTerminals_RealTimeCount(t *testing.T) {
	// Setup
	db := setupIntegrationDB(t)
	plans := seedTestPlans(t, db)
	quotaSvc := newQuotaService(db)

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
		check, err := quotaSvc.CheckUserQuota(userID, nil, "concurrent_terminals", 1)
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
		check, err := quotaSvc.CheckUserQuota(userID, nil, "concurrent_terminals", 1)
		require.NoError(t, err)
		require.NotNil(t, check)

		// Should be BLOCKED because 3 + 1 > 3
		assert.False(t, check.Allowed, "Should block when active count (3) + increment (1) > limit (3)")
		assert.Equal(t, int64(3), check.CurrentUsage, "Should show 3 active terminals")
		assert.Equal(t, int64(0), check.RemainingUsage, "No remaining slots")
		assert.Contains(t, check.Message, "Usage limit exceeded")
	})

	t.Run("Stopping does NOT free slots — only DELETE does", func(t *testing.T) {
		// Pull 2 of the active terminals and STOP them. Per the design
		// contract a stopped session still occupies a slot until DELETE,
		// so the counter must remain at 3.
		var terminalsToStop []terminalModels.Terminal
		err := db.Where("user_id = ? AND status = ?", userID, "active").
			Limit(2).
			Find(&terminalsToStop).Error
		require.NoError(t, err)
		require.Len(t, terminalsToStop, 2)

		for _, terminal := range terminalsToStop {
			err := db.Model(&terminal).Update("status", "stopped").Error
			require.NoError(t, err)
		}

		check, err := quotaSvc.CheckUserQuota(userID, nil, "concurrent_terminals", 1)
		require.NoError(t, err)

		assert.False(t, check.Allowed,
			"stop-only must not free slots; user is still at 3/3 after stop")
		assert.Equal(t, int64(3), check.CurrentUsage,
			"1 active + 2 stopped = 3 occupied")
		assert.Equal(t, int64(0), check.RemainingUsage)
	})

	t.Run("Deleting frees slots", func(t *testing.T) {
		// Now mark the 2 stopped terminals as deleted (matching DeleteSession
		// behavior). The counter must drop to 1 active and the next launch
		// must be allowed.
		var terminalsToDelete []terminalModels.Terminal
		err := db.Where("user_id = ? AND status = ?", userID, "stopped").
			Find(&terminalsToDelete).Error
		require.NoError(t, err)
		require.Len(t, terminalsToDelete, 2)

		for _, terminal := range terminalsToDelete {
			err := db.Model(&terminal).Update("status", "deleted").Error
			require.NoError(t, err)
		}

		check, err := quotaSvc.CheckUserQuota(userID, nil, "concurrent_terminals", 1)
		require.NoError(t, err)

		assert.True(t, check.Allowed, "Should allow after deleting terminals")
		assert.Equal(t, int64(1), check.CurrentUsage, "Only 1 active terminal remains")
		assert.Equal(t, int64(2), check.RemainingUsage)
	})
}

// TestQuotaCheck_FirstTimeUser tests the case where user has no metrics yet
func TestQuotaCheck_FirstTimeUser(t *testing.T) {
	// Setup
	db := setupIntegrationDB(t)
	plans := seedTestPlans(t, db)
	quotaSvc := newQuotaService(db)

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
		check, err := quotaSvc.CheckUserQuota(userID, nil, "concurrent_terminals", 1)
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
		check, err := quotaSvc.CheckUserQuota(userID, nil, "concurrent_terminals", 1)
		require.NoError(t, err)

		assert.False(t, check.Allowed, "Should block second terminal on Solo plan")
		assert.Equal(t, int64(1), check.CurrentUsage, "Should count 1 active terminal")
		assert.Equal(t, int64(0), check.RemainingUsage, "No remaining slots on Solo plan")
	})
}
