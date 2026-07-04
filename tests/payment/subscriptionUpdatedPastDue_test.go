// tests/payment/subscriptionUpdatedPastDue_test.go
//
// RED-phase tests for issue #371 / MR !274 follow-up: the PastDueSince dunning
// stamp lifecycle is maintained in the invoice handlers but NOT in
// handleSubscriptionUpdated (stripeService.go:~979), which writes Status without
// touching the stamp.
//
// Why it matters (the never-instant-lockout invariant): a customer who recovers
// via customer.subscription.updated (Stripe dashboard / API reactivation) keeps
// the OLD stamp. Because handleInvoicePaymentFailed only stamps when
// PastDueSince == nil, a LATER past_due episode then inherits that stale
// >grace stamp and is instantly locked out of new sessions — exactly what the
// grace window is meant to prevent.
//
// These are personal subscriptions (no bulk_purchase / organization_id
// metadata) so handleSubscriptionUpdated routes to the personal path.
package payment_tests

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v85"
)

// buildPersonalSubUpdatedWebhook builds a signed customer.subscription.updated
// event for a PERSONAL subscription (no bulk/org metadata) carrying the given
// status.
func buildPersonalSubUpdatedWebhook(eventID, stripeSubID, status string) []byte {
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
				"customer": {"id": "cus_subupd", "object": "customer"},
				"status": %q,
				"cancel_at_period_end": false,
				"metadata": {},
				"items": {
					"object": "list",
					"data": [{
						"id": "si_subupd",
						"object": "subscription_item",
						"current_period_start": %d,
						"current_period_end": %d,
						"price": {"id": "price_subupd", "object": "price", "currency": "eur", "unit_amount": 1999}
					}]
				}
			}
		}
	}`, eventID, stripe.APIVersion, now, stripeSubID, status, now, end))
}

// 1. Leaving past_due via subscription.updated (status=active) must clear the stamp.
func TestWebhook_SubscriptionUpdated_LeavingPastDue_ClearsPastDueSince(t *testing.T) {
	db := freshTestDB(t)
	secret := "whsec_subupd_" + uuid.NewString()
	router := newRouterWithRealService(t, db, secret)
	plan := seedPastDuePlan(t, db)

	subStripeID := "sub_leave_" + uuid.NewString()
	old := time.Now().Add(-30 * 24 * time.Hour)
	sub := &models.UserSubscription{
		UserID: "user_leave", SubscriptionPlanID: plan.ID, SubscriptionType: "personal",
		StripeSubscriptionID: &subStripeID, Status: "past_due", PastDueSince: &old,
		CurrentPeriodStart: time.Now().Add(-40 * 24 * time.Hour), CurrentPeriodEnd: time.Now().Add(30 * 24 * time.Hour),
	}
	require.NoError(t, db.Create(sub).Error)

	eventID := "evt_leave_" + uuid.NewString()
	w := httptest.NewRecorder()
	router.ServeHTTP(w, buildSignedWebhookRequest(t, buildPersonalSubUpdatedWebhook(eventID, subStripeID, "active"), secret))
	require.Equal(t, http.StatusOK, w.Code, "webhook should process; body: %s", w.Body.String())

	var reloaded models.UserSubscription
	require.NoError(t, db.First(&reloaded, "id = ?", sub.ID).Error)
	assert.Equal(t, "active", reloaded.Status, "subscription.updated must flip status to active")
	assert.Nil(t, reloaded.PastDueSince,
		"LIFECYCLE: leaving past_due via customer.subscription.updated must CLEAR "+
			"PastDueSince — today handleSubscriptionUpdated only writes Status, so the "+
			"stale stamp survives and poisons the next past_due episode.")
}

// 2. Entering past_due via subscription.updated (status=past_due) must stamp.
func TestWebhook_SubscriptionUpdated_EnteringPastDue_StampsPastDueSince(t *testing.T) {
	db := freshTestDB(t)
	secret := "whsec_subupd_" + uuid.NewString()
	router := newRouterWithRealService(t, db, secret)
	plan := seedPastDuePlan(t, db)

	subStripeID := "sub_enter_" + uuid.NewString()
	sub := &models.UserSubscription{
		UserID: "user_enter", SubscriptionPlanID: plan.ID, SubscriptionType: "personal",
		StripeSubscriptionID: &subStripeID, Status: "active", PastDueSince: nil,
		CurrentPeriodStart: time.Now(), CurrentPeriodEnd: time.Now().Add(30 * 24 * time.Hour),
	}
	require.NoError(t, db.Create(sub).Error)

	eventID := "evt_enter_" + uuid.NewString()
	w := httptest.NewRecorder()
	router.ServeHTTP(w, buildSignedWebhookRequest(t, buildPersonalSubUpdatedWebhook(eventID, subStripeID, "past_due"), secret))
	require.Equal(t, http.StatusOK, w.Code, "webhook should process; body: %s", w.Body.String())

	var reloaded models.UserSubscription
	require.NoError(t, db.First(&reloaded, "id = ?", sub.ID).Error)
	assert.Equal(t, "past_due", reloaded.Status, "subscription.updated must flip status to past_due")
	require.NotNil(t, reloaded.PastDueSince,
		"LIFECYCLE: entering past_due via customer.subscription.updated must STAMP "+
			"PastDueSince to start the grace window — today handleSubscriptionUpdated "+
			"leaves it nil, so the gate never engages for this dunning path.")
	assert.WithinDuration(t, time.Now(), *reloaded.PastDueSince, time.Minute,
		"the stamp must be ~now (when the sub entered past_due)")
}

// 3. Invariant end-to-end: a past_due sub with an OLD stamp that recovers via
// subscription.updated and later re-enters past_due (via payment_failed) must
// get a FRESH grace window — never inherit the pre-recovery stale stamp.
func TestPastDueReentry_GetsFreshGrace(t *testing.T) {
	db := freshTestDB(t)
	secret := "whsec_reentry_" + uuid.NewString()
	router := newRouterWithRealService(t, db, secret)
	plan := seedPastDuePlan(t, db)

	subStripeID := "sub_reentry_" + uuid.NewString()
	customerID := "cus_reentry_" + uuid.NewString()
	stale := time.Now().Add(-30 * 24 * time.Hour) // well beyond the 7d grace
	sub := &models.UserSubscription{
		UserID: "user_reentry", SubscriptionPlanID: plan.ID, SubscriptionType: "personal",
		StripeSubscriptionID: &subStripeID, StripeCustomerID: &customerID,
		Status: "past_due", PastDueSince: &stale,
		CurrentPeriodStart: time.Now().Add(-40 * 24 * time.Hour), CurrentPeriodEnd: time.Now().Add(30 * 24 * time.Hour),
	}
	require.NoError(t, db.Create(sub).Error)

	// Step A: recovery via customer.subscription.updated (dashboard reactivation).
	wA := httptest.NewRecorder()
	router.ServeHTTP(wA, buildSignedWebhookRequest(t, buildPersonalSubUpdatedWebhook("evt_reA_"+uuid.NewString(), subStripeID, "active"), secret))
	require.Equal(t, http.StatusOK, wA.Code, "recovery webhook should process; body: %s", wA.Body.String())

	// Step B: a NEW dunning episode via invoice.payment_failed.
	wB := httptest.NewRecorder()
	router.ServeHTTP(wB, buildSignedWebhookRequest(t, buildInvoiceWebhook("evt_reB_"+uuid.NewString(), "invoice.payment_failed", "in_"+uuid.NewString(), customerID), secret))
	require.Equal(t, http.StatusOK, wB.Code, "payment_failed webhook should process; body: %s", wB.Body.String())

	var reloaded models.UserSubscription
	require.NoError(t, db.First(&reloaded, "id = ?", sub.ID).Error)
	require.Equal(t, "past_due", reloaded.Status, "the sub re-entered past_due")
	require.NotNil(t, reloaded.PastDueSince)

	grace := time.Duration(services.PastDueGraceDays()) * 24 * time.Hour
	assert.Less(t, time.Since(*reloaded.PastDueSince), grace,
		"INVARIANT: a re-entered past_due must get a FRESH grace window — the gate "+
			"would allow. Today the recovery (subscription.updated=active) does NOT "+
			"clear the stamp, so payment_failed's `if PastDueSince == nil` guard skips "+
			"re-stamping and the stale %v-old stamp survives, instantly locking out a "+
			"just-recovered customer.", time.Since(stale).Round(time.Hour))
}
