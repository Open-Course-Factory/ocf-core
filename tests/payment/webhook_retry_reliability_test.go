// tests/payment/webhook_retry_reliability_test.go
//
// RED-phase failing tests for the "webhook retry reliability" work
// (branch fix/webhook-retry-reliability). They pin three distinct bugs in the
// Stripe webhook path. Each test drives the REAL StripeService through the real
// webhook controller with valid Stripe signatures (reusing the harness from
// webhookIdempotency_test.go), so the failures reflect production behavior.
//
// Bug 1 — age gate drops Stripe retries
//   src/payment/routes/webHookController.go rejects any event whose
//   `event.Created` is older than 10 minutes. Stripe does NOT refresh
//   `event.Created` when it redelivers a failed event, so every retried event
//   is permanently dropped once 10 minutes have elapsed since the original
//   creation. Desired: a well-signed, not-yet-processed event is handled even
//   if its `Created` is hours old.
//
// Bug 2 — panic on empty Items.Data
//   stripeService.handleSubscriptionUpdated reads subscription.Items.Data[0]
//   unconditionally (~:916-917), and handleBulkSubscriptionUpdated does the
//   same (~:1797, :1848-1849). A customer.subscription.updated event with an
//   empty items list panics with index-out-of-range. Compare with
//   handleSubscriptionCreated (~:735) which guards the empty case. Desired:
//   the handler returns gracefully (error or no-op), no panic.
//
// Bug 3 — invoice webhooks miss past_due subscriptions
//   The invoice handlers resolve the subscription via
//   GetActiveSubscriptionByCustomerID, which matches only status
//   active/trialing (paymentRepository.go:197). A customer whose subscription
//   is past_due (the exact case a successful invoice payment is meant to cure)
//   is never found, so the invoice is not recorded and the status stays
//   past_due. Desired: invoice.payment_succeeded records the invoice AND
//   returns the subscription to active.
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

// -----------------------------------------------------------------------------
// Bug 1 — age gate drops Stripe retries
// -----------------------------------------------------------------------------

// TestWebhook_OldEventFromStripeRetry_IsProcessed submits a validly-signed,
// not-yet-seen customer.subscription.created event whose `created` timestamp is
// 2 hours in the past (as happens when Stripe redelivers a previously-failed
// event — Stripe keeps the ORIGINAL created time on retries). The event must be
// processed.
//
// Today the controller's age gate
//
//	if time.Since(time.Unix(event.Created, 0)) > 10*time.Minute { ...400... }
//
// rejects it with 400 "Event too old" before it is ever handled, so a retry of
// a genuinely important event (e.g. a subscription creation) is silently lost.
//
// NOTE: the signature timestamp (Stripe-Signature `t=`) is fresh — the harness
// signs with time.Now() — so ValidateWebhookSignature passes. Only the in-body
// `created` field is old. This isolates the age-gate bug from signature
// tolerance.
func TestWebhook_OldEventFromStripeRetry_IsProcessed(t *testing.T) {
	db := freshTestDB(t)
	webhookSecret := "whsec_test_" + uuid.NewString()
	router := newRouterWithRealService(t, db, webhookSecret)

	// Seed a plan so the created-subscription handler's price lookup succeeds.
	stripePriceID := "price_oldretry_" + uuid.NewString()
	plan := &models.SubscriptionPlan{
		Name:            "Old Retry Plan",
		Description:     "plan for age-gate retry test",
		PriceAmount:     1999,
		Currency:        "eur",
		BillingInterval: "month",
		StripePriceID:   &stripePriceID,
		IsActive:        true,
	}
	require.NoError(t, db.Create(plan).Error)

	stripeSubID := "sub_oldretry_" + uuid.NewString()
	eventID := "evt_oldretry_" + uuid.NewString()

	// event.Created is 2 hours ago — well beyond the 10-minute age gate.
	oldCreated := time.Now().Add(-2 * time.Hour).Unix()
	periodStart := time.Now().Add(-2 * time.Hour).Unix()
	periodEnd := time.Now().Add(28 * 24 * time.Hour).Unix()

	payload := []byte(fmt.Sprintf(`{
		"id": %q,
		"object": "event",
		"api_version": %q,
		"type": "customer.subscription.created",
		"created": %d,
		"data": {
			"object": {
				"id": %q,
				"object": "subscription",
				"customer": {"id": "cus_oldretry", "object": "customer"},
				"status": "active",
				"cancel_at_period_end": false,
				"metadata": {"user_id": "user_oldretry"},
				"items": {
					"object": "list",
					"data": [{
						"id": "si_oldretry",
						"object": "subscription_item",
						"current_period_start": %d,
						"current_period_end": %d,
						"price": {"id": %q, "object": "price", "currency": "eur", "unit_amount": 1999}
					}]
				}
			}
		}
	}`, eventID, stripe.APIVersion, oldCreated, stripeSubID, periodStart, periodEnd, stripePriceID))

	w := httptest.NewRecorder()
	router.ServeHTTP(w, buildSignedWebhookRequest(t, payload, webhookSecret))

	assert.Equal(t, http.StatusOK, w.Code,
		"RETRY: a validly-signed event whose Created is 2h old (a Stripe "+
			"redelivery) must be processed, not rejected as 'too old'. Stripe "+
			"does not refresh event.Created on retries, so the 10-minute age "+
			"gate in webHookController.go permanently drops retried events. "+
			"Fix: base anti-replay on the signature timestamp / dedup state, "+
			"not on event.Created. Response body: %s", w.Body.String())

	// Prove it was actually PROCESSED (not merely admitted): the subscription
	// row must exist.
	var count int64
	require.NoError(t, db.Model(&models.UserSubscription{}).
		Where("stripe_subscription_id = ?", stripeSubID).
		Count(&count).Error)
	assert.Equal(t, int64(1), count,
		"the old-but-valid event must be fully handled — exactly one "+
			"UserSubscription row should have been created")
}

// -----------------------------------------------------------------------------
// Bug 2 — panic on empty Items.Data
// -----------------------------------------------------------------------------

// TestWebhook_SubscriptionUpdated_EmptyItems_DoesNotPanic feeds a
// customer.subscription.updated event whose subscription has an empty
// items.data list to a subscription that already exists locally. The plan-change
// block guards `len(Items.Data) > 0`, but the period-update lines run
// unconditionally:
//
//	userSub.CurrentPeriodStart = time.Unix(subscription.Items.Data[0].CurrentPeriodStart, 0)
//	userSub.CurrentPeriodEnd   = time.Unix(subscription.Items.Data[0].CurrentPeriodEnd, 0)
//
// so an empty list panics with index-out-of-range. The controller has no
// recovery middleware, so the panic escapes ServeHTTP.
//
// Desired: the handler guards the empty case (like handleSubscriptionCreated)
// and returns gracefully — no panic.
func TestWebhook_SubscriptionUpdated_EmptyItems_DoesNotPanic(t *testing.T) {
	db := freshTestDB(t)
	webhookSecret := "whsec_test_" + uuid.NewString()
	router := newRouterWithRealService(t, db, webhookSecret)

	// Seed a plan + an existing subscription so GetUserSubscriptionByStripeID
	// succeeds and the handler proceeds to the (unguarded) period-update lines.
	stripePriceID := "price_emptyitems_" + uuid.NewString()
	plan := &models.SubscriptionPlan{
		Name:            "Empty Items Plan",
		PriceAmount:     1999,
		Currency:        "eur",
		BillingInterval: "month",
		StripePriceID:   &stripePriceID,
		IsActive:        true,
	}
	require.NoError(t, db.Create(plan).Error)

	stripeSubID := "sub_emptyitems_" + uuid.NewString()
	stripeCustomerID := "cus_emptyitems_" + uuid.NewString()
	existing := &models.UserSubscription{
		UserID:               "user_emptyitems",
		SubscriptionPlanID:   plan.ID,
		StripeSubscriptionID: &stripeSubID,
		StripeCustomerID:     &stripeCustomerID,
		Status:               "active",
		CurrentPeriodStart:   time.Now(),
		CurrentPeriodEnd:     time.Now().Add(30 * 24 * time.Hour),
	}
	require.NoError(t, db.Create(existing).Error)

	eventID := "evt_emptyitems_" + uuid.NewString()
	payload := []byte(fmt.Sprintf(`{
		"id": %q,
		"object": "event",
		"api_version": %q,
		"type": "customer.subscription.updated",
		"created": %d,
		"data": {
			"object": {
				"id": %q,
				"object": "subscription",
				"customer": {"id": %q, "object": "customer"},
				"status": "active",
				"cancel_at_period_end": false,
				"metadata": {},
				"items": {"object": "list", "data": []}
			}
		}
	}`, eventID, stripe.APIVersion, time.Now().Unix(), stripeSubID, stripeCustomerID))

	assert.NotPanics(t, func() {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, buildSignedWebhookRequest(t, payload, webhookSecret))
		// Any 2xx/4xx/5xx is acceptable here — the point is "no panic". We do
		// not assert on the status code so the test isolates the panic bug.
	}, "PANIC: handleSubscriptionUpdated dereferences subscription.Items.Data[0] "+
		"unconditionally (webHookController path -> stripeService.go ~:916-917). "+
		"A customer.subscription.updated event with an empty items list panics "+
		"with index-out-of-range. Fix: guard len(Items.Data) > 0 before reading "+
		"the period, mirroring handleSubscriptionCreated.")
}

// TestWebhook_BulkSubscriptionUpdated_EmptyItems_DoesNotPanic is the bulk
// variant. handleBulkSubscriptionUpdated reads
// `int(subscription.Items.Data[0].Quantity)` (~:1797) and the period lines
// (~:1848-1849) without guarding an empty items list. Reaching this path
// requires a bulk-flagged subscription (metadata bulk_purchase=true) whose
// batch exists locally and which is NOT being cancelled.
func TestWebhook_BulkSubscriptionUpdated_EmptyItems_DoesNotPanic(t *testing.T) {
	db := freshTestDB(t)
	webhookSecret := "whsec_test_" + uuid.NewString()
	router := newRouterWithRealService(t, db, webhookSecret)

	stripePriceID := "price_bulkempty_" + uuid.NewString()
	plan := &models.SubscriptionPlan{
		Name:            "Bulk Empty Items Plan",
		PriceAmount:     1999,
		Currency:        "eur",
		BillingInterval: "month",
		StripePriceID:   &stripePriceID,
		IsActive:        true,
	}
	require.NoError(t, db.Create(plan).Error)

	stripeSubID := "sub_bulkempty_" + uuid.NewString()
	batch := &models.SubscriptionBatch{
		PurchaserUserID:      "user_bulkempty",
		SubscriptionPlanID:   plan.ID,
		StripeSubscriptionID: stripeSubID,
		TotalQuantity:        5,
		Status:               "active",
		CurrentPeriodStart:   time.Now(),
		CurrentPeriodEnd:     time.Now().Add(30 * 24 * time.Hour),
	}
	require.NoError(t, db.Create(batch).Error)

	eventID := "evt_bulkempty_" + uuid.NewString()
	// No canceled_at -> not a cancellation, so the handler falls through to the
	// quantity-change block that reads Items.Data[0].
	payload := []byte(fmt.Sprintf(`{
		"id": %q,
		"object": "event",
		"api_version": %q,
		"type": "customer.subscription.updated",
		"created": %d,
		"data": {
			"object": {
				"id": %q,
				"object": "subscription",
				"customer": {"id": "cus_bulkempty", "object": "customer"},
				"status": "active",
				"cancel_at_period_end": false,
				"metadata": {"bulk_purchase": "true"},
				"items": {"object": "list", "data": []}
			}
		}
	}`, eventID, stripe.APIVersion, time.Now().Unix(), stripeSubID))

	assert.NotPanics(t, func() {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, buildSignedWebhookRequest(t, payload, webhookSecret))
	}, "PANIC: handleBulkSubscriptionUpdated reads subscription.Items.Data[0] "+
		"(stripeService.go ~:1797 and ~:1848-1849) without guarding an empty "+
		"items list. A bulk subscription update with empty items panics. Fix: "+
		"guard len(Items.Data) > 0 before reading quantity/period.")
}

// -----------------------------------------------------------------------------
// Bug 3 — invoice webhooks miss past_due subscriptions
// -----------------------------------------------------------------------------

// TestWebhook_InvoicePaymentSucceeded_PastDueSubscription_IsRecoveredAndRecorded
// seeds a UserSubscription in status=past_due and processes an
// invoice.payment_succeeded for that customer. A successful invoice payment is
// precisely the event that should CURE a past_due subscription, yet the invoice
// handlers resolve the subscription with GetActiveSubscriptionByCustomerID,
// whose WHERE clause is `status IN ('active','trialing')`
// (paymentRepository.go:197). A past_due subscription is therefore never found:
// the handler returns an error, the controller returns 500, no invoice row is
// created, and the status stays past_due.
//
// Desired:
//   - the invoice is recorded locally (one Invoice row), and
//   - the subscription status is returned to active.
func TestWebhook_InvoicePaymentSucceeded_PastDueSubscription_IsRecoveredAndRecorded(t *testing.T) {
	db := freshTestDB(t)
	// The invoices table is not part of runTestMigrations — add it for this test.
	require.NoError(t, db.AutoMigrate(&models.Invoice{}))

	webhookSecret := "whsec_test_" + uuid.NewString()
	router := newRouterWithRealService(t, db, webhookSecret)

	stripePriceID := "price_pastdue_" + uuid.NewString()
	plan := &models.SubscriptionPlan{
		Name:            "Past Due Plan",
		PriceAmount:     1999,
		Currency:        "eur",
		BillingInterval: "month",
		StripePriceID:   &stripePriceID,
		IsActive:        true,
	}
	require.NoError(t, db.Create(plan).Error)

	stripeSubID := "sub_pastdue_" + uuid.NewString()
	stripeCustomerID := "cus_pastdue_" + uuid.NewString()
	pastDueSub := &models.UserSubscription{
		UserID:               "user_pastdue",
		SubscriptionPlanID:   plan.ID,
		StripeSubscriptionID: &stripeSubID,
		StripeCustomerID:     &stripeCustomerID,
		Status:               "past_due",
		CurrentPeriodStart:   time.Now().Add(-30 * 24 * time.Hour),
		CurrentPeriodEnd:     time.Now().Add(24 * time.Hour),
	}
	require.NoError(t, db.Create(pastDueSub).Error)

	stripeInvoiceID := "in_pastdue_" + uuid.NewString()
	eventID := "evt_invpastdue_" + uuid.NewString()
	now := time.Now().Unix()
	payload := []byte(fmt.Sprintf(`{
		"id": %q,
		"object": "event",
		"api_version": %q,
		"type": "invoice.payment_succeeded",
		"created": %d,
		"data": {
			"object": {
				"id": %q,
				"object": "invoice",
				"customer": {"id": %q, "object": "customer"},
				"status": "paid",
				"amount_paid": 1999,
				"currency": "eur",
				"number": "INV-PASTDUE-1",
				"created": %d,
				"status_transitions": {"paid_at": %d}
			}
		}
	}`, eventID, stripe.APIVersion, now, stripeInvoiceID, stripeCustomerID, now, now))

	w := httptest.NewRecorder()
	router.ServeHTTP(w, buildSignedWebhookRequest(t, payload, webhookSecret))

	assert.Equal(t, http.StatusOK, w.Code,
		"PAST_DUE: invoice.payment_succeeded for a past_due subscription must "+
			"succeed. Today the invoice handler uses "+
			"GetActiveSubscriptionByCustomerID (status IN active/trialing), so "+
			"a past_due subscription is never found -> 500. Fix: the invoice "+
			"lookup must include past_due (and cure it). Response body: %s",
		w.Body.String())

	// The invoice must be recorded locally.
	var invoiceCount int64
	require.NoError(t, db.Model(&models.Invoice{}).
		Where("stripe_invoice_id = ?", stripeInvoiceID).
		Count(&invoiceCount).Error)
	assert.Equal(t, int64(1), invoiceCount,
		"a successful invoice payment must be recorded even when the "+
			"subscription was past_due")

	// The subscription must be returned to active.
	var updated models.UserSubscription
	require.NoError(t, db.Where("stripe_subscription_id = ?", stripeSubID).
		First(&updated).Error)
	assert.Equal(t, "active", updated.Status,
		"a successful invoice payment must return a past_due subscription to "+
			"active")
}
