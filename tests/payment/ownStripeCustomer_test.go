// tests/payment/ownStripeCustomer_test.go
package payment_tests

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v85"
	"gorm.io/gorm"

	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"
)

const (
	ownCustomer       = "cus_own_holder"
	purchaserCustomer = "cus_purchaser"
	newCustomer       = "cus_new_holder"
)

// fakeStripeAnyCustomer serves the per-user Stripe calls (customer create, billing portal,
// invoice and card listing) for any customer, and records the customer each request named.
type fakeStripeAnyCustomer struct {
	mu        sync.Mutex
	customers []string // "<METHOD> <path> customer=<id>" per request
}

func (f *fakeStripeAnyCustomer) requests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.customers...)
}

func installFakeStripeAnyCustomer(t *testing.T) *fakeStripeAnyCustomer {
	t.Helper()
	f := &fakeStripeAnyCustomer{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = r.ParseForm()
		cus := r.Form.Get("customer")
		f.mu.Lock()
		f.customers = append(f.customers, fmt.Sprintf("%s %s customer=%s", r.Method, r.URL.Path, cus))
		f.mu.Unlock()
		switch {
		case r.URL.Path == "/v1/customers":
			fmt.Fprintf(w, `{"id":%q,"object":"customer"}`, newCustomer)
		case strings.HasPrefix(r.URL.Path, "/v1/customers/"):
			fmt.Fprintf(w, `{"id":%q,"object":"customer"}`, strings.TrimPrefix(r.URL.Path, "/v1/customers/"))
		case r.URL.Path == "/v1/billing_portal/sessions":
			fmt.Fprintf(w, `{"id":"bps_test","object":"billing_portal.session","customer":%q,"url":"https://portal.test/%s"}`, cus, cus)
		case r.URL.Path == "/v1/invoices":
			fmt.Fprintf(w, `{"object":"list","url":"/v1/invoices","has_more":false,"data":[{"id":"in_%s_%s","object":"invoice","customer":%q,"status":"paid","total":1000,"currency":"eur","number":"N-1","created":%d}]}`,
				cus, uuid.NewString()[:8], cus, time.Now().Unix())
		case r.URL.Path == "/v1/payment_methods":
			fmt.Fprintf(w, `{"object":"list","url":"/v1/payment_methods","has_more":false,"data":[{"id":"pm_%s_%s","object":"payment_method","type":"card","card":{"brand":"visa","last4":"4242","exp_month":12,"exp_year":2030}}]}`,
				cus, uuid.NewString()[:8])
		default:
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintf(w, `{"error":{"message":"unexpected %s %s","type":"invalid_request_error"}}`, r.Method, r.URL.Path)
		}
	}))

	prevBackend := stripe.GetBackend(stripe.APIBackend)
	prevKey := stripe.Key
	t.Setenv("STRIPE_SECRET_KEY", "sk_test_fake_own_customer")
	stripe.SetBackend(stripe.APIBackend, stripe.GetBackendWithConfig(stripe.APIBackend, &stripe.BackendConfig{
		URL: stripe.String(srv.URL),
	}))
	stripe.Key = "sk_test_fake_own_customer"
	t.Cleanup(func() {
		srv.Close()
		stripe.SetBackend(stripe.APIBackend, prevBackend)
		stripe.Key = prevKey
	})
	return f
}

// holderSub gives userID an active subscription on customerID, bought by purchaser ("" = by
// the user themselves), created at createdAt.
func holderSub(t *testing.T, db *gorm.DB, userID, customerID, purchaser string, createdAt time.Time) {
	t.Helper()
	sub := createTestUserSubscription(t, userID, createTestSubscriptionPlan(t))
	updates := map[string]any{"stripe_customer_id": customerID, "created_at": createdAt}
	if purchaser != "" {
		updates["purchaser_user_id"] = purchaser
		updates["subscription_type"] = "assigned"
	}
	require.NoError(t, db.Model(sub).Updates(updates).Error)
}

// licenceHolder returns a fresh user whose only subscription is a licence bought by someone else.
func licenceHolder(t *testing.T, db *gorm.DB) string {
	t.Helper()
	userID := "holder-" + uuid.NewString()
	holderSub(t, db, userID, purchaserCustomer, "purchaser-"+uuid.NewString(), time.Now())
	return userID
}

// selfBuyer returns a fresh user whose only subscription they bought themselves.
func selfBuyer(t *testing.T, db *gorm.DB) string {
	t.Helper()
	userID := "buyer-" + uuid.NewString()
	holderSub(t, db, userID, ownCustomer, "", time.Now())
	return userID
}

func assertNeverNamed(t *testing.T, fake *fakeStripeAnyCustomer, customerID string) {
	t.Helper()
	for _, req := range fake.requests() {
		assert.NotContains(t, req, "customer="+customerID, "no Stripe call may act on the purchaser's customer")
	}
}

func countRows(t *testing.T, db *gorm.DB, model any, userID string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(model).Where("user_id = ?", userID).Count(&n).Error)
	return n
}

// --- CreatePortalSession ---

func TestOwnStripeCustomer_PortalSession_LicenceOnly_AnsweredAsNoCustomer(t *testing.T) {
	db := freshTestDB(t)
	fake := installFakeStripeAnyCustomer(t)
	userID := licenceHolder(t, db)

	out, err := services.NewStripeService(db).CreatePortalSession(userID, dto.CreatePortalSessionInput{ReturnURL: "https://app.test"})

	require.Error(t, err)
	assert.Nil(t, out)
	assert.Contains(t, err.Error(), "portal session not available for free subscriptions",
		"a licence holder has no Stripe customer of their own: same answer as a user without one")
	assert.Empty(t, fake.requests(), "no billing portal session may be created")
}

func TestOwnStripeCustomer_PortalSession_SelfPurchased_UsesOwnCustomer(t *testing.T) {
	db := freshTestDB(t)
	fake := installFakeStripeAnyCustomer(t)
	userID := selfBuyer(t, db)

	out, err := services.NewStripeService(db).CreatePortalSession(userID, dto.CreatePortalSessionInput{ReturnURL: "https://app.test"})

	require.NoError(t, err)
	assert.Equal(t, "https://portal.test/"+ownCustomer, out.URL)
	assert.Equal(t, []string{"POST /v1/billing_portal/sessions customer=" + ownCustomer}, fake.requests())
}

// --- CreateOrGetCustomer ---

func TestOwnStripeCustomer_CreateOrGetCustomer_LicenceOnly_CreatesUsersOwnCustomer(t *testing.T) {
	db := freshTestDB(t)
	fake := installFakeStripeAnyCustomer(t)
	userID := licenceHolder(t, db)

	customerID, err := services.NewStripeService(db).CreateOrGetCustomer(userID, "holder@test.local", "Holder")

	require.NoError(t, err)
	assert.Equal(t, newCustomer, customerID, "a licence holder must get their own customer, not the purchaser's")
	assert.Equal(t, []string{"POST /v1/customers customer="}, fake.requests())
}

func TestOwnStripeCustomer_CreateOrGetCustomer_SelfPurchased_ReusesOwnCustomer(t *testing.T) {
	db := freshTestDB(t)
	fake := installFakeStripeAnyCustomer(t)
	userID := selfBuyer(t, db)

	customerID, err := services.NewStripeService(db).CreateOrGetCustomer(userID, "buyer@test.local", "Buyer")

	require.NoError(t, err)
	assert.Equal(t, ownCustomer, customerID)
	assert.Empty(t, fake.requests())
}

func TestOwnStripeCustomer_CreateOrGetCustomer_NewerLicenceAndOlderOwn_ReusesOwnCustomer(t *testing.T) {
	db := freshTestDB(t)
	fake := installFakeStripeAnyCustomer(t)
	userID := "mixed-" + uuid.NewString()
	holderSub(t, db, userID, ownCustomer, "", time.Now().Add(-time.Hour))
	holderSub(t, db, userID, purchaserCustomer, "purchaser-"+uuid.NewString(), time.Now())

	customerID, err := services.NewStripeService(db).CreateOrGetCustomer(userID, "mixed@test.local", "Mixed")

	require.NoError(t, err)
	assert.Equal(t, ownCustomer, customerID, "the user's own customer wins over a newer licence's purchaser customer")
	assert.Empty(t, fake.requests())
}

// --- SyncUserInvoices ---

func TestOwnStripeCustomer_SyncInvoices_LicenceOnly_ListsAndCreatesNothing(t *testing.T) {
	db := freshTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.Invoice{}))
	fake := installFakeStripeAnyCustomer(t)
	userID := licenceHolder(t, db)

	result, err := services.NewStripeService(db).SyncUserInvoices(userID)

	require.NoError(t, err)
	assert.Zero(t, result.ProcessedInvoices)
	assertNeverNamed(t, fake, purchaserCustomer)
	assert.Zero(t, countRows(t, db, &models.Invoice{}, userID), "the purchaser's invoices must not be copied to the licence holder")
}

func TestOwnStripeCustomer_SyncInvoices_SelfPurchased_CreatesOwnInvoice(t *testing.T) {
	db := freshTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.Invoice{}))
	_ = installFakeStripeAnyCustomer(t)
	userID := selfBuyer(t, db)

	result, err := services.NewStripeService(db).SyncUserInvoices(userID)

	require.NoError(t, err)
	assert.Equal(t, 1, result.CreatedInvoices)
	assert.Equal(t, int64(1), countRows(t, db, &models.Invoice{}, userID))
}

// --- SyncUserPaymentMethods ---

func TestOwnStripeCustomer_SyncPaymentMethods_LicenceOnly_ListsAndCreatesNothing(t *testing.T) {
	db := freshTestDB(t)
	fake := installFakeStripeAnyCustomer(t)
	userID := licenceHolder(t, db)

	result, err := services.NewStripeService(db).SyncUserPaymentMethods(userID)

	require.NoError(t, err)
	assert.Zero(t, result.ProcessedPaymentMethods)
	assertNeverNamed(t, fake, purchaserCustomer)
	assert.Zero(t, countRows(t, db, &models.PaymentMethod{}, userID), "the purchaser's cards must not be copied to the licence holder")
}

func TestOwnStripeCustomer_SyncPaymentMethods_SelfPurchased_CreatesOwnCard(t *testing.T) {
	db := freshTestDB(t)
	_ = installFakeStripeAnyCustomer(t)
	userID := selfBuyer(t, db)

	result, err := services.NewStripeService(db).SyncUserPaymentMethods(userID)

	require.NoError(t, err)
	assert.Equal(t, 1, result.CreatedPaymentMethods)
	assert.Equal(t, int64(1), countRows(t, db, &models.PaymentMethod{}, userID))
}
