// tests/payment/stripeTaxIdCollection_test.go
//
// Behavior/regression tests for issue #369 / MR !271 (B2B tax id collection at
// checkout).
//
// IMPORTANT — these tests are GREEN today, NOT red. The feature was already
// implemented on origin/main by commit 18835a2 ("feat: collect buyer tax id at
// checkout for valid b2b invoices"): both CreateCheckoutSession and
// CreateBulkCheckoutSession already send TaxIDCollection.Enabled=true alongside
// AutomaticTax.Enabled=true and CustomerUpdate{Name:auto, Address:auto}. The
// branch feat/stripe-tax-id-collection has zero commits ahead of origin/main.
//
// They are kept as regression guards for a compliance-critical behavior (EU
// reverse charge requires the buyer to be able to enter a VAT number, and Stripe
// rejects tax_id_collection on an existing Customer unless customer_update[name]
// is set). The exact Stripe form-key encodings below were captured live from
// stripe-go's serializer against a fake backend, not guessed.
package payment_tests

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v85"
)

// stripeBodyCapture records the raw (form-encoded) POST body of each Stripe
// request, keyed by request path.
type stripeBodyCapture struct {
	mu     sync.Mutex
	bodies map[string][]string
}

func (c *stripeBodyCapture) record(path, body string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.bodies[path] = append(c.bodies[path], body)
}

// checkoutSessionForm returns the concatenated form bodies of every
// POST /v1/checkout/sessions request seen (there is exactly one per checkout).
func (c *stripeBodyCapture) checkoutSessionForm() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return strings.Join(c.bodies["/v1/checkout/sessions"], "&")
}

// installTaxFormCapturingStripe points the global stripe backend at a local
// server that captures request bodies and returns minimal valid JSON. Globals
// are restored on cleanup.
func installTaxFormCapturingStripe(t *testing.T) *stripeBodyCapture {
	t.Helper()
	cap := &stripeBodyCapture{bodies: map[string][]string{}}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		cap.record(r.URL.Path, string(body))
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "checkout/sessions"):
			fmtFprint(w, `{"id":"cs_test_tax","object":"checkout.session","url":"https://checkout.stripe.test/pay"}`)
		default:
			fmtFprint(w, `{"id":"cus_test_tax","object":"customer"}`)
		}
	}))

	prevBackend := stripe.GetBackend(stripe.APIBackend)
	prevKey := stripe.Key
	stripe.SetBackend(stripe.APIBackend, stripe.GetBackendWithConfig(stripe.APIBackend, &stripe.BackendConfig{
		URL: stripe.String(srv.URL),
	}))
	stripe.Key = "sk_test_tax_capture"

	t.Cleanup(func() {
		srv.Close()
		stripe.SetBackend(stripe.APIBackend, prevBackend)
		stripe.Key = prevKey
	})
	return cap
}

func fmtFprint(w io.Writer, s string) { _, _ = io.WriteString(w, s) }

// TestStripeService_CreateCheckoutSession_EnablesTaxIdCollection asserts the
// personal checkout session enables tax id collection so B2B buyers can enter a
// VAT number (EU reverse charge).
//
// GREEN today (feature shipped in 18835a2).
func TestStripeService_CreateCheckoutSession_EnablesTaxIdCollection(t *testing.T) {
	db := freshTestDB(t)
	cap := installTaxFormCapturingStripe(t)
	installFakeCasdoor(t, "vat@example.com", "VAT Buyer")
	svc := services.NewStripeService(db)
	plan := activeStripePlan(t, db, "Tax Plan")

	_, err := svc.CreateCheckoutSession("user_tax_"+uuid.NewString(), dto.CreateCheckoutSessionInput{
		SubscriptionPlanID: plan.ID,
		SuccessURL:         "https://app.test/success",
		CancelURL:          "https://app.test/cancel",
	}, nil)
	require.NoError(t, err)

	form := cap.checkoutSessionForm()
	require.NotEmpty(t, form, "a checkout session request must have been sent")
	assert.Contains(t, form, "tax_id_collection[enabled]=true",
		"CreateCheckoutSession must enable tax id collection so B2B customers can "+
			"provide a VAT number (EU reverse charge). Captured form: %s", form)
	// AutomaticTax must remain on (the reviewer's premise) — cross-check.
	assert.Contains(t, form, "automatic_tax[enabled]=true",
		"automatic tax must stay enabled alongside tax id collection")
}

// TestStripeService_CreateBulkCheckoutSession_EnablesTaxIdCollection asserts the
// bulk checkout session also enables tax id collection.
//
// GREEN today (feature shipped in 18835a2).
func TestStripeService_CreateBulkCheckoutSession_EnablesTaxIdCollection(t *testing.T) {
	db := freshTestDB(t)
	cap := installTaxFormCapturingStripe(t)
	installFakeCasdoor(t, "bulkvat@example.com", "Bulk VAT Buyer")
	svc := services.NewStripeService(db)
	plan := activeStripePlan(t, db, "Bulk Tax Plan")

	_, err := svc.CreateBulkCheckoutSession("user_bulktax_"+uuid.NewString(), dto.CreateBulkCheckoutSessionInput{
		SubscriptionPlanID: plan.ID,
		Quantity:           5,
		SuccessURL:         "https://app.test/success",
		CancelURL:          "https://app.test/cancel",
	})
	require.NoError(t, err)

	form := cap.checkoutSessionForm()
	require.NotEmpty(t, form, "a bulk checkout session request must have been sent")
	assert.Contains(t, form, "tax_id_collection[enabled]=true",
		"CreateBulkCheckoutSession must enable tax id collection too. Captured form: %s", form)
	assert.Contains(t, form, "automatic_tax[enabled]=true",
		"automatic tax must stay enabled on the bulk path")
}

// TestStripeService_CreateCheckoutSession_SetsCustomerUpdateForTaxId asserts the
// customer_update fields required for tax id collection on an existing Customer.
// Stripe rejects tax_id_collection with an existing Customer unless
// customer_update[name] is set; the service always passes a Customer id, so this
// is mandatory. address=auto is also sent (automatic tax needs the address).
//
// GREEN today (feature shipped in 18835a2).
func TestStripeService_CreateCheckoutSession_SetsCustomerUpdateForTaxId(t *testing.T) {
	db := freshTestDB(t)
	cap := installTaxFormCapturingStripe(t)
	installFakeCasdoor(t, "cu@example.com", "CU Buyer")
	svc := services.NewStripeService(db)
	plan := activeStripePlan(t, db, "CU Plan")

	_, err := svc.CreateCheckoutSession("user_cu_"+uuid.NewString(), dto.CreateCheckoutSessionInput{
		SubscriptionPlanID: plan.ID,
		SuccessURL:         "https://app.test/success",
		CancelURL:          "https://app.test/cancel",
	}, nil)
	require.NoError(t, err)

	form := cap.checkoutSessionForm()
	require.NotEmpty(t, form)
	assert.Contains(t, form, "customer_update[name]=auto",
		"tax id collection with an existing Customer requires customer_update[name]=auto "+
			"(Stripe rejects the session otherwise). Captured form: %s", form)
	assert.Contains(t, form, "customer_update[address]=auto",
		"customer_update[address]=auto is also sent (automatic tax needs the address)")
}
