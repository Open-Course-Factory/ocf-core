// tests/payment/trialPlanFirstOrCreate_test.go
// Tests that EnsureTrialPlanExists is idempotent and uses FirstOrCreate to avoid
// TOCTOU races when two pods start simultaneously.
package payment_tests

import (
	"testing"

	"soli/formations/src/initialization"
	"soli/formations/src/payment/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEnsureTrialPlanExists_FirstOrCreate_Idempotent verifies that calling
// EnsureTrialPlanExists multiple times results in exactly one Trial plan in the DB.
func TestEnsureTrialPlanExists_FirstOrCreate_Idempotent(t *testing.T) {
	db := freshTestDB(t)

	// Call twice to simulate two pods starting at roughly the same time
	initialization.EnsureTrialPlanExists(db)
	initialization.EnsureTrialPlanExists(db)

	var plans []models.SubscriptionPlan
	err := db.Where("name = ? AND price_amount = 0", "Trial").Find(&plans).Error
	require.NoError(t, err, "querying Trial plans should not fail")

	assert.Len(t, plans, 1, "exactly one Trial plan should exist after two calls")
}

// TestEnsureTrialPlanExists_FirstOrCreate_CreatesWhenMissing verifies that when
// no Trial plan exists, EnsureTrialPlanExists creates one with the correct fields.
func TestEnsureTrialPlanExists_FirstOrCreate_CreatesWhenMissing(t *testing.T) {
	db := freshTestDB(t)

	// Precondition: no Trial plan exists
	var count int64
	db.Model(&models.SubscriptionPlan{}).Where("name = ?", "Trial").Count(&count)
	require.Equal(t, int64(0), count, "no Trial plan should exist before calling the function")

	initialization.EnsureTrialPlanExists(db)

	var plan models.SubscriptionPlan
	err := db.Where("name = ? AND price_amount = 0", "Trial").First(&plan).Error
	require.NoError(t, err, "Trial plan should exist after EnsureTrialPlanExists")

	assert.Equal(t, "Trial", plan.Name)
	assert.Equal(t, int64(0), plan.PriceAmount)
	assert.Equal(t, "eur", plan.Currency)
	assert.True(t, plan.IsActive, "Trial plan should be active")
	assert.Equal(t, "member", plan.RequiredRole)
	assert.Equal(t, 60, plan.MaxSessionDurationMinutes)
	assert.Equal(t, 1, plan.MaxConcurrentTerminals)
	assert.Equal(t, -1, plan.MaxCourses)
	assert.False(t, plan.NetworkAccessEnabled, "Trial plan should not have network access")
	assert.Equal(t, 7, plan.CommandHistoryRetentionDays)
	assert.Contains(t, plan.AllowedMachineSizes, "XS")
}
