// tests/payment/invoiceAmountCanonical_test.go
//
// RED-phase tests for the 2026-07-10 review finding I1 — Invoice.Amount is an
// SSOT violation: one logical value derived from three different Stripe fields
// depending on the write path in src/payment/services/stripeService.go:
//
//   - handleInvoiceCreated   (invoice.created)           → stripeInvoice.AmountDue
//   - handleInvoiceFinalized (invoice.finalized, insert) → stripeInvoice.AmountDue
//   - handleInvoicePaymentSucceeded (payment_succeeded):
//       * insert branch → stripeInvoice.AmountPaid
//       * update branch → never refreshes Amount at all
//   - processSingleInvoice   (Stripe sync)               → inv.Total  (canonical, already correct)
//
// These diverge when tax/credit adjustments apply. Amount is the denominator of
// the refund-percentage computation (handleCreditNoteCreated / setInvoiceRefundStatus),
// so a wrong Amount silently distorts refund status.
//
// Canonical value (decided): Stripe Total, on EVERY write path. Each test drives
// a REAL webhook end-to-end through the signed HandleStripeWebhook controller
// (reusing the pastDueGrace / refundWebhookSync harness) with an invoice whose
// three amount fields all differ, then reads the persisted Invoice row back and
// asserts Amount == Total.
//
// processSingleInvoice is intentionally NOT covered here: it is unexported and
// only reachable via SyncUserInvoices, which calls the live Stripe invoice.List
// API — untestable without a parallel Stripe-stubbing harness. It already writes
// inv.Total on both branches (green today), so there is nothing to pin at this
// layer without inventing scaffolding.
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
	"gorm.io/gorm"
)

// The three amount fields deliberately all differ: a tax/credit scenario where
// AmountDue (1300) != Total (1200) != AmountPaid (1100). Only Total (1200) is
// the canonical value every write path must persist.
const (
	invAmountDue  int64 = 1300
	invAmountPaid int64 = 1100
	invTotal      int64 = 1200
)

// uuidPtr returns a pointer to u. Invoice.UserSubscriptionID is nullable
// (*uuid.UUID) — org rows leave it NULL — so user-invoice fixtures must pass a
// pointer.
func uuidPtr(u uuid.UUID) *uuid.UUID { return &u }

// buildInvoiceAmountsWebhook builds a signed invoice.* event whose amount_due,
// amount_paid and total are all distinct, so we can tell which field a handler
// persisted into Invoice.Amount. status mirrors the real event (e.g. "open" for
// created/finalized, "paid" for payment_succeeded) since the handlers copy it
// verbatim into Invoice.Status.
func buildInvoiceAmountsWebhook(eventID, eventType, invoiceID, customerID, status string, amountDue, amountPaid, total int64) []byte {
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
				"status": %q,
				"amount_due": %d,
				"amount_paid": %d,
				"total": %d,
				"currency": "eur",
				"number": "INV-AMOUNT",
				"created": %d,
				"status_transitions": {"paid_at": %d}
			}
		}
	}`, eventID, stripe.APIVersion, eventType, now, invoiceID, customerID, status, amountDue, amountPaid, total, now, now))
}

// seedInvoiceAmountSub seeds a plan + an ACTIVE personal subscription bound to
// customerID, satisfying both GetActiveSubscriptionByCustomerID (created /
// finalized) and GetRecoverableSubscriptionByCustomerID (payment_succeeded).
func seedInvoiceAmountSub(t *testing.T, db *gorm.DB, customerID string) {
	t.Helper()
	priceID := "price_invamt_" + uuid.NewString()
	plan := &models.SubscriptionPlan{
		Name: "Invoice Amount Plan", PriceAmount: 1200, Currency: "eur",
		BillingInterval: "month", StripePriceID: &priceID, IsActive: true, Priority: 5,
	}
	require.NoError(t, db.Create(plan).Error)

	subStripeID := "sub_invamt_" + uuid.NewString()
	sub := &models.UserSubscription{
		UserID: "user_invamt", SubscriptionPlanID: plan.ID, SubscriptionType: "personal",
		StripeSubscriptionID: &subStripeID, StripeCustomerID: &customerID, Status: "active",
		CurrentPeriodStart: time.Now(), CurrentPeriodEnd: time.Now().Add(30 * 24 * time.Hour),
	}
	require.NoError(t, db.Create(sub).Error)
}

// TestInvoiceAmount_InsertPaths_PersistTotal drives the three insert write paths
// (invoice.created, invoice.finalized, invoice.payment_succeeded) that create a
// brand-new Invoice row, and asserts each persists Amount == Total.
//
//   - created / finalized persist AmountDue (1300) today → RED.
//   - payment_succeeded (insert) persists AmountPaid (1100) today → RED.
func TestInvoiceAmount_InsertPaths_PersistTotal(t *testing.T) {
	cases := []struct {
		name       string
		eventType  string
		status     string // the invoice status the real event carries
		testName   string // maps to the requested per-path RED test name
		wrongToday int64  // the value the path persists today (documents the bug)
	}{
		{"created", "invoice.created", "open", "TestInvoiceAmount_CreatedWebhook_PersistsTotal", invAmountDue},
		{"finalized", "invoice.finalized", "open", "TestInvoiceAmount_FinalizedWebhook_PersistsTotal", invAmountDue},
		{"payment_succeeded", "invoice.payment_succeeded", "paid", "TestInvoiceAmount_PaymentSucceededInsert_PersistsTotal", invAmountPaid},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := freshTestDB(t)
			require.NoError(t, db.AutoMigrate(&models.Invoice{}))
			secret := "whsec_invamt_" + uuid.NewString()
			router := newRouterWithRealService(t, db, secret)

			customerID := "cus_invamt_" + uuid.NewString()
			seedInvoiceAmountSub(t, db, customerID)

			stripeInvoiceID := "in_" + uuid.NewString()
			payload := buildInvoiceAmountsWebhook(
				"evt_"+uuid.NewString(), tc.eventType, stripeInvoiceID, customerID, tc.status,
				invAmountDue, invAmountPaid, invTotal)

			w := httptest.NewRecorder()
			router.ServeHTTP(w, buildSignedWebhookRequest(t, payload, secret))
			require.Equal(t, http.StatusOK, w.Code, "webhook should process; body: %s", w.Body.String())

			var reloaded models.Invoice
			require.NoError(t, db.First(&reloaded, "stripe_invoice_id = ?", stripeInvoiceID).Error,
				"%s must persist an Invoice row for %s", tc.testName, tc.eventType)
			assert.Equal(t, invTotal, reloaded.Amount,
				"SSOT: %s must persist Invoice.Amount = Stripe Total (%d), not %d. The %s path "+
					"currently writes %d; Amount is the refund-percentage denominator so every write "+
					"path must derive it from Total.",
				tc.testName, invTotal, reloaded.Amount, tc.eventType, tc.wrongToday)
		})
	}
}

// TestInvoiceAmount_PaymentSucceededUpdate_RefreshesTotal drives
// invoice.payment_succeeded against an ALREADY-EXISTING Invoice row seeded with a
// stale Amount. The handler's update branch (existing invoice) refreshes Status /
// PaidAt / DownloadURL but never touches Amount, so a stale/wrong Amount survives.
// RED today (Amount stays 999); GREEN once the update branch refreshes it to Total.
func TestInvoiceAmount_PaymentSucceededUpdate_RefreshesTotal(t *testing.T) {
	db := freshTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.Invoice{}))
	secret := "whsec_invamt_" + uuid.NewString()
	router := newRouterWithRealService(t, db, secret)

	customerID := "cus_invamt_" + uuid.NewString()
	seedInvoiceAmountSub(t, db, customerID)

	// Seed an existing invoice row with a STALE amount (e.g. a draft written from
	// AmountDue earlier). Its UserSubscriptionID need not match the resolved sub;
	// the handler looks the row up by Stripe invoice id.
	const staleAmount int64 = 999
	stripeInvoiceID := "in_" + uuid.NewString()
	require.NoError(t, db.Create(&models.Invoice{
		UserID: "user_invamt", UserSubscriptionID: uuidPtr(uuid.New()),
		StripeInvoiceID: stripeInvoiceID, Amount: staleAmount, Currency: "eur", Status: "open",
		InvoiceNumber: "INV-STALE", InvoiceDate: time.Now(),
	}).Error)

	payload := buildInvoiceAmountsWebhook(
		"evt_"+uuid.NewString(), "invoice.payment_succeeded", stripeInvoiceID, customerID, "paid",
		invAmountDue, invAmountPaid, invTotal)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, buildSignedWebhookRequest(t, payload, secret))
	require.Equal(t, http.StatusOK, w.Code, "webhook should process; body: %s", w.Body.String())

	var reloaded models.Invoice
	require.NoError(t, db.First(&reloaded, "stripe_invoice_id = ?", stripeInvoiceID).Error)
	assert.Equal(t, "paid", reloaded.Status,
		"sanity: the update branch ran and moved the invoice to paid")
	assert.Equal(t, invTotal, reloaded.Amount,
		"SSOT: the payment_succeeded UPDATE branch must REFRESH Invoice.Amount to Stripe Total "+
			"(%d). Today it updates Status/PaidAt/DownloadURL but never Amount, so the stale %d "+
			"survives and distorts refund-percentage math.", invTotal, staleAmount)
}

// buildPaidInvoiceWebhook builds a signed invoice.payment_succeeded event with
// explicit number / currency / hosted_invoice_url so a test can prove the update
// branch backfills each field. Amounts are fixed to the canonical trio.
func buildPaidInvoiceWebhook(eventID, invoiceID, customerID, number, currency, hostedURL string) []byte {
	now := time.Now().Unix()
	return []byte(fmt.Sprintf(`{
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
				"amount_due": %d,
				"amount_paid": %d,
				"total": %d,
				"currency": %q,
				"number": %q,
				"hosted_invoice_url": %q,
				"created": %d,
				"status_transitions": {"paid_at": %d}
			}
		}
	}`, eventID, stripe.APIVersion, now, invoiceID, customerID,
		invAmountDue, invAmountPaid, invTotal, currency, number, hostedURL, now, now))
}

// TestInvoiceAmount_PaymentSucceededUpdate_BackfillsInvoiceFields covers the
// broader partial-refresh bug in the same update branch: a row inserted at
// invoice.created has no InvoiceNumber (Stripe assigns it at finalization) and no
// hosted URL. When payment_succeeded later hits the UPDATE branch it refreshes
// Status/PaidAt/DownloadURL but silently drops InvoiceNumber, StripeHostedURL and
// Currency, so those never get backfilled — StripeHostedURL in particular feeds
// the invoice-download fallback. RED today; GREEN once the update branch copies
// all three from the incoming event.
func TestInvoiceAmount_PaymentSucceededUpdate_BackfillsInvoiceFields(t *testing.T) {
	db := freshTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.Invoice{}))
	secret := "whsec_invamt_" + uuid.NewString()
	router := newRouterWithRealService(t, db, secret)

	customerID := "cus_invamt_" + uuid.NewString()
	seedInvoiceAmountSub(t, db, customerID)

	// Seed a row as it looks straight after invoice.created: no number yet, empty
	// hosted URL, and a placeholder currency — all of which finalization/payment
	// should backfill.
	stripeInvoiceID := "in_" + uuid.NewString()
	require.NoError(t, db.Create(&models.Invoice{
		UserID: "user_invamt", UserSubscriptionID: uuidPtr(uuid.New()),
		StripeInvoiceID: stripeInvoiceID, Amount: 999, Currency: "usd", Status: "open",
		InvoiceNumber: "", StripeHostedURL: "", InvoiceDate: time.Now(),
	}).Error)

	const (
		freshNumber    = "INV-2026-0042"
		freshCurrency  = "eur"
		freshHostedURL = "https://invoice.stripe.com/i/fresh"
	)
	payload := buildPaidInvoiceWebhook(
		"evt_"+uuid.NewString(), stripeInvoiceID, customerID, freshNumber, freshCurrency, freshHostedURL)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, buildSignedWebhookRequest(t, payload, secret))
	require.Equal(t, http.StatusOK, w.Code, "webhook should process; body: %s", w.Body.String())

	var reloaded models.Invoice
	require.NoError(t, db.First(&reloaded, "stripe_invoice_id = ?", stripeInvoiceID).Error)
	require.Equal(t, "paid", reloaded.Status, "sanity: the update branch ran")
	assert.Equal(t, freshNumber, reloaded.InvoiceNumber,
		"REFRESH: a row inserted at invoice.created has no number until finalization; the "+
			"payment_succeeded UPDATE branch must backfill InvoiceNumber from the event.")
	assert.Equal(t, freshHostedURL, reloaded.StripeHostedURL,
		"REFRESH: the UPDATE branch must backfill StripeHostedURL — it feeds the invoice-download "+
			"fallback, so a stale/empty value breaks the download link.")
	assert.Equal(t, freshCurrency, reloaded.Currency,
		"REFRESH: the UPDATE branch must backfill Currency from the event, not leave the "+
			"placeholder written at insert time.")
}
