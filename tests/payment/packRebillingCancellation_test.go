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

// buildBulkPackUpdatedWebhook builds a signed customer.subscription.updated event
// for a bulk subscription that is STILL ACTIVE but carries a scheduled future
// end (cancel_at) — exactly what Stripe emits right after the pack's own
// schedulePackCancellation call. The item quantity is the BILLING UNITS
// (learners × days), as on the real subscription — not the licence count.
func buildBulkPackUpdatedWebhook(eventID, stripeSubID, planID string, learners, billingUnits int, cancelAt int64) []byte {
	now := time.Now().Unix()
	end := time.Now().Add(30 * 24 * time.Hour).Unix()
	return []byte(fmt.Sprintf(`{
		"id": %q,
		"object": "event",
		"api_version": %q,
		"type": "customer.subscription.updated",
		"created": %d,
		"data": {
			"object": {
				"id": %q,
				"object": "subscription",
				"customer": {"id": "cus_pack_days", "object": "customer"},
				"status": "active",
				"cancel_at": %d,
				"canceled_at": %d,
				"cancel_at_period_end": false,
				"metadata": {
					"user_id": "user_pack_days",
					"subscription_plan_id": %q,
					"quantity": "%d",
					"duration_days": "3",
					"bulk_purchase": "true"
				},
				"items": {
					"object": "list",
					"data": [{
						"id": "si_pack_days",
						"object": "subscription_item",
						"quantity": %d,
						"current_period_start": %d,
						"current_period_end": %d,
						"price": {"id": "price_pack_days", "object": "price", "currency": "eur"}
					}]
				}
			}
		}
	}`, eventID, stripe.APIVersion, now, stripeSubID, cancelAt, now, planID, learners, billingUnits, now, end))
}

// TestWebhook_BulkPackUpdated_ScheduledEndDoesNotCancelBatch pins that the
// pack's own scheduled Stripe cancellation (cancel_at = pack deadline, status
// still active) must NOT cancel the batch: the seats live until the deadline,
// which entitlement expiry already enforces. Without this the fix that stops
// monthly re-billing kills every pack at birth — the batch shows ANNULÉ with
// all its seats cancelled seconds after purchase.
func TestWebhook_BulkPackUpdated_ScheduledEndDoesNotCancelBatch(t *testing.T) {
	db := freshTestDB(t)
	webhookSecret := "whsec_packupd_" + uuid.NewString()
	router := newRouterWithRealService(t, db, webhookSecret)

	priceID := "price_pack_days"
	plan := &models.SubscriptionPlan{
		Name:             "Pack Jours Upd Test",
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

	const learners = 4
	stripeSubID := "sub_packupd_" + uuid.NewString()
	deadline := time.Now().Add(3 * 24 * time.Hour)
	batch := &models.SubscriptionBatch{
		PurchaserUserID:      "user_pack_days",
		SubscriptionPlanID:   plan.ID,
		StripeSubscriptionID: stripeSubID,
		TotalQuantity:        learners,
		Status:               "active",
		CurrentPeriodStart:   time.Now(),
		CurrentPeriodEnd:     deadline,
	}
	require.NoError(t, db.Create(batch).Error)
	for i := 0; i < learners; i++ {
		require.NoError(t, db.Create(&models.UserSubscription{
			PurchaserUserID:     &batch.PurchaserUserID,
			SubscriptionBatchID: &batch.ID,
			SubscriptionPlanID:  plan.ID,
			Status:              "unassigned",
			CurrentPeriodStart:  batch.CurrentPeriodStart,
			CurrentPeriodEnd:    batch.CurrentPeriodEnd,
			ExpiresAt:           &deadline,
		}).Error)
	}

	eventID := "evt_packupd_" + uuid.NewString()
	const durationDays = 3
	payload := buildBulkPackUpdatedWebhook(eventID, stripeSubID, plan.ID.String(),
		learners, learners*durationDays, deadline.Unix())
	req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", bytes.NewReader(payload))
	req.Header.Set("Stripe-Signature", signStripeWebhook(t, payload, webhookSecret))
	req.Header.Set("User-Agent", "Stripe/1.0 (+https://stripe.com/docs/webhooks)")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "updated webhook must process (body: %s)", w.Body.String())

	var reloaded models.SubscriptionBatch
	require.NoError(t, db.First(&reloaded, "id = ?", batch.ID).Error)
	assert.Equal(t, "active", reloaded.Status,
		"a SCHEDULED future end (the pack's own cancel_at) must not cancel the batch — "+
			"the seats live until the deadline")
	assert.Nil(t, reloaded.CancelledAt)

	var cancelled int64
	db.Model(&models.UserSubscription{}).
		Where("subscription_batch_id = ? AND status = ?", batch.ID, "cancelled").
		Count(&cancelled)
	assert.Zero(t, cancelled, "no licence may be cancelled by a scheduled future end")

	// And the quantity reconciliation must read the item quantity as BILLING
	// UNITS for a pack: 12 learner-days is still 4 licences, not 12 (#455 on
	// the update path — without the conversion the batch inflates to 12 seats).
	var licenceCount int64
	db.Model(&models.UserSubscription{}).
		Where("subscription_batch_id = ?", batch.ID).Count(&licenceCount)
	assert.EqualValues(t, learners, licenceCount,
		"the update handler must not create licences out of billing units")
	require.NoError(t, db.First(&reloaded, "id = ?", batch.ID).Error)
	assert.Equal(t, learners, reloaded.TotalQuantity)
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
