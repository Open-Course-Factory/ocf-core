// tests/payment/packRebillingCancellation_test.go
//
// A prepaid learner-day pack is checked out in SUBSCRIPTION mode against a
// monthly recurring tiered price. Its entitlement dies at the pack deadline
// (ExpiresAt, enforced by ScopeEntitling) — but nothing tells STRIPE the pack
// is finite, so the subscription happily renews and the trainer is re-billed
// every month for seats nobody can use ("4 students for 3 days" = 19.80€, then
// 19.80€/month forever).
//
// This test pins the fix: when the bulk-created webhook provisions a pack
// (duration_days > 0), the handler must schedule the Stripe-side cancellation
// of the subscription at the pack deadline (cancel_at = ExpiresAt).
package payment_tests

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"testing"
	"time"

	"soli/formations/src/payment/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v85"
)

// buildBulkPackCreatedWebhook is buildBulkCreatedWebhook plus the pack length
// (metadata duration_days), which routes the handler through ResolvePackTerms'
// learner-day branch.
func buildBulkPackCreatedWebhook(eventID, stripeSubID, planID string, learners, durationDays int) []byte {
	now := time.Now().Unix()
	end := time.Now().Add(30 * 24 * time.Hour).Unix()
	return []byte(fmt.Sprintf(`{
		"id": %q,
		"object": "event",
		"api_version": %q,
		"type": "customer.subscription.created",
		"created": %d,
		"data": {
			"object": {
				"id": %q,
				"object": "subscription",
				"customer": {"id": "cus_pack_days", "object": "customer"},
				"status": "active",
				"cancel_at_period_end": false,
				"metadata": {
					"user_id": "user_pack_days",
					"subscription_plan_id": %q,
					"quantity": "%d",
					"duration_days": "%d",
					"bulk_purchase": "true"
				},
				"items": {
					"object": "list",
					"data": [{
						"id": "si_pack_days",
						"object": "subscription_item",
						"current_period_start": %d,
						"current_period_end": %d,
						"price": {"id": "price_pack_days", "object": "price", "currency": "eur"}
					}]
				}
			}
		}
	}`, eventID, stripe.APIVersion, now, stripeSubID, planID, learners, durationDays, now, end))
}

// TestWebhook_BulkPackCreated_SchedulesStripeCancellationAtPackExpiry — see the
// file header for the re-billing scenario this pins.
func TestWebhook_BulkPackCreated_SchedulesStripeCancellationAtPackExpiry(t *testing.T) {
	db := freshTestDB(t)
	cap := installFakeStripeBackend(t)
	webhookSecret := "whsec_pack_" + uuid.NewString()
	router := newRouterWithRealService(t, db, webhookSecret)

	priceID := "price_pack_days"
	plan := &models.SubscriptionPlan{
		Name:             "Pack Jours Test",
		PriceAmount:      165,
		Currency:         "eur",
		BillingInterval:  "month",
		StripePriceID:    &priceID,
		IsActive:         true,
		SeatUnit:         models.SeatUnitLearnerDay,
		BulkPurchasable:  true,
		UseTieredPricing: true,
	}
	require.NoError(t, db.Create(plan).Error)

	const learners, durationDays = 4, 3
	stripeSubID := "sub_pack_" + uuid.NewString()
	eventID := "evt_pack_" + uuid.NewString()
	payload := buildBulkPackCreatedWebhook(eventID, stripeSubID, plan.ID.String(), learners, durationDays)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", bytes.NewReader(payload))
	req.Header.Set("Stripe-Signature", signStripeWebhook(t, payload, webhookSecret))
	req.Header.Set("User-Agent", "Stripe/1.0 (+https://stripe.com/docs/webhooks)")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code,
		"pack bulk-created webhook must process successfully (body: %s)", w.Body.String())

	// Provisioning sanity: one batch, `learners` licences, all carrying the deadline.
	var batch models.SubscriptionBatch
	require.NoError(t, db.Where("stripe_subscription_id = ?", stripeSubID).First(&batch).Error)
	assert.Equal(t, learners, batch.TotalQuantity, "a pack covers LEARNERS, not learner-days")
	var licences []models.UserSubscription
	require.NoError(t, db.Where("subscription_batch_id = ?", batch.ID).Find(&licences).Error)
	require.Len(t, licences, learners)
	for _, l := range licences {
		require.NotNil(t, l.ExpiresAt, "every pack licence must carry the pack deadline")
	}

	// The Stripe subscription must be scheduled to die with the pack — otherwise
	// it renews monthly and the purchaser is re-billed for an expired pack.
	updates := cap.getPayloads("subscription_update")
	require.NotEmpty(t, updates,
		"RE-BILLING: the handler never told Stripe the pack is finite — the "+
			"monthly subscription behind a %d-day pack will renew and charge the "+
			"purchaser again. Expected a subscription update scheduling cancel_at "+
			"at the pack deadline.", durationDays)

	cancelAtRe := regexp.MustCompile(`cancel_at=(\d+)`)
	m := cancelAtRe.FindStringSubmatch(updates[0])
	require.NotNil(t, m, "subscription update must carry cancel_at (body: %s)", updates[0])
	cancelAt, err := strconv.ParseInt(m[1], 10, 64)
	require.NoError(t, err)

	wantDeadline := licences[0].ExpiresAt.Unix()
	assert.InDelta(t, wantDeadline, cancelAt, 120,
		"cancel_at must be the pack deadline (licence ExpiresAt), so billing and "+
			"entitlement end together")
}
