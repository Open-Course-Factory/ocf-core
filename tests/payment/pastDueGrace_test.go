// tests/payment/pastDueGrace_test.go
//
// RED-phase tests for issue #371 / MR !274 — past_due dunning grace policy.
//
// Design (approved): a past_due subscription keeps full access for a grace
// period (default 7 days, env PAYMENT_PAST_DUE_GRACE_DAYS); beyond grace, NEW
// session-creation paths are rejected (402 subscription_past_due) while content
// and existing sessions stay untouched; recovery to active (#364) restores
// access immediately.
//
// This file covers the two cleanly-seamed halves:
//   - webhook: invoice.payment_failed stamps PastDueSince; the #364 recovery on
//     invoice.payment_succeeded clears it.
//   - resolution: a past_due subscription (any age) STILL resolves an effective
//     plan — today it does not, because both resolution paths filter
//     status IN ('active','trialing'). This pins the grace/content half AND the
//     filter-consistency fix (paymentRepository GetActiveUserSubscription /
//     effectivePlanService.resolveForOrg vs GetUserSubscriptions which already
//     includes past_due).
//
// PastDueSince does not exist on the model yet, so it is read via raw SQL (like
// fetchWebhookEventStatus) to keep the package compiling: the column-missing
// read is the red signal.
package payment_tests

import (
	"database/sql"
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
	"gorm.io/gorm"
)

// pastDueSinceState reads the (not-yet-existing) past_due_since column via raw
// SQL. Returns present=false when the column/row is absent (the red signal),
// else valid = whether the timestamp is non-null.
func pastDueSinceState(t *testing.T, db *gorm.DB, subID uuid.UUID) (present, valid bool) {
	t.Helper()
	var nt sql.NullTime
	row := db.Raw("SELECT past_due_since FROM user_subscriptions WHERE id = ?", subID).Row()
	if err := row.Scan(&nt); err != nil {
		return false, false
	}
	return true, nt.Valid
}

// buildInvoiceWebhook builds a signed invoice.* event for the given customer +
// invoice id.
func buildInvoiceWebhook(eventID, eventType, invoiceID, customerID string) []byte {
	now := time.Now().Unix()
	return []byte(fmt.Sprintf(`{
		"id": %q,
		"object": "event",
		"api_version": %q,
		"type": %q,
		"created": %d,
		"data": {
			"object": {
				"id": %q,
				"object": "invoice",
				"customer": {"id": %q, "object": "customer"},
				"status": "open",
				"amount_paid": 1999,
				"currency": "eur",
				"number": "INV-PASTDUE",
				"created": %d,
				"status_transitions": {"paid_at": %d}
			}
		}
	}`, eventID, stripe.APIVersion, eventType, now, invoiceID, customerID, now, now))
}

func seedPastDuePlan(t *testing.T, db *gorm.DB) *models.SubscriptionPlan {
	t.Helper()
	priceID := "price_pastdue_" + uuid.NewString()
	plan := &models.SubscriptionPlan{
		Name: "Past Due Plan", PriceAmount: 1999, Currency: "eur",
		BillingInterval: "month", StripePriceID: &priceID, IsActive: true, Priority: 5,
	}
	require.NoError(t, db.Create(plan).Error)
	return plan
}

// 1. invoice.payment_failed must set status=past_due AND stamp PastDueSince.
func TestWebhook_InvoicePaymentFailed_StampsPastDueSince(t *testing.T) {
	db := freshTestDB(t)
	secret := "whsec_pastdue_" + uuid.NewString()
	router := newRouterWithRealService(t, db, secret)
	plan := seedPastDuePlan(t, db)

	subStripeID := "sub_pd_" + uuid.NewString()
	customerID := "cus_pd_" + uuid.NewString()
	sub := &models.UserSubscription{
		UserID: "user_pd", SubscriptionPlanID: plan.ID, SubscriptionType: "personal",
		StripeSubscriptionID: &subStripeID, StripeCustomerID: &customerID, Status: "active",
		CurrentPeriodStart: time.Now(), CurrentPeriodEnd: time.Now().Add(30 * 24 * time.Hour),
	}
	require.NoError(t, db.Create(sub).Error)

	eventID := "evt_pf_" + uuid.NewString()
	payload := buildInvoiceWebhook(eventID, "invoice.payment_failed", "in_"+uuid.NewString(), customerID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, buildSignedWebhookRequest(t, payload, secret))
	require.Equal(t, http.StatusOK, w.Code, "webhook should process; body: %s", w.Body.String())

	// Status transition already works today.
	var reloaded models.UserSubscription
	require.NoError(t, db.First(&reloaded, "id = ?", sub.ID).Error)
	assert.Equal(t, "past_due", reloaded.Status, "payment_failed must mark the sub past_due")

	// PastDueSince stamping is the new behavior.
	present, valid := pastDueSinceState(t, db, sub.ID)
	require.True(t, present,
		"DUNNING: user_subscriptions needs a PastDueSince column — add PastDueSince "+
			"*time.Time to UserSubscription and stamp it in handleInvoicePaymentFailed.")
	assert.True(t, valid,
		"handleInvoicePaymentFailed must stamp PastDueSince with the time the sub went past_due")
}

// 2. Recovery (invoice.payment_succeeded on a past_due sub, #364) must restore
// active AND clear PastDueSince.
func TestWebhook_InvoicePaymentSucceeded_Recovery_ClearsPastDueSince(t *testing.T) {
	db := freshTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.Invoice{})) // recovery records an invoice
	secret := "whsec_pastdue_" + uuid.NewString()
	router := newRouterWithRealService(t, db, secret)
	plan := seedPastDuePlan(t, db)

	subStripeID := "sub_rec_" + uuid.NewString()
	customerID := "cus_rec_" + uuid.NewString()
	sub := &models.UserSubscription{
		UserID: "user_rec", SubscriptionPlanID: plan.ID, SubscriptionType: "personal",
		StripeSubscriptionID: &subStripeID, StripeCustomerID: &customerID, Status: "past_due",
		CurrentPeriodStart: time.Now().Add(-40 * 24 * time.Hour), CurrentPeriodEnd: time.Now().Add(-10 * 24 * time.Hour),
	}
	require.NoError(t, db.Create(sub).Error)
	// Best-effort pre-set PastDueSince (no-op pre-fix when the column is absent).
	_ = db.Exec("UPDATE user_subscriptions SET past_due_since = ? WHERE id = ?", time.Now().Add(-3*24*time.Hour), sub.ID).Error

	eventID := "evt_ps_" + uuid.NewString()
	payload := buildInvoiceWebhook(eventID, "invoice.payment_succeeded", "in_"+uuid.NewString(), customerID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, buildSignedWebhookRequest(t, payload, secret))
	require.Equal(t, http.StatusOK, w.Code, "recovery webhook should process; body: %s", w.Body.String())

	var reloaded models.UserSubscription
	require.NoError(t, db.First(&reloaded, "id = ?", sub.ID).Error)
	assert.Equal(t, "active", reloaded.Status, "recovery must restore the sub to active (#364)")

	present, valid := pastDueSinceState(t, db, sub.ID)
	require.True(t, present,
		"DUNNING: PastDueSince column must exist (add it to UserSubscription).")
	assert.False(t, valid,
		"recovery to active must CLEAR PastDueSince (set it back to NULL) so a later "+
			"past_due starts a fresh grace window")
}

// 3. A past_due subscription (any age) must STILL resolve an effective plan, so
// content and within-grace sessions keep working. Today both resolution paths
// filter active/trialing, so past_due resolves nothing.
func TestGetUserEffectivePlan_PastDueSubscription_StillResolves(t *testing.T) {
	db := freshTestDB(t)
	userID := "user-pastdue-resolves"
	plan := seedPastDuePlan(t, db)

	// A past_due personal subscription (period already ended — "any age").
	require.NoError(t, db.Create(&models.UserSubscription{
		UserID: userID, SubscriptionPlanID: plan.ID, SubscriptionType: "personal",
		Status: "past_due",
		CurrentPeriodStart: time.Now().Add(-40 * 24 * time.Hour), CurrentPeriodEnd: time.Now().Add(-10 * 24 * time.Hour),
	}).Error)

	svc := services.NewEffectivePlanService(db)
	result, err := svc.GetUserEffectivePlan(userID, nil)

	require.NoError(t, err,
		"GRACE: a past_due subscription must still resolve an effective plan (access "+
			"is only gated at session-creation beyond grace, not at plan resolution). "+
			"Today GetActiveUserSubscription / resolveForOrg filter status IN "+
			"('active','trialing'), so past_due resolves nothing and RequirePlan 403s "+
			"the user out of content too. Fix: include 'past_due' in both filters "+
			"(consistent with GetUserSubscriptions).")
	require.NotNil(t, result)
	assert.Equal(t, plan.ID, result.Plan.ID, "the resolved plan must be the past_due sub's plan")
	require.NotNil(t, result.UserSubscription)
	assert.Equal(t, "past_due", result.UserSubscription.Status)
}

// 4. Config: PastDueGraceDays defaults to 7 and honors PAYMENT_PAST_DUE_GRACE_DAYS.
func TestPastDueGraceDays_DefaultAndEnvOverride(t *testing.T) {
	t.Setenv("PAYMENT_PAST_DUE_GRACE_DAYS", "") // unset-equivalent
	assert.Equal(t, 7, services.PastDueGraceDays(), "default grace window is 7 days")

	t.Setenv("PAYMENT_PAST_DUE_GRACE_DAYS", "3")
	assert.Equal(t, 3, services.PastDueGraceDays(), "env override must be honored")

	t.Setenv("PAYMENT_PAST_DUE_GRACE_DAYS", "notanumber")
	assert.Equal(t, 7, services.PastDueGraceDays(), "an invalid value falls back to the default")

	t.Setenv("PAYMENT_PAST_DUE_GRACE_DAYS", "0")
	assert.Equal(t, 0, services.PastDueGraceDays(), "0 (gate immediately) is a valid override")
}
