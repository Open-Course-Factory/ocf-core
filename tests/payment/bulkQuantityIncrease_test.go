// tests/payment/bulkQuantityIncrease_test.go
//
// RED-phase failing tests for the follow-up to #367 / MR !269: the bulk
// QUANTITY-INCREASE paths still carry the two defects the created-path fix
// removed.
//
// Both paths add license rows with StripeSubscriptionID = the batch's (shared)
// Stripe subscription id, which collides on the partial unique index
// idx_user_stripe_sub_not_null once a second non-null row is inserted. And the
// webhook path additionally log-and-continues on the insert error.
//
//  1. handleBulkSubscriptionUpdated (stripeService.go) — the difference>0 branch
//     creates each added license with StripeSubscriptionID:&stripeSubID and calls
//     CreateUserSubscription with log-and-continue (no rollback, returns nil), so
//     adding >=2 licenses silently under-provisions and the webhook still 200s.
//  2. UpdateBatchQuantity (bulkLicenseService.go) — raises the Stripe quantity
//     FIRST, then creates added licenses with the same shared id inside a
//     transaction; a >=2 increase collides and rolls the whole tx back AFTER
//     Stripe was already raised → Stripe/DB divergence, and the call errors.
//
// The trigger-injection test reuses the deterministic SQLite BEFORE INSERT
// trigger technique from bulkWebhookTransactional_test.go, keyed on
// subscription_batch_id (independent of the shared-id collision) so it stays
// meaningful after the fix leaves added-license StripeSubscriptionID NULL.
package payment_tests

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v85"
	"gorm.io/gorm"
)

// buildBulkUpdatedWebhook builds a signed customer.subscription.updated event
// flagged as bulk (metadata bulk_purchase=true) carrying the new quantity on the
// subscription item, routed to handleBulkSubscriptionUpdated.
func buildBulkUpdatedWebhook(eventID, stripeSubID string, newQuantity int) []byte {
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
				"customer": {"id": "cus_bulk_upd", "object": "customer"},
				"status": "active",
				"cancel_at_period_end": false,
				"metadata": {"bulk_purchase": "true"},
				"items": {
					"object": "list",
					"data": [{
						"id": "si_bulk_upd",
						"object": "subscription_item",
						"quantity": %d,
						"current_period_start": %d,
						"current_period_end": %d,
						"price": {"id": "price_bulk_tx", "object": "price", "currency": "eur", "unit_amount": 1999}
					}]
				}
			}
		}
	}`, eventID, stripe.APIVersion, now, stripeSubID, newQuantity, now, end))
}

// provisionBulkBatchViaWebhook fires a bulk created event (quantity=2) through
// the fixed creation path and returns the batch. Used to set up an already-
// provisioned batch for the quantity-increase tests.
func provisionBulkBatchViaWebhook(t *testing.T, db *gorm.DB, router *gin.Engine, secret, stripeSubID, planID string) *models.SubscriptionBatch {
	t.Helper()
	eventID := "evt_bulkcreate_" + uuid.NewString()
	payload := buildBulkCreatedWebhook(eventID, stripeSubID, planID, 2)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, buildSignedWebhookRequest(t, payload, secret))
	require.Equal(t, http.StatusOK, w.Code, "batch provisioning (created event) should 200; body: %s", w.Body.String())

	var batch models.SubscriptionBatch
	require.NoError(t, db.Where("stripe_subscription_id = ?", stripeSubID).First(&batch).Error,
		"provisioned batch must exist after the created event")
	var licenseCount int64
	require.NoError(t, db.Model(&models.UserSubscription{}).
		Where("subscription_batch_id = ?", batch.ID).Count(&licenseCount).Error)
	require.Equal(t, int64(2), licenseCount, "created path should have provisioned exactly 2 licenses")
	return &batch
}

func seedBulkIncreasePlan(t *testing.T, db *gorm.DB) *models.SubscriptionPlan {
	t.Helper()
	priceID := "price_bulk_tx"
	plan := &models.SubscriptionPlan{
		Name:            "Bulk Increase Plan",
		PriceAmount:     1999,
		Currency:        "eur",
		BillingInterval: "month",
		StripePriceID:   &priceID,
		IsActive:        true,
	}
	require.NoError(t, db.Create(plan).Error)
	return plan
}

// TestWebhook_BulkSubscriptionUpdated_QuantityIncrease_CreatesExactDelta pins the
// happy path of the webhook increase path: a batch provisioned at quantity 2,
// then a bulk updated event raising quantity to 5, must persist exactly 5 total
// license rows.
//
// RED today: the delta loop creates added licenses with the batch's shared
// StripeSubscriptionID, so only the first added license inserts and the rest
// collide on idx_user_stripe_sub_not_null (swallowed by log-and-continue) — 3
// total instead of 5, yet the webhook still returns 200.
func TestWebhook_BulkSubscriptionUpdated_QuantityIncrease_CreatesExactDelta(t *testing.T) {
	db := freshTestDB(t)
	webhookSecret := "whsec_bulkinc_" + uuid.NewString()
	router := newRouterWithRealService(t, db, webhookSecret)

	plan := seedBulkIncreasePlan(t, db)
	stripeSubID := "sub_bulkinc_" + uuid.NewString()
	batch := provisionBulkBatchViaWebhook(t, db, router, webhookSecret, stripeSubID, plan.ID.String())

	// Raise quantity 2 -> 5 (delta 3).
	eventID := "evt_bulkupd_" + uuid.NewString()
	payload := buildBulkUpdatedWebhook(eventID, stripeSubID, 5)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, buildSignedWebhookRequest(t, payload, webhookSecret))

	assert.Equal(t, http.StatusOK, w.Code,
		"a valid bulk quantity increase must return 200 (body: %s)", w.Body.String())

	var licenseCount int64
	require.NoError(t, db.Model(&models.UserSubscription{}).
		Where("subscription_batch_id = ?", batch.ID).Count(&licenseCount).Error)
	assert.Equal(t, int64(5), licenseCount,
		"PROVISIONING: raising a bulk batch from 2 to 5 must create exactly 5 "+
			"license rows. Today the delta loop reuses the batch's shared "+
			"stripe_subscription_id, so added rows 2..N collide on "+
			"idx_user_stripe_sub_not_null and are swallowed by log-and-continue — "+
			"leaving 3. Fix: mirror the created path (leave added-license "+
			"StripeSubscriptionID NULL) and provision the delta atomically.")
}

// TestWebhook_BulkSubscriptionUpdated_LicenseCreationFailure_Propagates pins the
// transactionality of the webhook increase path. A mid-delta insert failure
// (injected by a BEFORE INSERT trigger) must surface as non-2xx / event failed
// and leave NO partial delta rows.
//
// RED today: handleBulkSubscriptionUpdated log-and-continues and returns nil, so
// the controller 200s / marks the event processed, and the first delta license
// persists (partial provisioning).
func TestWebhook_BulkSubscriptionUpdated_LicenseCreationFailure_Propagates(t *testing.T) {
	db := freshTestDB(t)
	webhookSecret := "whsec_bulkincfail_" + uuid.NewString()
	router := newRouterWithRealService(t, db, webhookSecret)

	plan := seedBulkIncreasePlan(t, db)
	stripeSubID := "sub_bulkincfail_" + uuid.NewString()
	batch := provisionBulkBatchViaWebhook(t, db, router, webhookSecret, stripeSubID, plan.ID.String())

	// Fail the SECOND delta insert: the batch already holds 2 licenses, so abort
	// once a 3rd batch-license row exists (2 seeded + 1 delta). Keyed on
	// subscription_batch_id so it fires regardless of how the fix sets the id.
	require.NoError(t, db.Exec(`
		CREATE TRIGGER fail_delta_license BEFORE INSERT ON user_subscriptions
		WHEN NEW.subscription_batch_id IS NOT NULL
		 AND (SELECT COUNT(*) FROM user_subscriptions WHERE subscription_batch_id IS NOT NULL) >= 3
		BEGIN
			SELECT RAISE(ABORT, 'injected mid-delta license failure');
		END;`).Error)
	t.Cleanup(func() { db.Exec(`DROP TRIGGER IF EXISTS fail_delta_license`) })

	eventID := "evt_bulkupdfail_" + uuid.NewString()
	payload := buildBulkUpdatedWebhook(eventID, stripeSubID, 5)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, buildSignedWebhookRequest(t, payload, webhookSecret))

	assert.GreaterOrEqual(t, w.Code, 500,
		"TRANSACTIONALITY: a mid-delta license-creation failure must surface as "+
			"5xx so Stripe retries. Today handleBulkSubscriptionUpdated logs the "+
			"failed insert and returns nil, so the controller returns 200 (body: %s).",
		w.Body.String())
	assert.Equal(t, "failed", fetchWebhookEventStatus(t, db, eventID),
		"the webhook_event row must be 'failed' (re-deliverable) after a partial "+
			"delta failure, not 'processed'")

	// No partial delta may persist: the batch must remain at its original 2.
	var licenseCount int64
	require.NoError(t, db.Model(&models.UserSubscription{}).
		Where("subscription_batch_id = ?", batch.ID).Count(&licenseCount).Error)
	assert.Equal(t, int64(2), licenseCount,
		"ATOMICITY: a failed quantity increase must roll the whole delta back — no "+
			"partial license rows. Today the first delta license survives "+
			"(log-and-continue swallows the aborted inserts), leaving 3.")
}

// TestBulkLicenseService_UpdateBatchQuantity_IncreaseByTwo_CreatesExactDelta
// pins the service-level increase path. A batch with 2 licenses, increased to 4,
// must create exactly 2 new license rows and return no error.
//
// The Stripe side is faked via NewBulkLicenseServiceWithDeps + the existing
// webhookTestStripeService mock (UpdateSubscriptionQuantity returns nil), so the
// test exercises only the local license/batch bookkeeping.
//
// RED today: UpdateBatchQuantity raises the Stripe quantity, then creates added
// licenses with the batch's shared StripeSubscriptionID inside its transaction;
// the 2nd added license collides on idx_user_stripe_sub_not_null, the tx rolls
// back (leaving 2 licenses) and the call returns an error — divergence from the
// already-raised Stripe quantity.
func TestBulkLicenseService_UpdateBatchQuantity_IncreaseByTwo_CreatesExactDelta(t *testing.T) {
	db := freshTestDB(t)
	plan := seedBulkIncreasePlan(t, db)

	purchaserID := "user_svc_" + uuid.NewString()
	stripeSubID := "sub_svc_" + uuid.NewString()
	stripeCustomerID := "cus_svc_" + uuid.NewString()

	batch := &models.SubscriptionBatch{
		PurchaserUserID:          purchaserID,
		SubscriptionPlanID:       plan.ID,
		StripeSubscriptionID:     stripeSubID,
		StripeSubscriptionItemID: "si_svc",
		TotalQuantity:            2,
		AssignedQuantity:         0,
		Status:                   "active",
		CurrentPeriodStart:       time.Now(),
		CurrentPeriodEnd:         time.Now().Add(30 * 24 * time.Hour),
	}
	require.NoError(t, db.Create(batch).Error)

	// Seed 2 existing licenses with NULL StripeSubscriptionID (as the fixed
	// created path leaves them), linked via the batch.
	for i := 0; i < 2; i++ {
		lic := &models.UserSubscription{
			UserID:              "",
			PurchaserUserID:     &purchaserID,
			SubscriptionBatchID: &batch.ID,
			SubscriptionPlanID:  plan.ID,
			StripeCustomerID:    &stripeCustomerID,
			Status:              "unassigned",
			CurrentPeriodStart:  batch.CurrentPeriodStart,
			CurrentPeriodEnd:    batch.CurrentPeriodEnd,
		}
		require.NoError(t, db.Create(lic).Error)
	}

	svc := services.NewBulkLicenseServiceWithDeps(db, &webhookTestStripeService{})

	// Increase 2 -> 4 (delta 2).
	err := svc.UpdateBatchQuantity(batch.ID, purchaserID, 4)
	assert.NoError(t, err,
		"ATOMICITY/PROVISIONING: increasing a bulk batch by 2 must succeed. Today "+
			"UpdateBatchQuantity creates added licenses with the batch's shared "+
			"stripe_subscription_id; the 2nd collides on idx_user_stripe_sub_not_null, "+
			"the transaction rolls back and this returns an error — AFTER the Stripe "+
			"quantity was already raised (Stripe/DB divergence). Fix: leave added "+
			"licenses' StripeSubscriptionID NULL.")

	var licenseCount int64
	require.NoError(t, db.Model(&models.UserSubscription{}).
		Where("subscription_batch_id = ?", batch.ID).Count(&licenseCount).Error)
	assert.Equal(t, int64(4), licenseCount,
		"a +2 increase must leave exactly 4 license rows (2 original + 2 added); "+
			"today the collision rolls the delta back to 2")
}
