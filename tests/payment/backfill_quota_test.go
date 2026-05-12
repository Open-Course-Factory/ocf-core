// Tests for the budget quota backfill + rollback commands.
//
// The backfill command derives equivalent CPU/RAM budgets from the legacy
// count-based plan limits (AllowedMachineSizes × MaxConcurrentTerminals)
// and flips the plan's QuotaModel to "budget".
//
// The rollback command resets MaxCPU/MaxMemoryMB to 0 and QuotaModel to
// "count" without touching AllowedMachineSizes.
package payment_tests

import (
	"testing"

	"soli/formations/src/payment/backfill"
	"soli/formations/src/payment/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBackfill_LegacyPlan_ProducesEquivalentBudget(t *testing.T) {
	db := freshTestDB(t)

	plan := &models.SubscriptionPlan{
		Name:                   "Legacy Plan SM",
		PriceAmount:            999,
		Currency:               "eur",
		BillingInterval:        "month",
		AllowedMachineSizes:    []string{"S", "M"},
		MaxConcurrentTerminals: 3,
		QuotaModel:             "count",
	}
	require.NoError(t, db.Create(plan).Error)

	report, err := backfill.Run(db, backfill.Options{Apply: true})
	require.NoError(t, err)
	require.Equal(t, 1, report.Updated, "one plan should be updated")

	var fetched models.SubscriptionPlan
	require.NoError(t, db.First(&fetched, "id = ?", plan.ID).Error)
	assert.Equal(t, "budget", fetched.QuotaModel)
	// largest size in {S, M} is M = 2 CPU / 1024 MiB. Times 3 terminals.
	assert.Equal(t, 6, fetched.MaxCPU)
	assert.Equal(t, 3072, fetched.MaxMemoryMB)
}

func TestBackfill_AllSizes_UsesXL(t *testing.T) {
	db := freshTestDB(t)

	plan := &models.SubscriptionPlan{
		Name:                   "All Sizes Plan",
		PriceAmount:            1999,
		Currency:               "eur",
		BillingInterval:        "month",
		AllowedMachineSizes:    []string{"all"},
		MaxConcurrentTerminals: 2,
		QuotaModel:             "count",
	}
	require.NoError(t, db.Create(plan).Error)

	report, err := backfill.Run(db, backfill.Options{Apply: true})
	require.NoError(t, err)
	assert.Equal(t, 1, report.Updated)

	var fetched models.SubscriptionPlan
	require.NoError(t, db.First(&fetched, "id = ?", plan.ID).Error)
	assert.Equal(t, "budget", fetched.QuotaModel)
	// "all" → XL = 4 CPU / 4096 MiB. Times 2 terminals.
	assert.Equal(t, 8, fetched.MaxCPU)
	assert.Equal(t, 8192, fetched.MaxMemoryMB)
}

func TestBackfill_EmptyAllowedSizes_UsesXL(t *testing.T) {
	db := freshTestDB(t)

	plan := &models.SubscriptionPlan{
		Name:                   "Empty Sizes Plan",
		PriceAmount:            2999,
		Currency:               "eur",
		BillingInterval:        "month",
		AllowedMachineSizes:    []string{},
		MaxConcurrentTerminals: 1,
		QuotaModel:             "count",
	}
	require.NoError(t, db.Create(plan).Error)

	report, err := backfill.Run(db, backfill.Options{Apply: true})
	require.NoError(t, err)
	assert.Equal(t, 1, report.Updated)

	var fetched models.SubscriptionPlan
	require.NoError(t, db.First(&fetched, "id = ?", plan.ID).Error)
	assert.Equal(t, "budget", fetched.QuotaModel)
	// Empty → XL = 4 CPU / 4096 MiB. Times 1.
	assert.Equal(t, 4, fetched.MaxCPU)
	assert.Equal(t, 4096, fetched.MaxMemoryMB)
}

func TestBackfill_UnlimitedTerminals(t *testing.T) {
	db := freshTestDB(t)

	plan := &models.SubscriptionPlan{
		Name:                   "Unlimited Plan",
		PriceAmount:            9999,
		Currency:               "eur",
		BillingInterval:        "month",
		AllowedMachineSizes:    []string{"L"},
		MaxConcurrentTerminals: -1, // unlimited
		QuotaModel:             "count",
	}
	require.NoError(t, db.Create(plan).Error)

	report, err := backfill.Run(db, backfill.Options{Apply: true})
	require.NoError(t, err)
	assert.Equal(t, 1, report.Updated)

	var fetched models.SubscriptionPlan
	require.NoError(t, db.First(&fetched, "id = ?", plan.ID).Error)
	assert.Equal(t, "budget", fetched.QuotaModel)
	assert.Equal(t, 0, fetched.MaxCPU, "unlimited terminals → MaxCPU=0")
	assert.Equal(t, 0, fetched.MaxMemoryMB, "unlimited terminals → MaxMemoryMB=0")
}

func TestBackfill_ZeroTerminals(t *testing.T) {
	db := freshTestDB(t)

	plan := &models.SubscriptionPlan{
		Name:                "Zero Plan",
		PriceAmount:         0,
		Currency:            "eur",
		BillingInterval:     "month",
		AllowedMachineSizes: []string{"XS"},
		QuotaModel:          "count",
	}
	require.NoError(t, db.Create(plan).Error)
	// GORM skips zero-value ints when the column has a non-zero default,
	// so force MaxConcurrentTerminals=0 via an explicit update.
	require.NoError(t, db.Model(plan).Update("max_concurrent_terminals", 0).Error)

	report, err := backfill.Run(db, backfill.Options{Apply: true})
	require.NoError(t, err)
	assert.Equal(t, 1, report.Updated)

	var fetched models.SubscriptionPlan
	require.NoError(t, db.First(&fetched, "id = ?", plan.ID).Error)
	assert.Equal(t, "budget", fetched.QuotaModel)
	assert.Equal(t, 0, fetched.MaxCPU)
	assert.Equal(t, 0, fetched.MaxMemoryMB)
}

func TestBackfill_Idempotent(t *testing.T) {
	db := freshTestDB(t)

	plan := &models.SubscriptionPlan{
		Name:                   "Idempotent Plan",
		PriceAmount:            499,
		Currency:               "eur",
		BillingInterval:        "month",
		AllowedMachineSizes:    []string{"M"},
		MaxConcurrentTerminals: 2,
		QuotaModel:             "count",
	}
	require.NoError(t, db.Create(plan).Error)

	// First run: should update.
	report1, err := backfill.Run(db, backfill.Options{Apply: true})
	require.NoError(t, err)
	require.Equal(t, 1, report1.Updated)

	// Second run: no-op.
	report2, err := backfill.Run(db, backfill.Options{Apply: true})
	require.NoError(t, err)
	assert.Equal(t, 0, report2.Updated, "second run should not modify already-budget plans")
	assert.Equal(t, 1, report2.Skipped, "second run should skip the already-budget plan")

	var fetched models.SubscriptionPlan
	require.NoError(t, db.First(&fetched, "id = ?", plan.ID).Error)
	assert.Equal(t, "budget", fetched.QuotaModel)
	assert.Equal(t, 4, fetched.MaxCPU)    // M = 2 CPU × 2 = 4
	assert.Equal(t, 2048, fetched.MaxMemoryMB) // M = 1024 MiB × 2 = 2048
}

func TestBackfill_DryRun_DoesNotPersist(t *testing.T) {
	db := freshTestDB(t)

	plan := &models.SubscriptionPlan{
		Name:                   "DryRun Plan",
		PriceAmount:            299,
		Currency:               "eur",
		BillingInterval:        "month",
		AllowedMachineSizes:    []string{"S"},
		MaxConcurrentTerminals: 1,
		QuotaModel:             "count",
	}
	require.NoError(t, db.Create(plan).Error)

	report, err := backfill.Run(db, backfill.Options{Apply: false})
	require.NoError(t, err)
	assert.Equal(t, 1, report.WouldUpdate, "dry-run reports what would change")
	assert.Equal(t, 0, report.Updated, "dry-run does not actually update")

	var fetched models.SubscriptionPlan
	require.NoError(t, db.First(&fetched, "id = ?", plan.ID).Error)
	assert.Equal(t, "count", fetched.QuotaModel, "dry-run must not persist")
	assert.Equal(t, 0, fetched.MaxCPU)
	assert.Equal(t, 0, fetched.MaxMemoryMB)
}

func TestRollback_RestoresCountMode(t *testing.T) {
	db := freshTestDB(t)

	plan := &models.SubscriptionPlan{
		Name:                   "Rollback Plan",
		PriceAmount:            1499,
		Currency:               "eur",
		BillingInterval:        "month",
		AllowedMachineSizes:    []string{"L"},
		MaxConcurrentTerminals: 2,
		QuotaModel:             "count",
	}
	require.NoError(t, db.Create(plan).Error)

	// Apply backfill first.
	_, err := backfill.Run(db, backfill.Options{Apply: true})
	require.NoError(t, err)

	// Sanity check.
	var afterBackfill models.SubscriptionPlan
	require.NoError(t, db.First(&afterBackfill, "id = ?", plan.ID).Error)
	require.Equal(t, "budget", afterBackfill.QuotaModel)
	require.Equal(t, 8, afterBackfill.MaxCPU)         // L = 4 CPU × 2 = 8
	require.Equal(t, 4096, afterBackfill.MaxMemoryMB) // L = 2048 MiB × 2 = 4096

	// Now rollback.
	report, err := backfill.Rollback(db, backfill.Options{Apply: true})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, report.Updated, 1)

	var afterRollback models.SubscriptionPlan
	require.NoError(t, db.First(&afterRollback, "id = ?", plan.ID).Error)
	assert.Equal(t, "count", afterRollback.QuotaModel)
	assert.Equal(t, 0, afterRollback.MaxCPU)
	assert.Equal(t, 0, afterRollback.MaxMemoryMB)
	// AllowedMachineSizes must be preserved.
	assert.Equal(t, []string{"L"}, afterRollback.AllowedMachineSizes)
}
