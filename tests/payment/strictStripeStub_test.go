// tests/payment/strictStripeStub_test.go
//
// A Stripe API stub that serves only the subscriptions a test gives it and
// fails the test on any other call.
package payment_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stripe/stripe-go/v85"
)

// Billing period carried by every subscription stripeSubscriptionJSON builds.
const (
	fakePeriodStart int64 = 1750000000
	fakePeriodEnd   int64 = 1752592000
)

// stripeSubscriptionJSON is a Stripe subscription as Stripe sends it, in a
// webhook event or in an API response.
func stripeSubscriptionJSON(id, customerID, status string, metadata map[string]string) string {
	meta, _ := json.Marshal(metadata)
	return fmt.Sprintf(`{"id":%q,"object":"subscription","customer":%q,"status":%q,"cancel_at_period_end":false,"metadata":%s,`+
		`"items":{"object":"list","data":[{"id":"si_fake","object":"subscription_item","current_period_start":%d,"current_period_end":%d}]}}`,
		id, customerID, status, meta, fakePeriodStart, fakePeriodEnd)
}

// installStrictStripeSubscriptions points the global Stripe backend at a local
// server that knows only the given subscriptions (id -> JSON; list and get by
// id) and fails the test on any other call. Unlike the package's lenient fakes
// it answers nothing by default, so with no subscription it proves a code path
// makes no Stripe call at all.
func installStrictStripeSubscriptions(t *testing.T, subs map[string]string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		byID, known := subs[strings.TrimPrefix(r.URL.Path, "/v1/subscriptions/")]
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/subscriptions":
			all := make([]string, 0, len(subs))
			for _, sub := range subs {
				all = append(all, sub)
			}
			fmt.Fprintf(w, `{"object":"list","url":"/v1/subscriptions","has_more":false,"data":[%s]}`, strings.Join(all, ","))
		case r.Method == http.MethodGet && known:
			fmt.Fprint(w, byID)
		default:
			t.Errorf("unexpected Stripe call: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"error":{"message":"unexpected call","type":"invalid_request_error"}}`)
		}
	}))

	prevBackend := stripe.GetBackend(stripe.APIBackend)
	prevKey := stripe.Key
	// NewStripeService reads the key from the environment on construction.
	t.Setenv("STRIPE_SECRET_KEY", "sk_test_fake_strict")
	stripe.SetBackend(stripe.APIBackend, stripe.GetBackendWithConfig(stripe.APIBackend, &stripe.BackendConfig{
		URL: stripe.String(srv.URL),
	}))
	t.Cleanup(func() {
		srv.Close()
		stripe.SetBackend(stripe.APIBackend, prevBackend)
		stripe.Key = prevKey
	})
}
