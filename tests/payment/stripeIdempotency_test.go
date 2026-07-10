// tests/payment/stripeIdempotency_test.go
//
// RED-phase failing tests for issue #365 / MR !268: mutating Stripe API calls
// carry NO idempotency key derived from a stable request identity.
//
// `grep -n SetIdempotencyKey src/` returns zero hits, so today the service never
// calls params.SetIdempotencyKey(...). IMPORTANT nuance that drives the test
// design: stripe-go v85 does NOT leave the Idempotency-Key header empty when the
// caller omits it — for every HTTP write method it auto-generates a *fresh
// random* key (see stripe-go/v85/stripe.go: `req.Header.Add("Idempotency-Key",
// NewIdempotencyKey())`). Therefore "the header is present and non-empty" is
// TRUE today and is NOT a valid red signal.
//
// The real defect is that the key is NOT stable across retries of the same
// logical operation: two invocations of the same logical request send two
// DIFFERENT random keys, so a client/network retry of e.g. "create the checkout
// session for user U / plan P" is NOT deduplicated by Stripe and can double-
// charge or create duplicate customers/subscriptions.
//
// Desired behavior (what these tests pin):
//   - Same logical operation (same inputs) -> SAME Idempotency-Key on retry.
//     (FAILS today: two independent random keys.)
//   - Different logical operations -> DIFFERENT keys. (Guard: passes today
//     because keys are random; locks in that a naive fix must not hardcode a
//     single constant key, which would collapse all requests into one.)
//
// Test approach — no src changes required to inject the backend:
// stripe-go uses package-level API functions (customer.New, session.New, ...)
// that dispatch through the GLOBAL backend. We override it with
// stripe.SetBackend(APIBackend, GetBackendWithConfig{URL: httptest server}) and
// capture the Idempotency-Key header the SDK sends. The checkout methods also
// call casdoorsdk.GetUserByUserId, so we point the Casdoor SDK's global client
// at a tiny fake that returns a user. All globals are saved and restored via
// t.Cleanup so other payment tests are unaffected.
package payment_tests

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v85"
	"gorm.io/gorm"
)

// -----------------------------------------------------------------------------
// Fake Stripe backend that records the Idempotency-Key header per logical call.
// -----------------------------------------------------------------------------

type stripeIdemCapture struct {
	mu   sync.Mutex
	keys map[string][]string // logical name -> idempotency keys, in request order
}

func (c *stripeIdemCapture) record(name, key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.keys[name] = append(c.keys[name], key)
}

func (c *stripeIdemCapture) get(name string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, len(c.keys[name]))
	copy(out, c.keys[name])
	return out
}

// installFakeStripeBackend points the global stripe backend at a local server
// that captures Idempotency-Key headers and returns minimal valid JSON. Globals
// (backend + key) are restored on cleanup.
//
// Logical names captured:
//   - "customer_create"  : POST .../v1/customers
//   - "checkout_session" : POST .../v1/checkout/sessions
func installFakeStripeBackend(t *testing.T) *stripeIdemCapture {
	t.Helper()

	cap := &stripeIdemCapture{keys: map[string][]string{}}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("Idempotency-Key")
		path := r.URL.Path

		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(path, "checkout/sessions"):
			cap.record("checkout_session", key)
			fmt.Fprint(w, `{"id":"cs_test_fake","object":"checkout.session","url":"https://checkout.stripe.test/pay/cs_test_fake"}`)
		case strings.Contains(path, "/customers/"):
			// customer UPDATE (path has an id segment) — not asserted on.
			cap.record("customer_update", key)
			fmt.Fprint(w, `{"id":"cus_test_fake","object":"customer"}`)
		case strings.HasSuffix(path, "/customers"):
			cap.record("customer_create", key)
			fmt.Fprint(w, `{"id":"cus_test_fake","object":"customer"}`)
		default:
			fmt.Fprint(w, `{}`)
		}
	}))

	prevBackend := stripe.GetBackend(stripe.APIBackend)
	prevKey := stripe.Key

	stripe.SetBackend(stripe.APIBackend, stripe.GetBackendWithConfig(stripe.APIBackend, &stripe.BackendConfig{
		URL: stripe.String(srv.URL),
	}))
	stripe.Key = "sk_test_fake_idempotency"

	t.Cleanup(func() {
		srv.Close()
		stripe.SetBackend(stripe.APIBackend, prevBackend)
		stripe.Key = prevKey
	})

	return cap
}

// installFakeCasdoor points the Casdoor SDK global client at a fake server so
// casdoorsdk.GetUserByUserId(...) resolves to the given email/name. Required by
// the checkout methods, which look up the user before any Stripe call.
func installFakeCasdoor(t *testing.T, email, name string) {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// GetUserByUserId hits /api/get-user and expects {"status":"ok","data":{...}}.
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","msg":"","data":{"owner":"test-org","name":%q,"email":%q,"id":"user-id"}}`, name, email)
	}))
	t.Cleanup(srv.Close)

	casdoorsdk.InitConfig(srv.URL, "test-client-id", "test-client-secret", "", "test-org", "test-app")
	t.Cleanup(func() {
		// Reset to zero-config so a dangling pointer at a closed server can't
		// affect any later test (no payment test relies on Casdoor being set).
		casdoorsdk.InitConfig("", "", "", "", "", "")
	})
}

// -----------------------------------------------------------------------------
// Bug — customer creation carries no stable idempotency key
// -----------------------------------------------------------------------------

// TestStripeService_CreateOrGetCustomer_IdempotencyKey_StableAcrossRetries pins
// the core defect on the highest-risk mutating call site: customer creation
// (stripeService.go:212, customer.New). Two invocations of CreateOrGetCustomer
// for the SAME user represent the same logical operation (a retry) and must send
// the SAME Idempotency-Key so Stripe does not create two customers.
//
// FAILS today: the service never calls SetIdempotencyKey, so stripe-go emits a
// fresh random key on each call — the two keys differ.
func TestStripeService_CreateOrGetCustomer_IdempotencyKey_StableAcrossRetries(t *testing.T) {
	db := freshTestDB(t)
	cap := installFakeStripeBackend(t)
	svc := services.NewStripeService(db)

	userID := "user_idem_" + uuid.NewString()
	email := "idem@example.com"
	name := "Idem User"

	// First attempt.
	id1, err := svc.CreateOrGetCustomer(userID, email, name)
	require.NoError(t, err, "first CreateOrGetCustomer should succeed against fake backend")
	// Second attempt = retry of the same logical operation (nothing persisted
	// in between, so it hits customer.New again).
	id2, err := svc.CreateOrGetCustomer(userID, email, name)
	require.NoError(t, err, "second CreateOrGetCustomer should succeed against fake backend")
	require.Equal(t, id1, id2)

	keys := cap.get("customer_create")
	require.Len(t, keys, 2, "expected exactly two customer-create calls (one per attempt)")
	require.NotEmpty(t, keys[0], "idempotency key must be present on customer creation")
	require.NotEmpty(t, keys[1], "idempotency key must be present on customer creation")

	assert.Equal(t, keys[0], keys[1],
		"IDEMPOTENCY: a retry of the same logical customer creation (same userID) "+
			"must reuse the SAME Idempotency-Key so Stripe deduplicates it. Today "+
			"the service sets no key, so stripe-go auto-generates a fresh random "+
			"key each call (%q vs %q) — a retry creates a SECOND Stripe customer. "+
			"Fix: params.SetIdempotencyKey(<stable identity, e.g. hash of "+
			"'customer:'+userID>) in CreateOrGetCustomer before customer.New.",
		keys[0], keys[1])
}

// TestStripeService_CreateOrGetCustomer_IdempotencyKey_DistinctPerUser is a
// companion guard: two DIFFERENT users are different logical operations and must
// get DIFFERENT keys. This already passes today (random keys) but locks in that
// the fix must derive the key from request identity — a naive fix that hardcodes
// one constant key would make every customer creation collapse into a single
// Stripe idempotency slot (the first customer wins, everyone else silently
// reuses it). This test would catch that.
func TestStripeService_CreateOrGetCustomer_IdempotencyKey_DistinctPerUser(t *testing.T) {
	db := freshTestDB(t)
	cap := installFakeStripeBackend(t)
	svc := services.NewStripeService(db)

	_, err := svc.CreateOrGetCustomer("user_A_"+uuid.NewString(), "a@example.com", "User A")
	require.NoError(t, err)
	_, err = svc.CreateOrGetCustomer("user_B_"+uuid.NewString(), "b@example.com", "User B")
	require.NoError(t, err)

	keys := cap.get("customer_create")
	require.Len(t, keys, 2)
	assert.NotEqual(t, keys[0], keys[1],
		"different users are different logical operations and must NOT share an "+
			"Idempotency-Key — otherwise Stripe would dedupe the second user's "+
			"customer creation against the first")
}

// -----------------------------------------------------------------------------
// Bug — checkout session creation carries no stable idempotency key
// -----------------------------------------------------------------------------

// TestStripeService_CreateCheckoutSession_IdempotencyKey_StableAcrossRetries
// pins the defect on checkout session creation (stripeService.go:348,
// session.New). A double-submitted checkout for the same user+plan must reuse
// the same key so Stripe returns the SAME session rather than opening a second
// billable checkout.
//
// FAILS today: fresh random key per call.
func TestStripeService_CreateCheckoutSession_IdempotencyKey_StableAcrossRetries(t *testing.T) {
	db := freshTestDB(t)
	cap := installFakeStripeBackend(t)
	installFakeCasdoor(t, "checkout@example.com", "Checkout User")
	svc := services.NewStripeService(db)

	plan := activeStripePlan(t, db, "Checkout Plan")

	input := dto.CreateCheckoutSessionInput{
		SubscriptionPlanID: plan.ID,
		SuccessURL:         "https://app.test/success",
		CancelURL:          "https://app.test/cancel",
	}

	userID := "user_checkout_" + uuid.NewString()
	_, err := svc.CreateCheckoutSession(userID, input, nil)
	require.NoError(t, err, "first CreateCheckoutSession should succeed against fakes")
	_, err = svc.CreateCheckoutSession(userID, input, nil)
	require.NoError(t, err, "second CreateCheckoutSession should succeed against fakes")

	keys := cap.get("checkout_session")
	require.Len(t, keys, 2, "expected exactly two checkout-session create calls")
	require.NotEmpty(t, keys[0])
	require.NotEmpty(t, keys[1])

	assert.Equal(t, keys[0], keys[1],
		"IDEMPOTENCY: a retry of the same checkout (same user+plan) must reuse the "+
			"SAME Idempotency-Key on session.New so Stripe returns the same session "+
			"instead of a second billable checkout. Today no key is set, so the two "+
			"calls carry different random keys (%q vs %q). Fix: "+
			"params.SetIdempotencyKey(<stable identity from userID + planID + intent>) "+
			"in CreateCheckoutSession before session.New.",
		keys[0], keys[1])
}

// TestStripeService_CreateBulkCheckoutSession_IdempotencyKey_StableAcrossRetries
// pins the same defect on bulk purchase checkout (stripeService.go:484,
// session.New) — the highest-value duplication risk since it provisions N paid
// licenses.
//
// FAILS today: fresh random key per call.
func TestStripeService_CreateBulkCheckoutSession_IdempotencyKey_StableAcrossRetries(t *testing.T) {
	db := freshTestDB(t)
	cap := installFakeStripeBackend(t)
	installFakeCasdoor(t, "bulk@example.com", "Bulk User")
	svc := services.NewStripeService(db)

	plan := activeStripePlan(t, db, "Bulk Plan")
	// The bulk-purchase paths now require the plan to carry the group_management
	// feature. Grant it on this bulk-path fixture only — the shared activeStripePlan
	// helper stays feature-less for the individual-path callers. Select("Features")
	// updates only that column and routes through the field's JSON serializer.
	plan.Features = []string{"group_management"}
	require.NoError(t, db.Model(plan).Select("Features").Updates(plan).Error)

	input := dto.CreateBulkCheckoutSessionInput{
		SubscriptionPlanID: plan.ID,
		Quantity:           5,
		SuccessURL:         "https://app.test/success",
		CancelURL:          "https://app.test/cancel",
	}

	userID := "user_bulk_" + uuid.NewString()
	_, err := svc.CreateBulkCheckoutSession(userID, input)
	require.NoError(t, err, "first CreateBulkCheckoutSession should succeed against fakes")
	_, err = svc.CreateBulkCheckoutSession(userID, input)
	require.NoError(t, err, "second CreateBulkCheckoutSession should succeed against fakes")

	keys := cap.get("checkout_session")
	require.Len(t, keys, 2, "expected exactly two bulk checkout-session create calls")
	require.NotEmpty(t, keys[0])
	require.NotEmpty(t, keys[1])

	assert.Equal(t, keys[0], keys[1],
		"IDEMPOTENCY: a retry of the same bulk checkout (same user+plan+quantity) "+
			"must reuse the SAME Idempotency-Key on session.New so a double-submit "+
			"does not provision two batches of paid licenses. Today the two calls "+
			"carry different random keys (%q vs %q). Fix: "+
			"params.SetIdempotencyKey(<stable identity from userID + planID + "+
			"quantity + intent>) in CreateBulkCheckoutSession before session.New.",
		keys[0], keys[1])
}

// activeStripePlan seeds an active SubscriptionPlan with a Stripe price ID so the
// checkout methods pass their plan validation and reach session.New.
func activeStripePlan(t *testing.T, db *gorm.DB, name string) *models.SubscriptionPlan {
	t.Helper()
	priceID := "price_idem_" + uuid.NewString()
	plan := &models.SubscriptionPlan{
		Name:            name,
		PriceAmount:     1999,
		Currency:        "eur",
		BillingInterval: "month",
		StripePriceID:   &priceID,
		IsActive:        true,
	}
	require.NoError(t, db.Create(plan).Error)
	return plan
}
