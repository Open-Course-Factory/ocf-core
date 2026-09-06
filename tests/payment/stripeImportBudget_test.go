// tests/payment/stripeImportBudget_test.go
//
// The Stripe import writes plans with db.Save / db.Create, which never runs
// the entity hook that refuses a non-positive budget. So the import has to
// apply the same rule itself, and it has to read the product metadata for
// what it says rather than defaulting an absent axis to zero: with no
// unlimited state, a zero budget is a plan nobody can launch anything on.
//
// The rule these tests pin:
//   - update: an axis the metadata does not state is left as it was; a stated
//     non-positive axis refuses the whole update and the row is untouched.
//   - create: both axes must be stated and positive, or nothing is persisted.
//   - either way a refused product lands in FailedPlans naming the axis, like
//     every other failed import.
package payment_tests

import (
	"testing"

	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// importedPlan seeds a plan already linked to a Stripe product whose metadata
// is exactly what the caller passes, so the import walks the update branch.
func importedPlan(t *testing.T, db *gorm.DB, cat *fakeStripeCatalog, metadata map[string]string) *models.SubscriptionPlan {
	t.Helper()
	plan := &models.SubscriptionPlan{
		Name:            "Imported",
		PriceAmount:     1999,
		Currency:        "eur",
		BillingInterval: "month",
		IsActive:        true,
		MaxCPU:          6000,
		MaxMemoryMB:     6144,
	}
	require.NoError(t, db.Create(plan).Error)

	prod := cat.seedProduct("Imported", "desc", true, metadata)
	price := cat.seedPrice(prod.ID, 1999, "eur", "month", nil)
	plan.StripeProductID = &prod.ID
	plan.StripePriceID = &price.ID
	require.NoError(t, db.Save(plan).Error)
	return plan
}

func TestStripeImport_UpdateWithoutBudgetMetadata_LeavesTheBudgetAlone(t *testing.T) {
	db := freshTestDB(t)
	cat := installFakeStripeCatalog(t)
	plan := importedPlan(t, db, cat, map[string]string{"plan_id": "whatever"})

	result, err := services.NewStripeService(db).ImportPlansFromStripe()
	require.NoError(t, err)

	assert.Equal(t, 1, result.UpdatedPlans)
	assert.Empty(t, result.FailedPlans)
	reloaded := reloadPlan(t, db, plan.ID)
	assert.Equal(t, 6000, reloaded.MaxCPU, "an axis the metadata does not state must not be zeroed")
	assert.Equal(t, 6144, reloaded.MaxMemoryMB)
}

func TestStripeImport_UpdateWithMalformedBudget_LeavesTheValueUnchanged(t *testing.T) {
	db := freshTestDB(t)
	cat := installFakeStripeCatalog(t)
	plan := importedPlan(t, db, cat, map[string]string{"max_cpu": "lots", "max_memory_mb": "8192"})

	result, err := services.NewStripeService(db).ImportPlansFromStripe()
	require.NoError(t, err)

	assert.Equal(t, 1, result.UpdatedPlans)
	reloaded := reloadPlan(t, db, plan.ID)
	assert.Equal(t, 6000, reloaded.MaxCPU, "a value that does not parse states nothing")
	assert.Equal(t, 8192, reloaded.MaxMemoryMB, "the axis that does parse is applied")
}

func TestStripeImport_UpdateWithStatedZero_RefusesAndKeepsTheRow(t *testing.T) {
	db := freshTestDB(t)
	cat := installFakeStripeCatalog(t)
	plan := importedPlan(t, db, cat, map[string]string{"max_cpu": "0"})
	prodID := *plan.StripeProductID
	require.NoError(t, db.Model(plan).Update("name", "Before import").Error)

	result, err := services.NewStripeService(db).ImportPlansFromStripe()
	require.NoError(t, err)

	assert.Equal(t, 0, result.UpdatedPlans)
	require.Len(t, result.FailedPlans, 1)
	assert.Equal(t, prodID, result.FailedPlans[0].StripeProductID)
	assert.Contains(t, result.FailedPlans[0].Error, "max_cpu")
	reloaded := reloadPlan(t, db, plan.ID)
	assert.Equal(t, 6000, reloaded.MaxCPU)
	assert.Equal(t, "Before import", reloaded.Name, "a refused update must not write any of its other fields either")
}

func TestStripeImport_CreateWithoutBudgetMetadata_FailsAndPersistsNothing(t *testing.T) {
	db := freshTestDB(t)
	cat := installFakeStripeCatalog(t)
	prod := cat.seedProduct("Fresh from Stripe", "desc", true, map[string]string{"max_cpu": "4000"})
	cat.seedPrice(prod.ID, 2999, "eur", "month", nil)

	result, err := services.NewStripeService(db).ImportPlansFromStripe()
	require.NoError(t, err)

	assert.Equal(t, 0, result.CreatedPlans)
	require.Len(t, result.FailedPlans, 1)
	assert.Equal(t, prod.ID, result.FailedPlans[0].StripeProductID)
	assert.Contains(t, result.FailedPlans[0].Error, "max_memory_mb", "the message must name the missing axis")
	assert.NotContains(t, result.FailedPlans[0].Error, "max_cpu", "and only the missing one")

	var count int64
	require.NoError(t, db.Model(&models.SubscriptionPlan{}).Where("name = ?", "Fresh from Stripe").Count(&count).Error)
	assert.Equal(t, int64(0), count)
}

func TestStripeImport_CreateWithBudgetMetadata_PersistsIt(t *testing.T) {
	db := freshTestDB(t)
	cat := installFakeStripeCatalog(t)
	prod := cat.seedProduct("Fresh from Stripe", "desc", true, map[string]string{"max_cpu": "4000", "max_memory_mb": "2048"})
	cat.seedPrice(prod.ID, 2999, "eur", "month", nil)

	result, err := services.NewStripeService(db).ImportPlansFromStripe()
	require.NoError(t, err)

	assert.Equal(t, 1, result.CreatedPlans)
	var created models.SubscriptionPlan
	require.NoError(t, db.First(&created, "name = ?", "Fresh from Stripe").Error)
	assert.Equal(t, 4000, created.MaxCPU)
	assert.Equal(t, 2048, created.MaxMemoryMB)
}

// The seed writes catalogue.json with db.Create, which bypasses the hook, so
// the catalogue itself is the last place a zero budget could enter unchecked.
func TestPlanCatalogue_EveryPlanStatesAPositiveBudget(t *testing.T) {
	plans, err := services.DecidedCatalogue()
	require.NoError(t, err)
	require.NotEmpty(t, plans)

	for _, plan := range plans {
		assert.Empty(t, plan.MissingBudgetAxes(), "plan %q in catalogue.json", plan.Name)
	}
}
