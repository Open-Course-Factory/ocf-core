// tests/payment/rgpdCustomerErasure_test.go
//
// RED-phase tests for issue #370 / MR !272: RGPD (GDPR Art. 17) erasure of the
// Stripe Customer object on account deletion.
//
// Premise confirmed on current main: PaymentDeletionHelper cancels active Stripe
// subscriptions and pseudonymizes local billing PII, but NEVER deletes the
// Stripe Customer — grep for customer.Del / DeleteCustomer / DELETE customers in
// src/payment/ returns nothing. The user's identifying data therefore survives
// on Stripe indefinitely.
//
// Desired: after cancellations succeed, the helper deletes every distinct Stripe
// Customer id associated with the user, with:
//   (a) FAIL-CLOSED — a hard Stripe error aborts the deletion (mirrors the
//       existing CancelAllActiveSubscriptionsForUser stop-on-error contract so
//       the caller does not drop the Casdoor account while Stripe still holds
//       the customer),
//   (b) IDEMPOTENT — an already-deleted / missing customer (Stripe 404
//       resource_missing) counts as success, not abort.
//
// Where customer ids live (investigation): user_subscriptions.stripe_customer_id
// (*string) is the user-scoped source. organization_subscriptions.stripe_customer_id
// belongs to the ORG, not the user, and is out of scope here. payment_methods
// carry no customer id. NOTE for the implementer: assigned bulk-license rows
// carry the PURCHASER's stripe_customer_id — erasing that when an *assignee* is
// deleted would wrongly delete the purchaser's Customer. Scope erasure to
// customers the user OWNS (self-purchased), not licenses assigned to them.
//
// PROPOSED API pinned here (undefined today): the helper gains
//
//	DeleteStripeCustomersForUser(userID string) error   // fail-closed
//
// It is pinned via a runtime interface assertion (not a direct method call) so
// this file keeps COMPILING while the method is absent — the tests fail at the
// assertion with a clear "not implemented yet" message, and the rest of the
// payment suite is unaffected. Once the method exists the DELETE/idempotency/
// fail-closed assertions run.
package payment_tests

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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

// stripeCustomerEraser is the proposed erasure method, asserted at runtime so
// the file compiles before the dev adds it to the helper.
type stripeCustomerEraser interface {
	DeleteStripeCustomersForUser(userID string) error
}

func asCustomerEraser(t *testing.T, h services.PaymentDeletionHelper) stripeCustomerEraser {
	t.Helper()
	e, ok := h.(stripeCustomerEraser)
	require.True(t, ok,
		"RGPD: PaymentDeletionHelper must implement DeleteStripeCustomersForUser(userID string) error "+
			"to erase the Stripe Customer on account deletion — not implemented yet.")
	return e
}

// customerDeleteCapture records DELETE /v1/customers/{id} requests the real
// stripe-go client sends to the fake backend, and can force a 404 (missing) or
// 500 (hard error) response.
type customerDeleteCapture struct {
	mu      sync.Mutex
	deleted []string
	mode    string // "" = deleted ok, "missing" = 404, "error" = 500
}

func (c *customerDeleteCapture) record(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.deleted = append(c.deleted, id)
}

func (c *customerDeleteCapture) distinctDeleted() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	seen := map[string]bool{}
	var out []string
	for _, id := range c.deleted {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// installCustomerDeleteBackend points the global stripe backend at a fake server
// that captures customer DELETEs. Globals restored on cleanup.
func installCustomerDeleteBackend(t *testing.T, mode string) *customerDeleteCapture {
	t.Helper()
	cap := &customerDeleteCapture{mode: mode}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/v1/customers/") {
			id := strings.TrimPrefix(r.URL.Path, "/v1/customers/")
			cap.record(id)
			switch cap.mode {
			case "missing":
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"error":{"type":"invalid_request_error","code":"resource_missing","message":"No such customer"}}`))
			case "error":
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":{"type":"api_error","message":"Stripe is down"}}`))
			default:
				_, _ = w.Write([]byte(`{"id":"` + id + `","object":"customer","deleted":true}`))
			}
			return
		}
		// Anything else: benign OK.
		_, _ = w.Write([]byte(`{}`))
	}))

	prevBackend := stripe.GetBackend(stripe.APIBackend)
	prevKey := stripe.Key
	stripe.SetBackend(stripe.APIBackend, stripe.GetBackendWithConfig(stripe.APIBackend, &stripe.BackendConfig{
		URL: stripe.String(srv.URL),
	}))
	stripe.Key = "sk_test_rgpd_erasure"
	t.Cleanup(func() {
		srv.Close()
		stripe.SetBackend(stripe.APIBackend, prevBackend)
		stripe.Key = prevKey
	})
	return cap
}

// seedUserSubscriptionWithCustomer inserts a self-owned subscription for userID
// carrying the given stripe_customer_id.
func seedUserSubscriptionWithCustomer(t *testing.T, db *gorm.DB, userID, customerID string) {
	t.Helper()
	priceID := "price_rgpd_" + uuid.NewString()
	plan := &models.SubscriptionPlan{
		Name: "RGPD Plan", PriceAmount: 1999, Currency: "eur",
		BillingInterval: "month", StripePriceID: &priceID, IsActive: true,
	}
	require.NoError(t, db.Create(plan).Error)

	subID := "sub_rgpd_" + uuid.NewString()
	require.NoError(t, db.Create(&models.UserSubscription{
		UserID:               userID,
		SubscriptionPlanID:   plan.ID,
		StripeSubscriptionID: &subID,
		StripeCustomerID:     &customerID,
		Status:               "active",
		CurrentPeriodStart:   time.Now(),
		CurrentPeriodEnd:     time.Now().Add(30 * 24 * time.Hour),
	}).Error)
}

// 1. The Stripe Customer is deleted on account erasure.
func TestUserDeletion_DeletesStripeCustomer(t *testing.T) {
	db := freshTestDB(t)
	userID := "rgpd-user-" + uuid.NewString()
	customerID := "cus_" + uuid.NewString()
	seedUserSubscriptionWithCustomer(t, db, userID, customerID)

	helper := services.NewPaymentDeletionHelper(db)
	cap := installCustomerDeleteBackend(t, "")

	err := asCustomerEraser(t, helper).DeleteStripeCustomersForUser(userID)
	require.NoError(t, err, "deleting an existing customer must succeed")

	assert.Contains(t, cap.distinctDeleted(), customerID,
		"RGPD: account erasure must send DELETE /v1/customers/%s to Stripe. Today no "+
			"customer deletion happens at all — the user's PII survives on Stripe.", customerID)
}

// 2. An already-deleted / missing customer (Stripe 404) is idempotent success.
func TestUserDeletion_AlreadyDeletedCustomer_Succeeds(t *testing.T) {
	db := freshTestDB(t)
	userID := "rgpd-user-" + uuid.NewString()
	customerID := "cus_" + uuid.NewString()
	seedUserSubscriptionWithCustomer(t, db, userID, customerID)

	helper := services.NewPaymentDeletionHelper(db)
	cap := installCustomerDeleteBackend(t, "missing") // Stripe returns 404 resource_missing

	err := asCustomerEraser(t, helper).DeleteStripeCustomersForUser(userID)

	// The DELETE must have been ATTEMPTED...
	assert.Contains(t, cap.distinctDeleted(), customerID,
		"the erasure must still attempt DELETE /v1/customers/%s even if it may be gone", customerID)
	// ...and a 404 (already deleted) must be treated as success, not abort.
	require.NoError(t, err,
		"IDEMPOTENT: a missing/already-deleted Stripe customer (404 resource_missing) "+
			"must count as success so account deletion is not wedged on a customer that "+
			"Stripe already removed.")
}

// 3. Every DISTINCT customer id owned by the user is deleted (and duplicates are
// collapsed to a single DELETE).
func TestUserDeletion_MultipleCustomerIDs_AllDeleted(t *testing.T) {
	db := freshTestDB(t)
	userID := "rgpd-user-" + uuid.NewString()
	custA := "cus_A_" + uuid.NewString()
	custB := "cus_B_" + uuid.NewString()
	// Two distinct customer ids (e.g. a legacy customer + a recreated one) plus a
	// duplicate of custA to prove de-duplication.
	seedUserSubscriptionWithCustomer(t, db, userID, custA)
	seedUserSubscriptionWithCustomer(t, db, userID, custB)
	seedUserSubscriptionWithCustomer(t, db, userID, custA)

	helper := services.NewPaymentDeletionHelper(db)
	cap := installCustomerDeleteBackend(t, "")

	err := asCustomerEraser(t, helper).DeleteStripeCustomersForUser(userID)
	require.NoError(t, err)

	distinct := cap.distinctDeleted()
	assert.ElementsMatch(t, []string{custA, custB}, distinct,
		"RGPD: all DISTINCT customer ids owned by the user must be deleted exactly once "+
			"(here custA appears twice in the data but must be deleted once). Got: %v", distinct)
}

// 4. Fail-closed: a hard Stripe error (non-404) aborts the deletion so the
// caller does not drop the account while the Stripe customer still exists.
func TestUserDeletion_HardStripeError_Aborts(t *testing.T) {
	db := freshTestDB(t)
	userID := "rgpd-user-" + uuid.NewString()
	customerID := "cus_" + uuid.NewString()
	seedUserSubscriptionWithCustomer(t, db, userID, customerID)

	helper := services.NewPaymentDeletionHelper(db)
	_ = installCustomerDeleteBackend(t, "error") // Stripe returns 500

	err := asCustomerEraser(t, helper).DeleteStripeCustomersForUser(userID)
	require.Error(t, err,
		"FAIL-CLOSED: a hard Stripe error (non-404) must abort erasure and return an "+
			"error, mirroring CancelAllActiveSubscriptionsForUser — the caller must not "+
			"drop the Casdoor account while the Stripe customer is still present.")
}

// -----------------------------------------------------------------------------
// Ownership-scope follow-up (reviewer): erasure must delete the customers the
// user OWNS — no more, no less. A bulk-license batch row carries
// purchaser_user_id = the buyer and stripe_customer_id = the buyer's customer.
// -----------------------------------------------------------------------------

// seedBulkLicenseRow inserts a bulk-license UserSubscription: user_id is the
// seat holder ("" when unassigned), purchaser_user_id is the buyer, and the
// stripe_customer_id is the buyer's customer. StripeSubscriptionID is left NULL
// (post-!269 shape).
func seedBulkLicenseRow(t *testing.T, db *gorm.DB, userID string, purchaserUserID *string, customerID string) {
	t.Helper()
	priceID := "price_bulkown_" + uuid.NewString()
	plan := &models.SubscriptionPlan{
		Name: "Bulk Own Plan", PriceAmount: 1999, Currency: "eur",
		BillingInterval: "month", StripePriceID: &priceID, IsActive: true,
	}
	require.NoError(t, db.Create(plan).Error)

	require.NoError(t, db.Create(&models.UserSubscription{
		UserID:             userID,
		PurchaserUserID:    purchaserUserID,
		SubscriptionPlanID: plan.ID,
		StripeCustomerID:   &customerID,
		Status:             "active",
		CurrentPeriodStart: time.Now(),
		CurrentPeriodEnd:   time.Now().Add(30 * 24 * time.Hour),
	}).Error)
}

// 5. GREEN GUARD — erasing an ASSIGNEE must NOT delete the purchaser's Stripe
// customer. The assigned bulk-license row has user_id = assignee but
// purchaser_user_id = the buyer and stripe_customer_id = the buyer's customer.
// The ownership predicate (purchaser_user_id IS NULL OR = user_id) excludes it.
func TestUserDeletion_AssignedBulkLicense_DoesNotDeletePurchaserCustomer(t *testing.T) {
	db := freshTestDB(t)
	assignee := "assignee-" + uuid.NewString()
	purchaser := "purchaser-" + uuid.NewString()
	purchaserCustomer := "cus_purchaser_" + uuid.NewString()

	seedBulkLicenseRow(t, db, assignee, &purchaser, purchaserCustomer)

	helper := services.NewPaymentDeletionHelper(db)
	cap := installCustomerDeleteBackend(t, "")

	err := asCustomerEraser(t, helper).DeleteStripeCustomersForUser(assignee)
	require.NoError(t, err)

	assert.NotContains(t, cap.distinctDeleted(), purchaserCustomer,
		"HARM CASE: deleting an assignee's account must NEVER delete the purchaser's "+
			"Stripe customer (%s) — it belongs to the buyer, not the seat holder.", purchaserCustomer)
	assert.Empty(t, cap.distinctDeleted(),
		"an assignee who owns no customer of their own should trigger zero customer deletes")
}

// 6. RED — a PURE bulk PURCHASER (no personal subscription; only batch rows
// where purchaser_user_id = them, user_id = "" or an assignee) must have THEIR
// Stripe customer deleted on erasure.
//
// RED today: the ownership query filters `user_id = ?`, so a purchaser whose
// only rows are bulk-license rows (user_id = "" / assignee) matches nothing and
// their customer is never erased — a GDPR gap. The fix must also match rows
// where purchaser_user_id = the user. NOTE: deleting the customer also cancels
// its remaining subscriptions server-side, so this same broadening additionally
// stops batch billing for a deleted purchaser.
func TestUserDeletion_PureBulkPurchaser_CustomerIsDeleted(t *testing.T) {
	db := freshTestDB(t)
	purchaser := "purchaser-" + uuid.NewString()
	purchaserCustomer := "cus_purchaser_" + uuid.NewString()

	// Two unassigned batch seats bought by `purchaser` — no personal sub for them.
	seedBulkLicenseRow(t, db, "", &purchaser, purchaserCustomer)
	seedBulkLicenseRow(t, db, "", &purchaser, purchaserCustomer)

	helper := services.NewPaymentDeletionHelper(db)
	cap := installCustomerDeleteBackend(t, "")

	err := asCustomerEraser(t, helper).DeleteStripeCustomersForUser(purchaser)
	require.NoError(t, err)

	assert.Contains(t, cap.distinctDeleted(), purchaserCustomer,
		"RGPD: a pure bulk purchaser (no personal subscription) must still have their "+
			"OWN Stripe customer %s erased. Today the query only matches user_id = ?, "+
			"so a buyer whose rows are all bulk-license rows (user_id = '' / assignee) "+
			"is missed. Fix: also match purchaser_user_id = the user.", purchaserCustomer)
}
