// tests/payment/checkoutCompletedPaymentMode_test.go
//
// #392: a checkout.session.completed event with no subscription object (a
// one-time payment, Stripe's `payment` mode) made handleCheckoutSessionCompleted
// dereference session.Subscription.ID outside its nil guard. The handler
// panicked, the webhook answered 500, and Stripe retried the same event
// forever.
package payment_tests

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	stripe "github.com/stripe/stripe-go/v85"

	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/mocks"
	"soli/formations/src/payment/services"
)

func TestCheckoutCompleted_PaymentModeWithoutSubscription_IsAcceptedWithoutPanic(t *testing.T) {
	_ = freshTestDB(t)
	casdoor.Enforcer = mocks.NewMockEnforcer()

	secret := "whsec_test_payment_mode"
	t.Setenv("STRIPE_WEBHOOK_SECRET", secret)

	event := map[string]any{
		"id":          "evt_test_payment_mode",
		"object":      "event",
		"api_version": stripe.APIVersion,
		"type":        "checkout.session.completed",
		"data": map[string]any{
			"object": map[string]any{
				"id":     "cs_test_payment_mode",
				"object": "checkout.session",
				"mode":   "payment",
				// No "subscription" key at all: that is what Stripe sends for a
				// one-time payment.
				"metadata": map[string]any{
					"user_id": "one-time-buyer-392",
				},
			},
		},
	}
	payload, err := json.Marshal(event)
	require.NoError(t, err)

	svc := services.NewStripeService(sharedTestDB)
	require.NotPanics(t, func() {
		err = svc.ProcessWebhook(payload, signStripeWebhook(t, payload, secret))
	}, "a checkout without a subscription must not crash the webhook handler")
	require.NoError(t, err, "a one-time payment checkout is a valid event and must be acknowledged")
}
