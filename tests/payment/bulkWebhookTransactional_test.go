// tests/payment/bulkWebhookTransactional_test.go
//
// RED-phase failing tests for issue #367 / MR !269: bulk subscription creation
// is not transactional.
//
// `handleBulkSubscriptionCreated` (src/payment/services/stripeService.go) creates
// the SubscriptionBatch, then loops `TotalQuantity` times creating UserSubscription
// license rows. On a per-row insert error it merely logs (`utils.Error(...)`) and
// CONTINUES, then returns nil. The webhook controller therefore marks the event
// `processed` and returns 200, so Stripe never retries — leaving a paid batch
// with fewer license rows than TotalQuantity.
//
// Desired behavior: the batch and all license rows are created atomically in one
// DB transaction; any failure rolls everything back and returns an error, so the
// webhook is marked `failed` and is re-deliverable via the reservation scheme.
//
// Deterministic failure injection (no src seam, no fragile mocking):
// the unit-test DB is in-memory SQLite, so the rollback test installs a
// temporary BEFORE INSERT trigger on user_subscriptions that RAISE(ABORT)s once
// a license row already exists for the batch — forcing the SECOND license insert
// to fail MID-LOOP, after the first has been created. The trigger is dropped via
// t.Cleanup. RAISE(ABORT) surfaces as an error from GORM's Create, so with an
// atomic handler the whole batch (batch row + already-inserted licenses) must
// roll back and the webhook must be marked failed.
//
// This injection is deliberately INDEPENDENT of the earlier shared-
// stripe_subscription_id collision: the fix legitimately removes that collision
// by leaving license rows' StripeSubscriptionID NULL (linkage via
// SubscriptionBatchID), so the trigger — not the unique index — is what drives
// the mid-loop failure here. It therefore keeps pinning the transactionality/
// rollback requirement regardless of how the shared-id issue is resolved.
package payment_tests

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"soli/formations/src/payment/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v85"
)

// buildBulkCreatedWebhook builds a signed customer.subscription.created event
// flagged as a bulk purchase (metadata bulk_purchase=true) with the given
// quantity, routed to handleBulkSubscriptionCreated.
func buildBulkCreatedWebhook(eventID, stripeSubID, planID string, quantity int) []byte {
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
				"customer": {"id": "cus_bulk_tx", "object": "customer"},
				"status": "active",
				"cancel_at_period_end": false,
				"metadata": {
					"user_id": "user_bulk_tx",
					"subscription_plan_id": %q,
					"quantity": "%d",
					"bulk_purchase": "true"
				},
				"items": {
					"object": "list",
					"data": [{
						"id": "si_bulk_tx",
						"object": "subscription_item",
						"current_period_start": %d,
						"current_period_end": %d,
						"price": {"id": "price_bulk_tx", "object": "price", "currency": "eur", "unit_amount": 1999}
					}]
				}
			}
		}
	}`, eventID, stripe.APIVersion, now, stripeSubID, planID, quantity, now, end))
}

// TestWebhook_BulkSubscriptionCreated_LicenseCreationFailure_RollsBackAndFails
// pins the transactionality requirement. A quantity>=2 bulk creation hits a
// mid-loop insert failure — injected by a BEFORE INSERT trigger that aborts the
// 2nd license insert (see the file header). The handler must roll back the batch
// AND every already-created license and return an error so the webhook is marked
// failed and Stripe can retry.
//
// Against original (non-transactional) code: the batch is committed before the
// loop and the first license survives (log-and-continue swallows the aborted
// rows), so this fails on the status assertion (200/processed) AND the
// no-partial-rows assertions (1 batch, 1 license persist).
func TestWebhook_BulkSubscriptionCreated_LicenseCreationFailure_RollsBackAndFails(t *testing.T) {
	db := freshTestDB(t)
	webhookSecret := "whsec_bulktx_" + uuid.NewString()
	router := newRouterWithRealService(t, db, webhookSecret)

	priceID := "price_bulk_tx"
	plan := &models.SubscriptionPlan{
		Name:            "Bulk Tx Plan",
		PriceAmount:     1999,
		Currency:        "eur",
		BillingInterval: "month",
		StripePriceID:   &priceID,
		IsActive:        true,
	}
	require.NoError(t, db.Create(plan).Error)

	// Inject a deterministic mid-loop failure: abort the INSERT once one license
	// row already exists for a batch, so the 2nd license insert fails while the
	// 1st is pending in the transaction. Scoped to batch licenses so unrelated
	// user_subscriptions rows can't trip it. Dropped after the test.
	require.NoError(t, db.Exec(`
		CREATE TRIGGER fail_second_bulk_license BEFORE INSERT ON user_subscriptions
		WHEN NEW.subscription_batch_id IS NOT NULL
		 AND (SELECT COUNT(*) FROM user_subscriptions WHERE subscription_batch_id IS NOT NULL) >= 1
		BEGIN
			SELECT RAISE(ABORT, 'injected mid-loop license failure');
		END;`).Error)
	t.Cleanup(func() {
		db.Exec(`DROP TRIGGER IF EXISTS fail_second_bulk_license`)
	})

	stripeSubID := "sub_bulktx_" + uuid.NewString()
	eventID := "evt_bulktx_" + uuid.NewString()
	// quantity=3: license 1 inserts, license 2 aborts (trigger) mid-loop.
	payload := buildBulkCreatedWebhook(eventID, stripeSubID, plan.ID.String(), 3)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, buildSignedWebhookRequest(t, payload, webhookSecret))

	// The event must NOT be reported as successfully processed — the partial
	// failure has to surface so Stripe retries.
	assert.GreaterOrEqual(t, w.Code, 500,
		"TRANSACTIONALITY: a license-creation failure mid-batch must surface as "+
			"5xx so Stripe retries. Non-transactional handleBulkSubscriptionCreated "+
			"logs the failed insert and returns nil, so the controller returns 200 "+
			"(body: %s).",
		w.Body.String())
	assert.Equal(t, "failed", fetchWebhookEventStatus(t, db, eventID),
		"the webhook_event row must be 'failed' (re-deliverable) after a partial "+
			"batch failure, not 'processed'")

	// No partial state may remain: the batch and every license must be rolled back.
	var batchCount int64
	require.NoError(t, db.Model(&models.SubscriptionBatch{}).
		Where("stripe_subscription_id = ?", stripeSubID).Count(&batchCount).Error)
	assert.Equal(t, int64(0), batchCount,
		"ATOMICITY: the SubscriptionBatch must be rolled back when license "+
			"creation fails. Today the batch is created before the loop and never "+
			"rolled back, leaving a paid batch with missing licenses.")

	var licenseCount int64
	require.NoError(t, db.Model(&models.UserSubscription{}).
		Where("subscription_batch_id IS NOT NULL").Count(&licenseCount).Error)
	assert.Equal(t, int64(0), licenseCount,
		"ATOMICITY: no license rows may persist after a rolled-back bulk creation. "+
			"Without a transaction the first license row survives (the aborted "+
			"inserts are swallowed by log-and-continue), leaving an orphaned license.")
}

// TestWebhook_BulkSubscriptionCreated_Success_CreatesExactQuantity pins the
// happy path: a quantity=N bulk creation must persist exactly N license rows and
// exactly 1 batch, and return 200.
//
// Status today: RED. The probe shows quantity=3 yields only 1 license row (rows
// 2..N collide on idx_user_stripe_sub_not_null because every license carries the
// same StripeSubscriptionID). IMPORTANT for the implementer: wrapping the create
// in a transaction alone will NOT make this pass — the collision would roll the
// whole batch back to 0 rows and fail the webhook forever. To provision N
// licenses atomically, the fix must ALSO stop bulk license rows from sharing one
// stripe_subscription_id under the partial unique index (e.g. leave
// StripeSubscriptionID null on license rows and rely on SubscriptionBatchID for
// linkage, or scope the index to exclude batch rows). This test guards the true
// end state: a fully-provisioned batch.
func TestWebhook_BulkSubscriptionCreated_Success_CreatesExactQuantity(t *testing.T) {
	db := freshTestDB(t)
	webhookSecret := "whsec_bulkok_" + uuid.NewString()
	router := newRouterWithRealService(t, db, webhookSecret)

	priceID := "price_bulk_tx"
	plan := &models.SubscriptionPlan{
		Name:            "Bulk OK Plan",
		PriceAmount:     1999,
		Currency:        "eur",
		BillingInterval: "month",
		StripePriceID:   &priceID,
		IsActive:        true,
	}
	require.NoError(t, db.Create(plan).Error)

	stripeSubID := "sub_bulkok_" + uuid.NewString()
	eventID := "evt_bulkok_" + uuid.NewString()
	const quantity = 3
	payload := buildBulkCreatedWebhook(eventID, stripeSubID, plan.ID.String(), quantity)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, buildSignedWebhookRequest(t, payload, webhookSecret))

	assert.Equal(t, http.StatusOK, w.Code,
		"a valid bulk creation must succeed with 200 (body: %s)", w.Body.String())

	var batch models.SubscriptionBatch
	require.NoError(t, db.Where("stripe_subscription_id = ?", stripeSubID).First(&batch).Error,
		"exactly one SubscriptionBatch must exist for the bulk subscription")

	var licenseCount int64
	require.NoError(t, db.Model(&models.UserSubscription{}).
		Where("subscription_batch_id = ?", batch.ID).Count(&licenseCount).Error)
	assert.Equal(t, int64(quantity), licenseCount,
		"PROVISIONING: a batch of %d must create exactly %d license rows. Today "+
			"only 1 is created because rows 2..N share the batch's "+
			"stripe_subscription_id and collide on idx_user_stripe_sub_not_null. "+
			"A transaction-only fix rolls this back to 0 — the fix must also stop "+
			"license rows from sharing one stripe_subscription_id.", quantity, quantity)
}
