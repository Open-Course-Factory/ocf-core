// tests/payment/subscriptionPlanExplicitFalseOnCreate_test.go
//
// #447: POST /subscription-plans with "is_catalog": false stored true. The
// column carried gorm:"default:true", and GORM omits a zero-value field that
// has a default tag from the INSERT, so the column default won the write and a
// plan meant to be hidden landed on the public pricing page. The converter
// (DtoToModel) resolved the *bool correctly; the value was lost below it, in
// the very db.Create the generic repository issues.
package payment_tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/models"
)

func boolPtr(v bool) *bool { return &v }

// createPlanThroughTheEntityConverter runs the same two steps the generic
// create path runs: DtoToModel, then db.Create on the model.
func createPlanThroughTheEntityConverter(t *testing.T, input dto.CreateSubscriptionPlanInput) *models.SubscriptionPlan {
	t.Helper()
	out, err := getSubscriptionPlanOps(t).ConvertDtoToModel(input)
	require.NoError(t, err)
	plan, ok := out.(*models.SubscriptionPlan)
	require.True(t, ok, "converter must return *SubscriptionPlan, got %T", out)
	require.NoError(t, sharedTestDB.Create(plan).Error)

	var stored models.SubscriptionPlan
	require.NoError(t, sharedTestDB.First(&stored, "id = ?", plan.ID).Error)
	return &stored
}

func hiddenPlanInput(name string) dto.CreateSubscriptionPlanInput {
	return dto.CreateSubscriptionPlanInput{
		Name:            name,
		Currency:        "eur",
		BillingInterval: "month",
		MaxCPU:          1000,
		MaxMemoryMB:     1024,
		IsCatalog:       boolPtr(false),
	}
}

func TestSubscriptionPlanCreate_ExplicitIsCatalogFalse_IsStored(t *testing.T) {
	_ = freshTestDB(t)

	stored := createPlanThroughTheEntityConverter(t, hiddenPlanInput("hidden-seat-447"))

	assert.False(t, stored.IsCatalog, "a plan created with is_catalog=false must stay off the pricing page")
}

func TestSubscriptionPlanCreate_UnstatedIsCatalog_DefaultsToTrue(t *testing.T) {
	_ = freshTestDB(t)
	input := hiddenPlanInput("listed-plan-447")
	input.IsCatalog = nil

	stored := createPlanThroughTheEntityConverter(t, input)

	assert.True(t, stored.IsCatalog, "the default is still listed: the converter owns it now, not the column")
}
