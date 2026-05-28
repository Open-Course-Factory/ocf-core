// Tests for the one-shot startup migration that rescales legacy
// integer-vCPU CPU budget values to integer millicores (mCPU).
//
// Why: the budget engine switched units so XS (cpu_allowance=50% on
// tt-backend) can be priced as 500 mCPU instead of rounding to 1 vCPU
// (which over-counted XS by 2×). MaxCPU and SizeCPU now live in mCPU
// across catalog, model, DTO, and JSON wire format.
//
// The migration multiplies any non-zero, sub-100 legacy value by 1000.
// Anything ≥100 is treated as already in mCPU and left alone — the new
// seeds start at 500 mCPU so the guard fires only on legacy rows.
package payment_tests

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/initialization"
	paymentModels "soli/formations/src/payment/models"
)

func TestRescaleVCPUToMillicores_RescalesLegacyPlanValue(t *testing.T) {
	db := freshTestDB(t)

	// Legacy plan: MaxCPU=8 (vCPU). After migration must be 8000 mCPU.
	plan := &paymentModels.SubscriptionPlan{
		BaseModel:       entityManagementModels.BaseModel{ID: uuid.New()},
		Name:            "LegacyVCPUPlan",
		PriceAmount:     0,
		Currency:        "eur",
		BillingInterval: "month",
		IsActive:        true,
		MaxCPU:          8, // legacy vCPU reading
		MaxMemoryMB:     4096,
	}
	require.NoError(t, db.Create(plan).Error)

	initialization.RescaleVCPUToMillicores(db)

	var got paymentModels.SubscriptionPlan
	require.NoError(t, db.First(&got, "id = ?", plan.ID).Error)
	assert.Equal(t, 8000, got.MaxCPU,
		"legacy MaxCPU=8 vCPU must be rescaled to 8000 mCPU")
}

func TestRescaleVCPUToMillicores_LeavesAlreadyScaledPlanAlone(t *testing.T) {
	db := freshTestDB(t)

	// Already in mCPU: MaxCPU=5000. Guard keeps it at 5000.
	plan := &paymentModels.SubscriptionPlan{
		BaseModel:       entityManagementModels.BaseModel{ID: uuid.New()},
		Name:            "AlreadyMcpuPlan",
		PriceAmount:     0,
		Currency:        "eur",
		BillingInterval: "month",
		IsActive:        true,
		MaxCPU:          5000,
		MaxMemoryMB:     4096,
	}
	require.NoError(t, db.Create(plan).Error)

	initialization.RescaleVCPUToMillicores(db)

	var got paymentModels.SubscriptionPlan
	require.NoError(t, db.First(&got, "id = ?", plan.ID).Error)
	assert.Equal(t, 5000, got.MaxCPU,
		"already-mCPU value (5000) must NOT be re-multiplied")
}

func TestRescaleVCPUToMillicores_LeavesUnlimitedPlanAlone(t *testing.T) {
	db := freshTestDB(t)

	// Unlimited (MaxCPU=0). The >0 guard prevents a no-op multiply that
	// would still be 0, but we assert explicitly to pin the contract.
	plan := &paymentModels.SubscriptionPlan{
		BaseModel:       entityManagementModels.BaseModel{ID: uuid.New()},
		Name:            "UnlimitedPlan",
		PriceAmount:     0,
		Currency:        "eur",
		BillingInterval: "month",
		IsActive:        true,
		MaxCPU:          0,
		MaxMemoryMB:     0,
	}
	require.NoError(t, db.Create(plan).Error)

	initialization.RescaleVCPUToMillicores(db)

	var got paymentModels.SubscriptionPlan
	require.NoError(t, db.First(&got, "id = ?", plan.ID).Error)
	assert.Equal(t, 0, got.MaxCPU, "0 (unlimited) must remain 0")
}

func TestRescaleVCPUToMillicores_RescalesLegacyTerminalSize(t *testing.T) {
	db := freshTestDB(t)
	ensureTerminalsTable(t, db)

	// Legacy terminal: size_cpu=2 (vCPU) → must become 2000 mCPU.
	insertTerminal(t, db, "u-rescale-term", nil, "running", "ephemeral", 2, 1024)

	initialization.RescaleVCPUToMillicores(db)

	var sizeCPU int
	require.NoError(t, db.Raw(
		`SELECT size_cpu FROM terminals WHERE user_id = ?`, "u-rescale-term",
	).Scan(&sizeCPU).Error)
	assert.Equal(t, 2000, sizeCPU,
		"legacy size_cpu=2 vCPU must be rescaled to 2000 mCPU")
}

func TestRescaleVCPUToMillicores_IsIdempotent(t *testing.T) {
	db := freshTestDB(t)

	// Legacy plan: MaxCPU=4 (vCPU).
	plan := &paymentModels.SubscriptionPlan{
		BaseModel:       entityManagementModels.BaseModel{ID: uuid.New()},
		Name:            "IdempotentPlan",
		PriceAmount:     0,
		Currency:        "eur",
		BillingInterval: "month",
		IsActive:        true,
		MaxCPU:          4,
		MaxMemoryMB:     4096,
	}
	require.NoError(t, db.Create(plan).Error)

	// Run twice. After the first run MaxCPU=4000 (≥100), so the second
	// run must leave it alone — proving the guard.
	initialization.RescaleVCPUToMillicores(db)
	initialization.RescaleVCPUToMillicores(db)

	var got paymentModels.SubscriptionPlan
	require.NoError(t, db.First(&got, "id = ?", plan.ID).Error)
	assert.Equal(t, 4000, got.MaxCPU,
		"second run must NOT re-multiply (idempotency via >0 AND <100 guard)")
}
