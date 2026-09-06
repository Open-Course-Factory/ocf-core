// tests/payment/stripeIdempotencyFingerprint_test.go
//
// A Stripe idempotency key promises "same key, same request". The checkout keys
// folded the user, plan, coupon, address, day and purchase generation, but not
// the request itself, so a release that changed a checkout parameter (dropping
// the hardcoded payment methods) reused the key of a request made minutes
// earlier with the old parameters, and Stripe refused every checkout for the
// rest of the day with idempotency_error.
//
// The key now folds a fingerprint of the encoded request, so two requests that
// differ in ANY parameter cannot share a key, while an identical retry still
// does (pinned by the StableAcrossRetries tests).
package payment_tests

import (
	"testing"

	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStripeService_CreateCheckoutSession_KeyChangesWithTheRequest(t *testing.T) {
	db := freshTestDB(t)
	cap := installFakeStripeBackend(t)
	installFakeCasdoor(t, "fingerprint@example.com", "Fingerprint User")
	svc := services.NewStripeService(db)
	plan := activeStripePlan(t, db, "Fingerprint Plan")
	userID := "user_fp_" + uuid.NewString()

	first := dto.CreateCheckoutSessionInput{
		SubscriptionPlanID: plan.ID,
		SuccessURL:         "https://app.test/success",
		CancelURL:          "https://app.test/cancel",
	}
	second := first
	second.SuccessURL = "https://app.test/success?attempt=2"

	_, err := svc.CreateCheckoutSession(userID, first, nil)
	require.NoError(t, err)
	_, err = svc.CreateCheckoutSession(userID, second, nil)
	require.NoError(t, err)

	keys := cap.get("checkout_session")
	require.Len(t, keys, 2)
	assert.NotEqual(t, keys[0], keys[1],
		"a checkout whose request differs must not reuse the key of the earlier one: Stripe refuses a reused key with different parameters")
}

func TestStripeService_CreateBulkCheckoutSession_KeyChangesWithTheRequest(t *testing.T) {
	db := freshTestDB(t)
	cap := installFakeStripeBackend(t)
	installFakeCasdoor(t, "bulkfp@example.com", "Bulk Fingerprint User")
	svc := services.NewStripeService(db)
	plan := activeStripePlan(t, db, "Bulk Fingerprint Plan")
	require.NoError(t, db.Model(plan).Update("bulk_purchasable", true).Error)
	buyerID := "user_bulkfp_" + uuid.NewString()
	seedTrainerWithGroupManagement(t, db, buyerID)

	first := dto.CreateBulkCheckoutSessionInput{
		SubscriptionPlanID: plan.ID,
		Quantity:           5,
		SuccessURL:         "https://app.test/success",
		CancelURL:          "https://app.test/cancel",
	}
	second := first
	second.SuccessURL = "https://app.test/success?attempt=2"

	_, err := svc.CreateBulkCheckoutSession(buyerID, first)
	require.NoError(t, err)
	_, err = svc.CreateBulkCheckoutSession(buyerID, second)
	require.NoError(t, err)

	keys := cap.get("checkout_session")
	require.Len(t, keys, 2)
	assert.NotEqual(t, keys[0], keys[1],
		"the bulk checkout key must change with the request too")
}
