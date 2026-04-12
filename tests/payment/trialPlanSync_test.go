// tests/payment/trialPlanSync_test.go
// Tests that EnsureTrialPlanExists resets all Trial plan fields on every startup,
// not just the subset originally included in the sync block.
package payment_tests

import (
	"testing"

	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/initialization"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTrialPlanSync_AllFieldsReset verifies that EnsureTrialPlanExists resets
// every governed field — including DataPersistenceEnabled, DataPersistenceGB,
// NetworkAccessEnabled, and IsActive — even when they have been manually altered
// in the database.
func TestTrialPlanSync_AllFieldsReset(t *testing.T) {
	db := freshTestDB(t)

	// Seed a Trial plan whose fields have drifted from the expected defaults.
	driftedPlan := paymentModels.SubscriptionPlan{
		Name:                        "Trial",
		Description:                 "Tampered description",
		PriceAmount:                 0,
		Currency:                    "eur",
		BillingInterval:             "month",
		MaxSessionDurationMinutes:   999,
		MaxConcurrentTerminals:      99,
		MaxCourses:                  999,
		NetworkAccessEnabled:        true,  // should be reset to false
		DataPersistenceEnabled:      true,  // should be reset to false
		DataPersistenceGB:           100,   // should be reset to 0
		IsActive:                    false, // should be reset to true
		CommandHistoryRetentionDays: 999,
		Features:                    []string{"tampered"},
		AllowedMachineSizes:         []string{"XXL"},
		AllowedTemplates:            []string{"tampered-template"},
	}
	require.NoError(t, db.Create(&driftedPlan).Error, "failed to seed drifted Trial plan")

	// Act: run the sync function (the code-under-test path for an existing plan).
	initialization.EnsureTrialPlanExists(db)

	// Reload from DB.
	var synced paymentModels.SubscriptionPlan
	require.NoError(t, db.Where("name = ? AND price_amount = 0", "Trial").First(&synced).Error)

	// Fields that were already synced before this fix:
	assert.Equal(t, "Free plan for testing the platform. 1 hour sessions, no network access. Perfect for trying out terminals.", synced.Description)
	assert.Equal(t, 60, synced.MaxSessionDurationMinutes)
	assert.Equal(t, 1, synced.MaxConcurrentTerminals)
	assert.Equal(t, -1, synced.MaxCourses)
	assert.Equal(t, false, synced.NetworkAccessEnabled)
	assert.Equal(t, 7, synced.CommandHistoryRetentionDays)

	// Fields that were missing from the sync block (the bug):
	assert.Equal(t, false, synced.DataPersistenceEnabled, "DataPersistenceEnabled must be reset to false")
	assert.Equal(t, 0, synced.DataPersistenceGB, "DataPersistenceGB must be reset to 0")
	assert.Equal(t, true, synced.IsActive, "IsActive must be reset to true")
}
