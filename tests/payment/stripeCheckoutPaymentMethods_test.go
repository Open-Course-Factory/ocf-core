// tests/payment/stripeCheckoutPaymentMethods_test.go
//
// Checkout sessions must not name their payment methods. Which methods the
// account accepts is decided once, in the Stripe dashboard; naming them here
// was a second copy of that decision, and the two drifted: the code offered
// sepa_debit while the live account had it disabled, so Stripe refused every
// checkout with "The payment method type provided: sepa_debit is invalid".
//
// Omitting payment_method_types makes Checkout use the dashboard's payment
// method configuration, so a method toggled there is offered or withdrawn
// without a release.
package payment_tests

import (
	"testing"

	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStripeService_CreateCheckoutSession_LeavesPaymentMethodsToTheDashboard(t *testing.T) {
	db := freshTestDB(t)
	cap := installTaxFormCapturingStripe(t)
	installFakeCasdoor(t, "pm@example.com", "PM Buyer")
	svc := services.NewStripeService(db)
	plan := activeStripePlan(t, db, "PM Plan")

	_, err := svc.CreateCheckoutSession("user_pm_"+uuid.NewString(), dto.CreateCheckoutSessionInput{
		SubscriptionPlanID: plan.ID,
		SuccessURL:         "https://app.test/success",
		CancelURL:          "https://app.test/cancel",
	}, nil)
	require.NoError(t, err)

	form := cap.checkoutSessionForm()
	require.NotEmpty(t, form, "a checkout session request must have been sent")
	assert.NotContains(t, form, "payment_method_types",
		"the accepted payment methods are configured in the Stripe dashboard, not here. Captured form: %s", form)
}

func TestStripeService_CreateBulkCheckoutSession_LeavesPaymentMethodsToTheDashboard(t *testing.T) {
	db := freshTestDB(t)
	cap := installTaxFormCapturingStripe(t)
	installFakeCasdoor(t, "bulkpm@example.com", "Bulk PM Buyer")
	svc := services.NewStripeService(db)
	plan := activeStripePlan(t, db, "Bulk PM Plan")
	require.NoError(t, db.Model(plan).Update("bulk_purchasable", true).Error)
	buyerID := "user_bulkpm_" + uuid.NewString()
	seedTrainerWithGroupManagement(t, db, buyerID)

	_, err := svc.CreateBulkCheckoutSession(buyerID, dto.CreateBulkCheckoutSessionInput{
		SubscriptionPlanID: plan.ID,
		Quantity:           5,
		SuccessURL:         "https://app.test/success",
		CancelURL:          "https://app.test/cancel",
	})
	require.NoError(t, err)

	form := cap.checkoutSessionForm()
	require.NotEmpty(t, form, "a bulk checkout session request must have been sent")
	assert.NotContains(t, form, "payment_method_types",
		"the bulk path must leave payment methods to the dashboard too. Captured form: %s", form)
}
