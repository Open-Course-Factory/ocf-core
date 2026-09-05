// tests/payment/planHealth_test.go
//
// Plan health: what a subscription plan promises, measured against what it can
// actually deliver.
//
// Mirrors scenarios/services/scenarioHealth.go. Every check asks a question the
// platform already answers at the moment it matters — the budget engine, the
// plan resolver, the Stripe checkout — and asks it early enough for an operator
// to fix the answer. A plan with nothing wrong produces no entry: the report is
// a list of things to fix, not an inventory.
package payment_tests

import (
	"testing"

	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// healthPlan creates an otherwise-sound plan with explicit budgets, bypassing
// the validation hook so the report can be tested against rows that predate it.
//
// The Stripe price is set because IsCatalog defaults to true: without it every
// fixture would trip catalog_without_price and no test could isolate its own
// subject.
func healthPlan(t *testing.T, db *gorm.DB, name string, cpu, mem int) *models.SubscriptionPlan {
	t.Helper()
	priceID := "price_" + uuid.New().String()[:8]
	plan := &models.SubscriptionPlan{
		BaseModel:       entityManagementModels.BaseModel{ID: uuid.New()},
		Name:            name,
		PriceAmount:     1990,
		Currency:        "eur",
		BillingInterval: "month",
		IsActive:        true,
		StripePriceID:   &priceID,
		MaxCPU:          cpu,
		MaxMemoryMB:     mem,
	}
	require.NoError(t, db.Create(plan).Error)
	return plan
}

func findingCodes(h services.PlanHealth) []string {
	out := []string{}
	for _, f := range h.Findings {
		out = append(out, f.Code)
	}
	return out
}

// A plan that can deliver what it promises produces no entry at all.
func TestPlanHealth_HealthyPlanIsAbsent(t *testing.T) {
	db := freshTestDB(t)
	// 24000 mCPU / 12288 MB affords 24 size-S sessions on both axes.
	healthPlan(t, db, "Balanced", 24000, 12288)

	report, err := services.CheckAllPlanHealth(db)

	require.NoError(t, err)
	assert.Empty(t, report, "a plan with nothing wrong must not appear in the report")
}

// The fault the validation hook now prevents, for rows that predate it.
func TestPlanHealth_ZeroBudgetIsBlocking(t *testing.T) {
	db := freshTestDB(t)
	healthPlan(t, db, "No CPU", 0, 6144)
	healthPlan(t, db, "No RAM", 6000, 0)

	report, err := services.CheckAllPlanHealth(db)

	require.NoError(t, err)
	require.Len(t, report, 2)
	for _, h := range report {
		assert.Contains(t, findingCodes(h), services.PlanHealthZeroBudget)
		for _, f := range h.Findings {
			if f.Code == services.PlanHealthZeroBudget {
				assert.Equal(t, services.PlanHealthBlocking, f.Severity)
				assert.NotEmpty(t, f.Detail, "the detail must name the axis at fault")
			}
		}
	}
}

// The #481 shape: a live subscription pointing at a plan that has been deleted.
// GORM's Preload honours the soft delete and returns a zero-value plan, so the
// subscription silently entitles nothing — and used to entitle everything.
func TestPlanHealth_DanglingReferenceIsBlocking(t *testing.T) {
	db := freshTestDB(t)
	plan := healthPlan(t, db, "Retired", 6000, 6144)
	personalSubscriptionOn(t, db, "orphaned-user", plan.ID)
	require.NoError(t, db.Delete(plan).Error)

	report, err := services.CheckAllPlanHealth(db)

	require.NoError(t, err)
	require.Len(t, report, 1, "a deleted plan with live subscribers must be reported")
	assert.Equal(t, plan.ID.String(), report[0].PlanID)
	assert.Contains(t, findingCodes(report[0]), services.PlanHealthDanglingReference)
}

// A deleted plan nobody references is simply retired, not broken.
func TestPlanHealth_DeletedPlanWithoutSubscribersIsSilent(t *testing.T) {
	db := freshTestDB(t)
	plan := healthPlan(t, db, "Retired Cleanly", 6000, 6144)
	require.NoError(t, db.Delete(plan).Error)

	report, err := services.CheckAllPlanHealth(db)

	require.NoError(t, err)
	assert.Empty(t, report, "a retired plan with no subscribers is not a fault")
}

// A plan offered for sale that Stripe cannot charge for.
func TestPlanHealth_CatalogPlanWithoutPriceIsWarning(t *testing.T) {
	db := freshTestDB(t)
	plan := healthPlan(t, db, "Unsellable", 6000, 6144)
	require.NoError(t, db.Model(plan).Updates(map[string]any{
		"is_catalog": true, "stripe_price_id": "",
	}).Error)

	report, err := services.CheckAllPlanHealth(db)

	require.NoError(t, err)
	require.Len(t, report, 1)
	assert.Contains(t, findingCodes(report[0]), services.PlanHealthCatalogWithoutPrice)
}

// A free catalog plan needs no Stripe price — nothing is ever charged.
func TestPlanHealth_FreeCatalogPlanNeedsNoPrice(t *testing.T) {
	db := freshTestDB(t)
	plan := healthPlan(t, db, "Free Tier", 500, 256)
	require.NoError(t, db.Model(plan).Updates(map[string]any{
		"is_catalog": true, "stripe_price_id": "", "price_amount": 0,
	}).Error)

	report, err := services.CheckAllPlanHealth(db)

	require.NoError(t, err)
	assert.Empty(t, report, "a free plan does not need a Stripe price")
}

// The advisory that would have made 2026-09-04 visible in advance: the two
// budget axes do not afford the same number of sessions, so the plan quietly
// delivers half what its RAM suggests.
func TestPlanHealth_AxisImbalanceIsAdvisory(t *testing.T) {
	db := freshTestDB(t)
	// Formateur's real shape: 6 size-S sessions by CPU, 12 by RAM.
	healthPlan(t, db, "Formateur", 6000, 6144)

	report, err := services.CheckAllPlanHealth(db)

	require.NoError(t, err)
	require.Len(t, report, 1)
	require.Contains(t, findingCodes(report[0]), services.PlanHealthAxisImbalance)

	for _, f := range report[0].Findings {
		if f.Code == services.PlanHealthAxisImbalance {
			assert.Equal(t, services.PlanHealthAdvisory, f.Severity,
				"an imbalance may be deliberate pricing — it is not a fault")
			assert.NotEmpty(t, f.Detail, "the detail must carry the two counts an operator needs")
		}
	}
}

// A plan whose axes agree says nothing. This is the shape the MDS class plan
// was corrected to.
func TestPlanHealth_BalancedAxesProduceNoAdvisory(t *testing.T) {
	db := freshTestDB(t)
	healthPlan(t, db, "Balanced", 24000, 12288)

	report, err := services.CheckAllPlanHealth(db)

	require.NoError(t, err)
	assert.Empty(t, report)
}

// A plan whose budgets are positive but too small to pay for any catalog size
// looks configured and launches nothing. It is reported in its own right, not
// as an imbalance — the imbalance would restate it in weaker terms.
func TestPlanHealth_BudgetBelowSmallestSizeIsBlocking(t *testing.T) {
	db := freshTestDB(t)
	// 100 mCPU / 64 MB is under xs (500 mCPU / 256 MB).
	healthPlan(t, db, "Too Small", 100, 64)

	report, err := services.CheckAllPlanHealth(db)

	require.NoError(t, err)
	require.Len(t, report, 1)
	assert.Contains(t, findingCodes(report[0]), services.PlanHealthAffordsNoSize)
	assert.NotContains(t, findingCodes(report[0]), services.PlanHealthAxisImbalance,
		"a plan that affords nothing has no imbalance to describe")
	assert.NotContains(t, findingCodes(report[0]), services.PlanHealthZeroBudget,
		"its budgets are positive — this is a different fault")
}
