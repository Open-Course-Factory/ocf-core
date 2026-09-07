package payment_tests

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/payment/services"
)

// UpdateSubscription under always_invoice bills a proration immediately, so a
// retried call is a second invoice unless Stripe can recognise it as the same
// request. These tests pin the Idempotency-Key the plan-upgrade path sends.

// TestStripeService_UpdateSubscription_IdempotencyKey_StableAcrossRetries pins
// that an identical retry (same subscription, same live state, same target
// price) reuses the same key, so Stripe replays instead of invoicing twice.
func TestStripeService_UpdateSubscription_IdempotencyKey_StableAcrossRetries(t *testing.T) {
	db := freshTestDB(t)
	cap := installFakeStripeBackend(t)
	svc := services.NewStripeService(db)

	subID := "sub_upgrade_" + uuid.NewString()
	cap.setSubscriptionState(subID, "price_old", "in_1")

	_, err := svc.UpdateSubscription(subID, "price_new", "always_invoice")
	require.NoError(t, err)
	_, err = svc.UpdateSubscription(subID, "price_new", "always_invoice")
	require.NoError(t, err)

	keys := cap.get("subscription_update")
	require.Len(t, keys, 2)
	require.NotEmpty(t, keys[0])
	assert.Equal(t, keys[0], keys[1],
		"a retry of the same plan change must reuse the same Idempotency-Key, "+
			"or always_invoice bills the proration twice")
}

// TestStripeService_UpdateSubscription_IdempotencyKey_DistinctForForwardAndRevert
// pins that the compensating revert (back to the old price) never collides
// with the forward charge: same subscription, different target price, distinct
// key — and that the retry of an upgrade AFTER a revert is a fresh request,
// not a replay of the forward charge Stripe already cached.
func TestStripeService_UpdateSubscription_IdempotencyKey_DistinctForForwardAndRevert(t *testing.T) {
	db := freshTestDB(t)
	cap := installFakeStripeBackend(t)
	svc := services.NewStripeService(db)

	subID := "sub_revert_" + uuid.NewString()

	// Forward charge: the subscription is on the old price.
	cap.setSubscriptionState(subID, "price_old", "in_1")
	_, err := svc.UpdateSubscription(subID, "price_new", "always_invoice")
	require.NoError(t, err)

	// Revert: Stripe now holds the new price and the forward invoice.
	cap.setSubscriptionState(subID, "price_new", "in_2")
	_, err = svc.UpdateSubscription(subID, "price_old", "always_invoice")
	require.NoError(t, err)

	// The user tries again: the subscription is back on the old price, but the
	// revert left its own invoice behind.
	cap.setSubscriptionState(subID, "price_old", "in_3")
	_, err = svc.UpdateSubscription(subID, "price_new", "always_invoice")
	require.NoError(t, err)

	keys := cap.get("subscription_update")
	require.Len(t, keys, 3)
	assert.NotEqual(t, keys[0], keys[1], "forward and revert must not share a key")
	assert.NotEqual(t, keys[0], keys[2],
		"an upgrade attempted again after a revert must not replay the forward "+
			"charge Stripe cached: the subscription would stay on the old price "+
			"while the database moves to the new one")
}
