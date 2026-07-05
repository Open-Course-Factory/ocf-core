// tests/payment/refundWebhookSync_test.go
//
// RED-phase tests for issue #375 / MR !277 — reconcile local Invoice records
// from refund / credit-note webhooks.
//
// Investigation (current main):
//   - models.Invoice has Status (paid/open/void/uncollectible) and Amount
//     (int64 cents). It has NO amount-refunded field and no refunded statuses.
//     Design adds `AmountRefunded int64` (cents, mirroring Amount) plus statuses
//     'refunded' / 'partially_refunded'.
//   - ProcessWebhook's switch does NOT handle `charge.refunded` or
//     `credit_note.created`; they fall through to `default:` which logs and
//     returns nil → the controller marks the event processed and returns 200,
//     so today a refund silently leaves the local invoice untouched (status stays
//     'paid').
//   - A Stripe Charge links to its invoice via charge.invoice; a CreditNote via
//     credit_note.invoice. Local lookup is repository.GetInvoiceByStripeID.
//
// AmountRefunded does not exist yet, so it is read via raw SQL (like PastDueSince
// / webhook_events.status) to keep the package compiling; the column-missing
// read is the red signal. Status is asserted via the existing model field.
package payment_tests

import (
	"database/sql"
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

func seedRefundInvoice(t *testing.T, db *gorm.DB, stripeInvoiceID string, amount int64, status string) uuid.UUID {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&models.Invoice{}))
	inv := &models.Invoice{
		UserID: "user_refund", UserSubscriptionID: uuid.New(),
		StripeInvoiceID: stripeInvoiceID, Amount: amount, Currency: "eur", Status: status,
		InvoiceNumber: "INV-REFUND", InvoiceDate: time.Now(),
	}
	require.NoError(t, db.Create(inv).Error)
	return inv.ID
}

// invoiceAmountRefunded reads the (not-yet-existing) amount_refunded column via
// raw SQL. present=false when the column is absent (the red signal).
func invoiceAmountRefunded(t *testing.T, db *gorm.DB, invoiceID uuid.UUID) (val int64, present bool) {
	t.Helper()
	var n sql.NullInt64
	row := db.Raw("SELECT amount_refunded FROM invoices WHERE id = ?", invoiceID).Row()
	if err := row.Scan(&n); err != nil {
		return 0, false
	}
	return n.Int64, true
}

func buildChargeRefundedWebhook(eventID, stripeInvoiceID string, amount, amountRefunded int64, fullyRefunded bool) []byte {
	now := time.Now().Unix()
	return []byte(fmt.Sprintf(`{
		"id": %q,
		"object": "event",
		"api_version": %q,
		"type": "charge.refunded",
		"created": %d,
		"data": {
			"object": {
				"id": "ch_%s",
				"object": "charge",
				"invoice": %q,
				"amount": %d,
				"amount_refunded": %d,
				"refunded": %t,
				"currency": "eur",
				"status": "succeeded"
			}
		}
	}`, eventID, stripe.APIVersion, now, uuid.NewString(), stripeInvoiceID, amount, amountRefunded, fullyRefunded))
}

func buildCreditNoteCreatedWebhook(eventID, stripeInvoiceID string, total int64) []byte {
	now := time.Now().Unix()
	return []byte(fmt.Sprintf(`{
		"id": %q,
		"object": "event",
		"api_version": %q,
		"type": "credit_note.created",
		"created": %d,
		"data": {
			"object": {
				"id": "cn_%s",
				"object": "credit_note",
				"invoice": %q,
				"total": %d,
				"amount": %d,
				"currency": "eur",
				"status": "issued"
			}
		}
	}`, eventID, stripe.APIVersion, now, uuid.NewString(), stripeInvoiceID, total, total))
}

// 1. Full refund via charge.refunded → invoice 'refunded', AmountRefunded=Amount.
func TestWebhook_ChargeRefunded_FullRefund_MarksInvoiceRefunded(t *testing.T) {
	db := freshTestDB(t)
	secret := "whsec_refund_" + uuid.NewString()
	router := newRouterWithRealService(t, db, secret)

	const amount int64 = 1999
	stripeInvoiceID := "in_" + uuid.NewString()
	invID := seedRefundInvoice(t, db, stripeInvoiceID, amount, "paid")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, buildSignedWebhookRequest(t, buildChargeRefundedWebhook("evt_"+uuid.NewString(), stripeInvoiceID, amount, amount, true), secret))
	require.Equal(t, http.StatusOK, w.Code, "webhook should process; body: %s", w.Body.String())

	var reloaded models.Invoice
	require.NoError(t, db.First(&reloaded, "id = ?", invID).Error)
	assert.Equal(t, "refunded", reloaded.Status,
		"REFUND: a full charge.refunded must mark the local invoice 'refunded'. Today "+
			"charge.refunded is unhandled (falls through to default → 200) so the invoice "+
			"stays 'paid'.")

	refunded, present := invoiceAmountRefunded(t, db, invID)
	require.True(t, present,
		"REFUND: invoices needs an AmountRefunded column — add `AmountRefunded int64` "+
			"(cents) to models.Invoice.")
	assert.Equal(t, amount, refunded, "AmountRefunded must equal the refunded amount")
}

// 2. Partial refund → 'partially_refunded', AmountRefunded tracks the partial amount.
func TestWebhook_ChargeRefunded_PartialRefund_TracksAmount(t *testing.T) {
	db := freshTestDB(t)
	secret := "whsec_refund_" + uuid.NewString()
	router := newRouterWithRealService(t, db, secret)

	const amount int64 = 2000
	const partial int64 = 800
	stripeInvoiceID := "in_" + uuid.NewString()
	invID := seedRefundInvoice(t, db, stripeInvoiceID, amount, "paid")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, buildSignedWebhookRequest(t, buildChargeRefundedWebhook("evt_"+uuid.NewString(), stripeInvoiceID, amount, partial, false), secret))
	require.Equal(t, http.StatusOK, w.Code, "webhook should process; body: %s", w.Body.String())

	var reloaded models.Invoice
	require.NoError(t, db.First(&reloaded, "id = ?", invID).Error)
	assert.Equal(t, "partially_refunded", reloaded.Status,
		"REFUND: a partial charge.refunded (amount_refunded < amount, refunded=false) must "+
			"mark the invoice 'partially_refunded'.")

	refunded, present := invoiceAmountRefunded(t, db, invID)
	require.True(t, present, "AmountRefunded column must exist")
	assert.Equal(t, partial, refunded, "AmountRefunded must track the partial amount")
}

// 3. credit_note.created reconciles the invoice. Semantics choice (documented for
// the dev): treat the credit note's total as a refund of that amount — full
// ('refunded') when total >= invoice.Amount, else 'partially_refunded' — and set
// AmountRefunded to the credited total. Here total == Amount → full.
func TestWebhook_CreditNoteCreated_ReconcilesInvoice(t *testing.T) {
	db := freshTestDB(t)
	secret := "whsec_refund_" + uuid.NewString()
	router := newRouterWithRealService(t, db, secret)

	const amount int64 = 1500
	stripeInvoiceID := "in_" + uuid.NewString()
	invID := seedRefundInvoice(t, db, stripeInvoiceID, amount, "paid")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, buildSignedWebhookRequest(t, buildCreditNoteCreatedWebhook("evt_"+uuid.NewString(), stripeInvoiceID, amount), secret))
	require.Equal(t, http.StatusOK, w.Code, "webhook should process; body: %s", w.Body.String())

	var reloaded models.Invoice
	require.NoError(t, db.First(&reloaded, "id = ?", invID).Error)
	assert.Equal(t, "refunded", reloaded.Status,
		"REFUND: a credit_note.created covering the full invoice must reconcile it to "+
			"'refunded'. Today credit_note.created is unhandled.")

	refunded, present := invoiceAmountRefunded(t, db, invID)
	require.True(t, present, "AmountRefunded column must exist")
	assert.Equal(t, amount, refunded, "AmountRefunded must equal the credited total")
}

// 4. Unknown/absent invoice → graceful 200, no rows created, event NOT 'failed'
// (no retry storm). GUARD — green today (unhandled events already 200) and must
// stay green once handled (invoice-missing is a no-op, not an error).
func TestWebhook_ChargeRefunded_UnknownInvoice_SucceedsGracefully(t *testing.T) {
	db := freshTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.Invoice{}))
	secret := "whsec_refund_" + uuid.NewString()
	router := newRouterWithRealService(t, db, secret)

	eventID := "evt_" + uuid.NewString()
	unknownInvoiceID := "in_does_not_exist_" + uuid.NewString()
	w := httptest.NewRecorder()
	router.ServeHTTP(w, buildSignedWebhookRequest(t, buildChargeRefundedWebhook(eventID, unknownInvoiceID, 1999, 1999, true), secret))

	assert.Equal(t, http.StatusOK, w.Code,
		"a refund for an unknown invoice must NOT 5xx (no Stripe retry storm). Body: %s", w.Body.String())
	assert.NotEqual(t, "failed", fetchWebhookEventStatus(t, db, eventID),
		"the event must not be marked failed for a missing invoice")

	// Scoped to the unknown id (freshTestDB does not clean the invoices table, so
	// a global count would pick up other tests' rows). No row must be created FOR
	// the missing invoice.
	var count int64
	require.NoError(t, db.Model(&models.Invoice{}).Where("stripe_invoice_id = ?", unknownInvoiceID).Count(&count).Error)
	assert.Equal(t, int64(0), count, "no invoice row may be created for an unknown invoice")
}

// 5. Idempotency: the same charge.refunded delivered twice (fresh signatures,
// same event id) must reconcile EXACTLY ONCE — the second delivery is deduped by
// the existing event-reservation pipeline, so AmountRefunded stays = Amount (not
// doubled). RED today (unhandled → no reconciliation at all); becomes green once
// handled, with the pipeline guaranteeing single-apply.
func TestWebhook_ChargeRefunded_DuplicateDelivery_AppliesOnce(t *testing.T) {
	db := freshTestDB(t)
	secret := "whsec_refund_" + uuid.NewString()
	router := newRouterWithRealService(t, db, secret)

	const amount int64 = 1999
	stripeInvoiceID := "in_" + uuid.NewString()
	invID := seedRefundInvoice(t, db, stripeInvoiceID, amount, "paid")

	eventID := "evt_" + uuid.NewString() // SAME event id for both deliveries
	payload := buildChargeRefundedWebhook(eventID, stripeInvoiceID, amount, amount, true)

	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, buildSignedWebhookRequest(t, payload, secret))
	require.Equal(t, http.StatusOK, w1.Code)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, buildSignedWebhookRequest(t, payload, secret))
	require.Equal(t, http.StatusOK, w2.Code)

	var reloaded models.Invoice
	require.NoError(t, db.First(&reloaded, "id = ?", invID).Error)
	assert.Equal(t, "refunded", reloaded.Status, "invoice reconciled once")

	refunded, present := invoiceAmountRefunded(t, db, invID)
	require.True(t, present, "AmountRefunded column must exist")
	assert.Equal(t, amount, refunded,
		"IDEMPOTENCY: two deliveries of the same charge.refunded event must apply the "+
			"refund exactly once (deduped by the event pipeline) — AmountRefunded must be "+
			"Amount, not 2×Amount.")
}
