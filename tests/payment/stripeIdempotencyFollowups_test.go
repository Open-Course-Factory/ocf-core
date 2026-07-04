// tests/payment/stripeIdempotencyFollowups_test.go
//
// Follow-up idempotency-key coverage from the !268 review (issue #366 / MR !273).
// Reuses the fake-Stripe-backend technique (global backend override + captured
// Idempotency-Key headers) from stripeIdempotency_test.go.
//
// Recap of the real defect: stripe-go auto-generates a FRESH RANDOM Idempotency-
// Key on every POST when the caller sets none, so "header present" is always
// true — the red signal is that the key is NOT STABLE across retries of the same
// logical operation.
package payment_tests

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v85"
)

// idemKeyCapture records the Idempotency-Key header of every request, keyed by
// URL path.
type idemKeyCapture struct {
	mu   sync.Mutex
	keys map[string][]string // path -> idempotency keys in request order
}

// keysMatching returns the captured keys whose path contains sub, in order.
func (c *idemKeyCapture) keysMatching(sub string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []string
	for path, ks := range c.keys {
		if strings.Contains(path, sub) {
			out = append(out, ks...)
		}
	}
	return out
}

// installIdemKeyBackend points the global stripe backend at a fake server that
// captures Idempotency-Key headers and returns minimal valid JSON for the
// resources exercised here. Globals restored on cleanup.
func installIdemKeyBackend(t *testing.T) *idemKeyCapture {
	t.Helper()
	cap := &idemKeyCapture{keys: map[string][]string{}}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("Idempotency-Key")
		path := r.URL.Path
		cap.mu.Lock()
		cap.keys[path] = append(cap.keys[path], key)
		cap.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(path, "/send"):
			fmt.Fprint(w, `{"id":"in_test","object":"invoice"}`)
		case strings.Contains(path, "/v1/prices"):
			fmt.Fprint(w, `{"id":"price_test","object":"price"}`)
		case strings.Contains(path, "/v1/products"):
			fmt.Fprint(w, `{"id":"prod_test","object":"product"}`)
		case strings.Contains(path, "/v1/subscriptions"):
			fmt.Fprint(w, `{"id":"sub_test","object":"subscription","status":"incomplete"}`)
		default:
			fmt.Fprint(w, `{}`)
		}
	}))

	prevBackend := stripe.GetBackend(stripe.APIBackend)
	prevKey := stripe.Key
	stripe.SetBackend(stripe.APIBackend, stripe.GetBackendWithConfig(stripe.APIBackend, &stripe.BackendConfig{
		URL: stripe.String(srv.URL),
	}))
	stripe.Key = "sk_test_idem_followups"
	t.Cleanup(func() {
		srv.Close()
		stripe.SetBackend(stripe.APIBackend, prevBackend)
		stripe.Key = prevKey
	})
	return cap
}

// -----------------------------------------------------------------------------
// Item 1 — CreateSubscriptionPlanInStripe: product.New + price.New need keys
// -----------------------------------------------------------------------------

// TestStripeService_CreateSubscriptionPlanInStripe_IdempotencyKeys pins that the
// product and price creation POSTs carry DETERMINISTIC idempotency keys derived
// from plan.ID: the same plan re-synced twice must reuse the SAME key on each
// endpoint (so a retry doesn't create a duplicate Stripe product/price), and a
// different plan must get different keys.
//
// RED today: neither product.New (~:608) nor price.New (~:626) sets a key, so
// stripe-go emits a fresh random key per call.
func TestStripeService_CreateSubscriptionPlanInStripe_IdempotencyKeys(t *testing.T) {
	db := freshTestDB(t)
	cap := installIdemKeyBackend(t)
	svc := services.NewStripeService(db)

	price1 := "price_seed_" + uuid.NewString()
	plan := &models.SubscriptionPlan{
		Name: "Plan A", Description: "desc", PriceAmount: 1999, Currency: "eur",
		BillingInterval: "month", StripePriceID: &price1, IsActive: true,
	}
	require.NoError(t, db.Create(plan).Error)

	// Two syncs of the SAME plan. The final EditEntity write is irrelevant here —
	// both product.New and price.New fire (and are captured) before it.
	_ = svc.CreateSubscriptionPlanInStripe(plan)
	_ = svc.CreateSubscriptionPlanInStripe(plan)

	productKeys := cap.keysMatching("/v1/products")
	priceKeys := cap.keysMatching("/v1/prices")
	require.Len(t, productKeys, 2, "expected two product-create POSTs")
	require.Len(t, priceKeys, 2, "expected two price-create POSTs")
	require.NotEmpty(t, productKeys[0])
	require.NotEmpty(t, priceKeys[0])

	assert.Equal(t, productKeys[0], productKeys[1],
		"IDEMPOTENCY: re-syncing the same plan must reuse the SAME product-create "+
			"Idempotency-Key (derived from plan.ID) so a retry does not create a "+
			"duplicate Stripe product. Today product.New sets no key -> random (%q vs %q).",
		productKeys[0], productKeys[1])
	assert.Equal(t, priceKeys[0], priceKeys[1],
		"IDEMPOTENCY: re-syncing the same plan must reuse the SAME price-create "+
			"Idempotency-Key. Today price.New sets no key -> random (%q vs %q).",
		priceKeys[0], priceKeys[1])

	// A different plan must get different keys (guards against a constant key).
	price2 := "price_seed_" + uuid.NewString()
	planB := &models.SubscriptionPlan{
		Name: "Plan B", Description: "desc", PriceAmount: 2999, Currency: "eur",
		BillingInterval: "month", StripePriceID: &price2, IsActive: true,
	}
	require.NoError(t, db.Create(planB).Error)
	_ = svc.CreateSubscriptionPlanInStripe(planB)

	productKeysAll := cap.keysMatching("/v1/products")
	require.Len(t, productKeysAll, 3)
	assert.NotEqual(t, productKeys[0], productKeysAll[2],
		"different plans must not share a product-create key")
}

// -----------------------------------------------------------------------------
// Item 2 — SendInvoice: invoice.SendInvoice needs a key from the invoice id
// -----------------------------------------------------------------------------

// TestStripeService_SendInvoice_IdempotencyKey pins that SendInvoice sends a
// deterministic key derived from the invoice id, so a retried send does not
// re-email the customer.
//
// RED today: invoice.SendInvoice (~:2200) sets no key -> random per call.
func TestStripeService_SendInvoice_IdempotencyKey(t *testing.T) {
	db := freshTestDB(t)
	cap := installIdemKeyBackend(t)
	svc := services.NewStripeService(db)

	invoiceID := "in_" + uuid.NewString()
	require.NoError(t, svc.SendInvoice(invoiceID))
	require.NoError(t, svc.SendInvoice(invoiceID))

	keys := cap.keysMatching("/send")
	require.Len(t, keys, 2, "expected two invoice-send POSTs")
	require.NotEmpty(t, keys[0])
	assert.Equal(t, keys[0], keys[1],
		"IDEMPOTENCY: re-sending the same invoice must reuse the SAME Idempotency-Key "+
			"(derived from the invoice id) so the customer is not e-mailed twice. Today "+
			"invoice.SendInvoice sets no key -> random (%q vs %q).", keys[0], keys[1])

	// Different invoice -> different key.
	require.NoError(t, svc.SendInvoice("in_"+uuid.NewString()))
	allKeys := cap.keysMatching("/send")
	require.Len(t, allKeys, 3)
	assert.NotEqual(t, keys[0], allKeys[2], "different invoices must not share a send key")
}

// -----------------------------------------------------------------------------
// Item 3 — CreateSubscriptionWithQuantity: GUARD (key implemented in !268)
// -----------------------------------------------------------------------------

// TestStripeService_CreateSubscriptionWithQuantity_IdempotencyKey_StableAcrossRetries
// is a GUARD: the key (customer+plan+quantity+pm+day) was added in !268 but never
// tested. Expected GREEN today. It locks the behavior in against regression.
func TestStripeService_CreateSubscriptionWithQuantity_IdempotencyKey_StableAcrossRetries(t *testing.T) {
	db := freshTestDB(t)
	cap := installIdemKeyBackend(t)
	svc := services.NewStripeService(db)

	priceID := "price_" + uuid.NewString()
	plan := &models.SubscriptionPlan{
		Name: "Qty Plan", PriceAmount: 1999, Currency: "eur",
		BillingInterval: "month", StripePriceID: &priceID, IsActive: true,
	}
	plan.ID = uuid.New()

	customerID := "cus_" + uuid.NewString()
	pmID := "pm_" + uuid.NewString()

	_, err := svc.CreateSubscriptionWithQuantity(customerID, plan, 5, pmID)
	require.NoError(t, err)
	_, err = svc.CreateSubscriptionWithQuantity(customerID, plan, 5, pmID)
	require.NoError(t, err)

	keys := cap.keysMatching("/v1/subscriptions")
	require.Len(t, keys, 2, "expected two subscription-create POSTs")
	require.NotEmpty(t, keys[0])
	assert.Equal(t, keys[0], keys[1],
		"GUARD: same customer+plan+quantity+pm on the same day must reuse the SAME "+
			"subscription-create Idempotency-Key (implemented in !268). Got %q vs %q.",
		keys[0], keys[1])
}
