// tests/payment/orgInvoiceWebhook_test.go
//
// RED-phase tests for issue #384 (facture-compliance track): invoices are
// user-scoped only. Today Invoice carries UserID / UserSubscriptionID, and the
// Stripe invoice webhook handlers resolve ONLY user subscriptions by customer
// ID (GetActiveSubscriptionByCustomerID). When a Stripe customer maps to an
// OrganizationSubscription and NOT a user subscription, the handlers log
// "no active subscription found" and bail — so an organization's paid invoices
// leave NO local Invoice row, and there is no way to list them.
//
// Target contract:
//   1. Invoice gains nullable OrganizationID + OrganizationSubscriptionID.
//   2. The invoice webhook insert paths create a row for ORG subscriptions too,
//      with the org fields set and Amount canonicalized to Stripe Total.
//   3. User invoices keep their existing shape (org fields NULL).
//
// SSOT note: today "which entity owns this Stripe customer" is resolved through
// ONE path (GetActiveSubscriptionByCustomerID, user subs only). The fix must add
// exactly ONE org resolution path and call it as a consistent fallback across
// all three invoice handlers — not duplicate ad-hoc lookups per handler.
//
// These tests drive the REAL signed webhook end-to-end through the
// HandleStripeWebhook controller (reusing the invoiceAmountCanonical harness:
// newRouterWithRealService / buildSignedWebhookRequest / buildInvoiceAmountsWebhook)
// and read the persisted row back via a local struct so they compile before the
// OrganizationID / OrganizationSubscriptionID columns exist on the model.
package payment_tests

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"soli/formations/src/payment/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// orgScopedInvoiceRow reads back the invoices row through a local struct rather
// than models.Invoice, so these tests compile before the model gains the org
// columns. GORM scans whatever columns exist; OrganizationID /
// OrganizationSubscriptionID stay nil until the columns are added, and the
// row-existence require fails first while the org branch is unimplemented.
type orgScopedInvoiceRow struct {
	StripeInvoiceID            string
	UserID                     string
	Amount                     int64
	Status                     string
	PaidAt                     *time.Time
	OrganizationID             *string
	OrganizationSubscriptionID *string
}

// seedOrgSubForCustomer seeds a plan + an ACTIVE OrganizationSubscription bound
// to customerID, with NO user subscription for that customer. This is the state
// that currently makes the invoice handlers bail: the user lookup misses and
// there is no org fallback. Returns the org and org-subscription IDs so the test
// can assert the persisted row is scoped to them.
func seedOrgSubForCustomer(t *testing.T, db *gorm.DB, customerID string) (orgID uuid.UUID, orgSubID uuid.UUID) {
	t.Helper()
	priceID := "price_orginv_" + uuid.NewString()
	plan := &models.SubscriptionPlan{
		Name: "Org Invoice Plan", PriceAmount: 1200, Currency: "eur",
		BillingInterval: "month", StripePriceID: &priceID, IsActive: true, Priority: 5,
	}
	require.NoError(t, db.Create(plan).Error)

	orgID = uuid.New()
	subStripeID := "sub_orginv_" + uuid.NewString()
	orgSub := &models.OrganizationSubscription{
		OrganizationID:       orgID,
		SubscriptionPlanID:   plan.ID,
		StripeSubscriptionID: &subStripeID,
		StripeCustomerID:     customerID,
		Status:               "active",
		CurrentPeriodStart:   time.Now(),
		CurrentPeriodEnd:     time.Now().Add(30 * 24 * time.Hour),
	}
	require.NoError(t, db.Create(orgSub).Error)
	return orgID, orgSub.ID
}

// TestOrgInvoice_WebhookCreated_PersistsOrgScopedRow drives a signed
// invoice.created for a Stripe customer that maps to an OrganizationSubscription
// (and NO user subscription). Today the handler resolves only user subs, finds
// none, and returns nil — so NO Invoice row is written and the row-existence
// require fails (RED). Once the org fallback lands, an Invoice row must exist
// scoped to the org (OrganizationID + OrganizationSubscriptionID set, user
// fields empty) with Amount canonicalized to Stripe Total.
//
// invoice.created is the cheapest of the three insert paths; finalized and
// payment_succeeded share the same resolution seam and must behave identically.
func TestOrgInvoice_WebhookCreated_PersistsOrgScopedRow(t *testing.T) {
	db := freshTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.Invoice{}))
	secret := "whsec_orginv_" + uuid.NewString()
	router := newRouterWithRealService(t, db, secret)

	customerID := "cus_orginv_" + uuid.NewString()
	orgID, orgSubID := seedOrgSubForCustomer(t, db, customerID)

	stripeInvoiceID := "in_" + uuid.NewString()
	payload := buildInvoiceAmountsWebhook(
		"evt_"+uuid.NewString(), "invoice.created", stripeInvoiceID, customerID, "open",
		invAmountDue, invAmountPaid, invTotal)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, buildSignedWebhookRequest(t, payload, secret))
	require.Equal(t, http.StatusOK, w.Code, "webhook should process; body: %s", w.Body.String())

	var row orgScopedInvoiceRow
	require.NoError(t,
		db.Table("invoices").Where("stripe_invoice_id = ?", stripeInvoiceID).Take(&row).Error,
		"an invoice.created webhook for a Stripe customer that maps to an "+
			"OrganizationSubscription must persist an Invoice row. Today the handler "+
			"resolves only user subscriptions (GetActiveSubscriptionByCustomerID), "+
			"finds none, and bails — so org invoices are silently dropped.")

	require.NotNil(t, row.OrganizationID,
		"the persisted invoice must set OrganizationID for an org-scoped invoice")
	assert.Equal(t, orgID.String(), *row.OrganizationID,
		"OrganizationID must match the OrganizationSubscription's org")

	require.NotNil(t, row.OrganizationSubscriptionID,
		"the persisted invoice must set OrganizationSubscriptionID for an org-scoped invoice")
	assert.Equal(t, orgSubID.String(), *row.OrganizationSubscriptionID,
		"OrganizationSubscriptionID must reference the resolved org subscription")

	assert.Equal(t, invTotal, row.Amount,
		"org invoice Amount must be canonicalized to Stripe Total (%d), like the "+
			"user path (canonicalInvoiceAmount) — Amount is the refund-percentage "+
			"denominator.", invTotal)

	assert.Empty(t, row.UserID,
		"an org-scoped invoice has no individual user; UserID must be empty so the "+
			"row is unambiguously org-owned")
}

// TestOrgInvoice_UserInvoice_Unaffected guards the existing user path: an
// invoice.created for a customer that maps to a USER subscription must still
// write a user-scoped row, with the new org columns NULL. GREEN today (the user
// path already writes the row; the org columns simply do not exist yet, so the
// local struct reads them as nil) and must stay GREEN after the columns are
// added — a user invoice must never be tagged with an org.
func TestOrgInvoice_UserInvoice_Unaffected(t *testing.T) {
	db := freshTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.Invoice{}))
	secret := "whsec_userinv_" + uuid.NewString()
	router := newRouterWithRealService(t, db, secret)

	customerID := "cus_userinv_" + uuid.NewString()
	seedInvoiceAmountSub(t, db, customerID) // active USER subscription, no org sub

	stripeInvoiceID := "in_" + uuid.NewString()
	payload := buildInvoiceAmountsWebhook(
		"evt_"+uuid.NewString(), "invoice.created", stripeInvoiceID, customerID, "open",
		invAmountDue, invAmountPaid, invTotal)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, buildSignedWebhookRequest(t, payload, secret))
	require.Equal(t, http.StatusOK, w.Code, "webhook should process; body: %s", w.Body.String())

	var row orgScopedInvoiceRow
	require.NoError(t,
		db.Table("invoices").Where("stripe_invoice_id = ?", stripeInvoiceID).Take(&row).Error,
		"the user invoice path must keep writing a row")

	assert.Equal(t, "user_invamt", row.UserID,
		"a user invoice must remain scoped to its user")
	assert.Nil(t, row.OrganizationID,
		"a user invoice must not carry an OrganizationID")
	assert.Nil(t, row.OrganizationSubscriptionID,
		"a user invoice must not carry an OrganizationSubscriptionID")
}

// TestOrgInvoice_WebhookReplayed_SingleRow pins the replay safety of
// persistOrgScopedInvoice: the SAME Stripe invoice delivered twice (two distinct
// events carrying the same invoice id — the second bypasses the webhook_events
// idempotency guard) must yield EXACTLY ONE org row, updated in place, never a
// duplicate. GREEN post-implementation (the find-by-StripeInvoiceID → update
// branch exists).
//
// Red-if-gutted: StripeInvoiceID is uniquely indexed, so removing the
// GetInvoiceByStripeID lookup makes the second delivery attempt a fresh
// CreateInvoice that violates the unique index and returns an error → the second
// webhook responds 500, failing the second require(200). Either way the invariant
// "one row, second delivery OK" catches the regression.
func TestOrgInvoice_WebhookReplayed_SingleRow(t *testing.T) {
	db := freshTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.Invoice{}))
	secret := "whsec_orgreplay_" + uuid.NewString()
	router := newRouterWithRealService(t, db, secret)

	customerID := "cus_orgreplay_" + uuid.NewString()
	orgID, orgSubID := seedOrgSubForCustomer(t, db, customerID)

	stripeInvoiceID := "in_" + uuid.NewString()
	deliver := func() *httptest.ResponseRecorder {
		// Distinct event id each time so the webhook_events idempotency guard does
		// NOT short-circuit the second delivery — we want persistOrgScopedInvoice
		// to actually run twice for the same invoice.
		payload := buildInvoiceAmountsWebhook(
			"evt_"+uuid.NewString(), "invoice.created", stripeInvoiceID, customerID, "open",
			invAmountDue, invAmountPaid, invTotal)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, buildSignedWebhookRequest(t, payload, secret))
		return w
	}

	require.Equal(t, http.StatusOK, deliver().Code, "first delivery should process")
	require.Equal(t, http.StatusOK, deliver().Code,
		"a replayed invoice must process cleanly (update in place), not error on the "+
			"unique stripe_invoice_id — the second insert must be avoided by the "+
			"find-by-StripeInvoiceID lookup")

	var count int64
	require.NoError(t,
		db.Table("invoices").Where("stripe_invoice_id = ?", stripeInvoiceID).Count(&count).Error)
	assert.Equal(t, int64(1), count,
		"delivering the same invoice twice must leave EXACTLY one org row, not a duplicate")

	var row orgScopedInvoiceRow
	require.NoError(t,
		db.Table("invoices").Where("stripe_invoice_id = ?", stripeInvoiceID).Take(&row).Error)
	require.NotNil(t, row.OrganizationID)
	assert.Equal(t, orgID.String(), *row.OrganizationID,
		"ownership fields must survive the replay update intact")
	require.NotNil(t, row.OrganizationSubscriptionID)
	assert.Equal(t, orgSubID.String(), *row.OrganizationSubscriptionID)
	assert.Equal(t, invTotal, row.Amount, "Amount must remain canonical (Total) after replay")
}

// TestOrgInvoice_WebhookPaymentSucceeded_PersistsOrgScopedRow pins the second
// org write path: a signed invoice.payment_succeeded for a customer that maps to
// an org subscription (no user sub, no prior row) must persist an org-scoped row
// with org fields set, Amount canonicalized to Total, status "paid", and PaidAt
// populated from the event's status_transitions.paid_at — mirroring what the
// user payment path does. GREEN post-implementation.
func TestOrgInvoice_WebhookPaymentSucceeded_PersistsOrgScopedRow(t *testing.T) {
	db := freshTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.Invoice{}))
	secret := "whsec_orgpaid_" + uuid.NewString()
	router := newRouterWithRealService(t, db, secret)

	customerID := "cus_orgpaid_" + uuid.NewString()
	orgID, orgSubID := seedOrgSubForCustomer(t, db, customerID)

	stripeInvoiceID := "in_" + uuid.NewString()
	payload := buildInvoiceAmountsWebhook(
		"evt_"+uuid.NewString(), "invoice.payment_succeeded", stripeInvoiceID, customerID, "paid",
		invAmountDue, invAmountPaid, invTotal)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, buildSignedWebhookRequest(t, payload, secret))
	require.Equal(t, http.StatusOK, w.Code, "webhook should process; body: %s", w.Body.String())

	var row orgScopedInvoiceRow
	require.NoError(t,
		db.Table("invoices").Where("stripe_invoice_id = ?", stripeInvoiceID).Take(&row).Error,
		"an invoice.payment_succeeded for an org customer must persist an org row")

	require.NotNil(t, row.OrganizationID)
	assert.Equal(t, orgID.String(), *row.OrganizationID)
	require.NotNil(t, row.OrganizationSubscriptionID)
	assert.Equal(t, orgSubID.String(), *row.OrganizationSubscriptionID)
	assert.Equal(t, invTotal, row.Amount, "org invoice Amount must be canonical Total")
	assert.Equal(t, "paid", row.Status, "a payment_succeeded invoice must be marked paid")
	assert.NotNil(t, row.PaidAt,
		"PaidAt must be populated from status_transitions.paid_at, like the user path")
	assert.Empty(t, row.UserID, "an org invoice has no user_id")
}
