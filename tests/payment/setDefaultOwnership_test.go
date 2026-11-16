// tests/payment/setDefaultOwnership_test.go
package payment_tests

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v85"
	"gorm.io/gorm"

	"soli/formations/src/payment/models"
	"soli/formations/src/payment/repositories"
	paymentController "soli/formations/src/payment/routes"
	"soli/formations/src/payment/services"
)

// defaultBillingAddressIDs returns the IDs of the user's addresses flagged as default.
func defaultBillingAddressIDs(t *testing.T, db *gorm.DB, userID string) []uuid.UUID {
	t.Helper()
	var ids []uuid.UUID
	require.NoError(t, db.Model(&models.BillingAddress{}).
		Where("user_id = ? AND is_default = ?", userID, true).Pluck("id", &ids).Error)
	return ids
}

// defaultPaymentMethodIDs returns the IDs of the user's payment methods flagged as default.
func defaultPaymentMethodIDs(t *testing.T, db *gorm.DB, userID string) []uuid.UUID {
	t.Helper()
	var ids []uuid.UUID
	require.NoError(t, db.Model(&models.PaymentMethod{}).
		Where("user_id = ? AND is_default = ?", userID, true).Pluck("id", &ids).Error)
	return ids
}

// setupDefaultBillingAddress gives the caller a current default address and a second one.
func setupDefaultBillingAddress(t *testing.T, db *gorm.DB, userID string) (current, other *models.BillingAddress) {
	t.Helper()
	current = createTestBillingAddress(t, userID)
	other = createTestBillingAddress(t, userID)
	require.NoError(t, db.Model(current).Update("is_default", true).Error)
	return current, other
}

func setupDefaultPaymentMethod(t *testing.T, db *gorm.DB, userID string) (current, other *models.PaymentMethod) {
	t.Helper()
	current = createTestPaymentMethod(t, userID)
	other = createTestPaymentMethod(t, userID)
	require.NoError(t, db.Model(current).Update("is_default", true).Error)
	return current, other
}

func TestSetDefaultBillingAddress_ForeignID_ReturnsNotFoundAndKeepsDefault(t *testing.T) {
	db := freshTestDB(t)
	repo := repositories.NewPaymentRepository(db)
	current, _ := setupDefaultBillingAddress(t, db, "caller")
	foreign := createTestBillingAddress(t, "someone-else")

	err := repo.SetDefaultBillingAddress("caller", foreign.ID)

	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
	assert.Equal(t, []uuid.UUID{current.ID}, defaultBillingAddressIDs(t, db, "caller"))
	assert.Empty(t, defaultBillingAddressIDs(t, db, "someone-else"))
}

func TestSetDefaultBillingAddress_UnknownID_ReturnsNotFoundAndKeepsDefault(t *testing.T) {
	db := freshTestDB(t)
	repo := repositories.NewPaymentRepository(db)
	current, _ := setupDefaultBillingAddress(t, db, "caller")

	err := repo.SetDefaultBillingAddress("caller", uuid.New())

	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
	assert.Equal(t, []uuid.UUID{current.ID}, defaultBillingAddressIDs(t, db, "caller"))
}

func TestSetDefaultBillingAddress_OwnID_MovesTheDefault(t *testing.T) {
	db := freshTestDB(t)
	repo := repositories.NewPaymentRepository(db)
	_, other := setupDefaultBillingAddress(t, db, "caller")

	require.NoError(t, repo.SetDefaultBillingAddress("caller", other.ID))

	assert.Equal(t, []uuid.UUID{other.ID}, defaultBillingAddressIDs(t, db, "caller"))
}

func TestSetDefaultPaymentMethod_ForeignID_ReturnsNotFoundAndKeepsDefault(t *testing.T) {
	db := freshTestDB(t)
	repo := repositories.NewPaymentRepository(db)
	current, _ := setupDefaultPaymentMethod(t, db, "caller")
	foreign := createTestPaymentMethod(t, "someone-else")

	err := repo.SetDefaultPaymentMethod("caller", foreign.ID)

	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
	assert.Equal(t, []uuid.UUID{current.ID}, defaultPaymentMethodIDs(t, db, "caller"))
	assert.Empty(t, defaultPaymentMethodIDs(t, db, "someone-else"))
}

func TestSetDefaultPaymentMethod_UnknownID_ReturnsNotFoundAndKeepsDefault(t *testing.T) {
	db := freshTestDB(t)
	repo := repositories.NewPaymentRepository(db)
	current, _ := setupDefaultPaymentMethod(t, db, "caller")

	err := repo.SetDefaultPaymentMethod("caller", uuid.New())

	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
	assert.Equal(t, []uuid.UUID{current.ID}, defaultPaymentMethodIDs(t, db, "caller"))
}

func TestSetDefaultPaymentMethod_OwnID_MovesTheDefault(t *testing.T) {
	db := freshTestDB(t)
	repo := repositories.NewPaymentRepository(db)
	_, other := setupDefaultPaymentMethod(t, db, "caller")

	require.NoError(t, repo.SetDefaultPaymentMethod("caller", other.ID))

	assert.Equal(t, []uuid.UUID{other.ID}, defaultPaymentMethodIDs(t, db, "caller"))
}

func TestSetDefaultBillingAddress_AlreadyDefault_StaysSingleDefault(t *testing.T) {
	db := freshTestDB(t)
	repo := repositories.NewPaymentRepository(db)
	current, _ := setupDefaultBillingAddress(t, db, "caller")

	require.NoError(t, repo.SetDefaultBillingAddress("caller", current.ID))

	assert.Equal(t, []uuid.UUID{current.ID}, defaultBillingAddressIDs(t, db, "caller"))
}

func TestSetDefaultPaymentMethod_AlreadyDefault_StaysSingleDefault(t *testing.T) {
	db := freshTestDB(t)
	repo := repositories.NewPaymentRepository(db)
	current, _ := setupDefaultPaymentMethod(t, db, "caller")

	require.NoError(t, repo.SetDefaultPaymentMethod("caller", current.ID))

	assert.Equal(t, []uuid.UUID{current.ID}, defaultPaymentMethodIDs(t, db, "caller"))
}

func TestSetDefaultBillingAddress_SoftDeletedID_ReturnsNotFoundAndKeepsDefault(t *testing.T) {
	db := freshTestDB(t)
	repo := repositories.NewPaymentRepository(db)
	current, other := setupDefaultBillingAddress(t, db, "caller")
	require.NoError(t, db.Delete(other).Error)

	err := repo.SetDefaultBillingAddress("caller", other.ID)

	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
	assert.Equal(t, []uuid.UUID{current.ID}, defaultBillingAddressIDs(t, db, "caller"))
}

func TestSetDefaultBillingAddress_CallerWithoutRows_ReturnsNotFound(t *testing.T) {
	db := freshTestDB(t)
	repo := repositories.NewPaymentRepository(db)
	foreign := createTestBillingAddress(t, "someone-else")

	err := repo.SetDefaultBillingAddress("caller", foreign.ID)

	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
	assert.Empty(t, defaultBillingAddressIDs(t, db, "someone-else"))
}

// setDefaultRouter mounts the two real set-default handlers with the caller id in the context.
func setDefaultRouter(db *gorm.DB, userID string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("userId", userID); c.Next() })
	r.POST("/billing-addresses/:id/set-default", paymentController.NewBillingAddressController(db).SetDefaultBillingAddress)
	r.POST("/payment-methods/:id/set-default", paymentController.NewPaymentMethodController(db).SetDefaultPaymentMethod)
	return r
}

func postSetDefault(r *gin.Engine, path string) int {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, nil))
	return w.Code
}

func TestSetDefaultBillingAddressHandler_Answers(t *testing.T) {
	db := freshTestDB(t)
	r := setDefaultRouter(db, "caller")
	current, other := setupDefaultBillingAddress(t, db, "caller")
	foreign := createTestBillingAddress(t, "someone-else")

	assert.Equal(t, http.StatusNotFound, postSetDefault(r, "/billing-addresses/"+foreign.ID.String()+"/set-default"))
	assert.Equal(t, []uuid.UUID{current.ID}, defaultBillingAddressIDs(t, db, "caller"))

	assert.Equal(t, http.StatusBadRequest, postSetDefault(r, "/billing-addresses/not-a-uuid/set-default"))

	assert.Equal(t, http.StatusOK, postSetDefault(r, "/billing-addresses/"+other.ID.String()+"/set-default"))
	assert.Equal(t, []uuid.UUID{other.ID}, defaultBillingAddressIDs(t, db, "caller"))
}

func TestSetDefaultPaymentMethodHandler_Answers(t *testing.T) {
	db := freshTestDB(t)
	r := setDefaultRouter(db, "caller")
	current, other := setupDefaultPaymentMethod(t, db, "caller")
	foreign := createTestPaymentMethod(t, "someone-else")

	assert.Equal(t, http.StatusNotFound, postSetDefault(r, "/payment-methods/"+foreign.ID.String()+"/set-default"))
	assert.Equal(t, []uuid.UUID{current.ID}, defaultPaymentMethodIDs(t, db, "caller"))

	assert.Equal(t, http.StatusBadRequest, postSetDefault(r, "/payment-methods/not-a-uuid/set-default"))

	assert.Equal(t, http.StatusOK, postSetDefault(r, "/payment-methods/"+other.ID.String()+"/set-default"))
	assert.Equal(t, []uuid.UUID{other.ID}, defaultPaymentMethodIDs(t, db, "caller"))
}

// fakeStripeCustomerDefault serves the Stripe calls behind set-default and the payment-method
// sync for one customer: it records customer updates and keeps the customer's default.
type fakeStripeCustomerDefault struct {
	mu         sync.Mutex
	customerID string
	defaultPM  string
	pmIDs      []string
	failUpdate bool
	onUpdate   func()   // runs when a customer update arrives, before it is applied
	updatedTo  []string // invoice_settings[default_payment_method] of each customer update
}

func (f *fakeStripeCustomerDefault) updates() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.updatedTo...)
}

func installFakeStripeCustomerDefault(t *testing.T, f *fakeStripeCustomerDefault) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		f.mu.Lock()
		defer f.mu.Unlock()
		switch {
		case r.URL.Path == "/v1/customers/"+f.customerID && r.Method == http.MethodPost:
			if f.onUpdate != nil {
				f.onUpdate()
			}
			if f.failUpdate {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprint(w, `{"error":{"message":"injected customer update failure","type":"api_error"}}`)
				return
			}
			_ = r.ParseForm()
			f.defaultPM = r.PostForm.Get("invoice_settings[default_payment_method]")
			f.updatedTo = append(f.updatedTo, f.defaultPM)
			fmt.Fprintf(w, `{"id":%q,"object":"customer","invoice_settings":{"default_payment_method":%q}}`, f.customerID, f.defaultPM)
		case r.URL.Path == "/v1/customers/"+f.customerID:
			fmt.Fprintf(w, `{"id":%q,"object":"customer","invoice_settings":{"default_payment_method":%q}}`, f.customerID, f.defaultPM)
		case r.URL.Path == "/v1/payment_methods":
			items := make([]string, 0, len(f.pmIDs))
			for _, id := range f.pmIDs {
				items = append(items, fmt.Sprintf(`{"id":%q,"object":"payment_method","type":"card","card":{"brand":"visa","last4":"4242","exp_month":12,"exp_year":2030}}`, id))
			}
			fmt.Fprintf(w, `{"object":"list","url":"/v1/payment_methods","has_more":false,"data":[%s]}`, strings.Join(items, ","))
		default:
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintf(w, `{"error":{"message":"unexpected %s %s","type":"invalid_request_error"}}`, r.Method, r.URL.Path)
		}
	}))

	prevBackend := stripe.GetBackend(stripe.APIBackend)
	prevKey := stripe.Key
	// NewStripeService reads the key from the environment on construction.
	t.Setenv("STRIPE_SECRET_KEY", "sk_test_fake_set_default")
	stripe.SetBackend(stripe.APIBackend, stripe.GetBackendWithConfig(stripe.APIBackend, &stripe.BackendConfig{
		URL: stripe.String(srv.URL),
	}))
	stripe.Key = "sk_test_fake_set_default"
	t.Cleanup(func() {
		srv.Close()
		stripe.SetBackend(stripe.APIBackend, prevBackend)
		stripe.Key = prevKey
	})
}

// setupStripeCustomerWithDefault gives the caller a Stripe customer (on an active subscription)
// whose default is the caller's current local default.
func setupStripeCustomerWithDefault(t *testing.T, db *gorm.DB, userID string) (current, other *models.PaymentMethod, fake *fakeStripeCustomerDefault) {
	t.Helper()
	current, other = setupDefaultPaymentMethod(t, db, userID)
	sub := createTestUserSubscription(t, userID, createTestSubscriptionPlan(t))
	require.NoError(t, db.Model(sub).Update("stripe_customer_id", "cus_test_caller").Error)
	fake = &fakeStripeCustomerDefault{
		customerID: "cus_test_caller",
		defaultPM:  current.StripePaymentMethodID,
		pmIDs:      []string{current.StripePaymentMethodID, other.StripePaymentMethodID},
	}
	installFakeStripeCustomerDefault(t, fake)
	return current, other, fake
}

func TestSetDefaultPaymentMethodHandler_StripeCustomer_SetsStripeDefault(t *testing.T) {
	db := freshTestDB(t)
	current, other, fake := setupStripeCustomerWithDefault(t, db, "caller")
	var localDefaultAtStripeCall []uuid.UUID
	fake.onUpdate = func() {
		db.Model(&models.PaymentMethod{}).Where("user_id = ? AND is_default = ?", "caller", true).Pluck("id", &localDefaultAtStripeCall)
	}
	r := setDefaultRouter(db, "caller")

	assert.Equal(t, http.StatusOK, postSetDefault(r, "/payment-methods/"+other.ID.String()+"/set-default"))

	assert.Equal(t, []string{other.StripePaymentMethodID}, fake.updates(),
		"the Stripe customer's invoice_settings.default_payment_method must be set to the chosen method")
	assert.Equal(t, []uuid.UUID{current.ID}, localDefaultAtStripeCall, "Stripe must be called before the local rows change")
	assert.Equal(t, []uuid.UUID{other.ID}, defaultPaymentMethodIDs(t, db, "caller"))
}

func TestSetDefaultPaymentMethodHandler_ThenSync_KeepsChosenDefault(t *testing.T) {
	db := freshTestDB(t)
	_, other, _ := setupStripeCustomerWithDefault(t, db, "caller")
	r := setDefaultRouter(db, "caller")

	require.Equal(t, http.StatusOK, postSetDefault(r, "/payment-methods/"+other.ID.String()+"/set-default"))
	result, err := services.NewStripeService(db).SyncUserPaymentMethods("caller")
	require.NoError(t, err)
	require.Empty(t, result.FailedPaymentMethods)

	assert.Equal(t, []uuid.UUID{other.ID}, defaultPaymentMethodIDs(t, db, "caller"),
		"the sync after set-default must not flip the default back")
}

func TestSetDefaultPaymentMethodHandler_StripeFails_ErrorAndKeepsDefault(t *testing.T) {
	db := freshTestDB(t)
	current, other, fake := setupStripeCustomerWithDefault(t, db, "caller")
	fake.failUpdate = true
	r := setDefaultRouter(db, "caller")

	code := postSetDefault(r, "/payment-methods/"+other.ID.String()+"/set-default")

	assert.GreaterOrEqual(t, code, http.StatusInternalServerError)
	assert.Equal(t, []uuid.UUID{current.ID}, defaultPaymentMethodIDs(t, db, "caller"))
}

func TestSetDefaultPaymentMethodHandler_StripeCustomer_ForeignID_NotFoundAndNothingSent(t *testing.T) {
	db := freshTestDB(t)
	current, _, fake := setupStripeCustomerWithDefault(t, db, "caller")
	foreign := createTestPaymentMethod(t, "someone-else")
	r := setDefaultRouter(db, "caller")

	assert.Equal(t, http.StatusNotFound, postSetDefault(r, "/payment-methods/"+foreign.ID.String()+"/set-default"))

	assert.Empty(t, fake.updates())
	assert.Equal(t, []uuid.UUID{current.ID}, defaultPaymentMethodIDs(t, db, "caller"))
}

func TestSetDefaultPaymentMethodHandler_StripeCustomer_NoStripeID_ConflictAndNothingSent(t *testing.T) {
	db := freshTestDB(t)
	current, _, fake := setupStripeCustomerWithDefault(t, db, "caller")
	localOnly := createTestPaymentMethod(t, "caller")
	require.NoError(t, db.Model(localOnly).Update("stripe_payment_method_id", "").Error)
	r := setDefaultRouter(db, "caller")

	assert.Equal(t, http.StatusConflict, postSetDefault(r, "/payment-methods/"+localOnly.ID.String()+"/set-default"))

	assert.Empty(t, fake.updates())
	assert.Equal(t, []uuid.UUID{current.ID}, defaultPaymentMethodIDs(t, db, "caller"))
}

// A row turns inactive when Stripe reports it detached (handlePaymentMethodDetached), so it can
// no longer be the customer's default.
func TestSetDefaultPaymentMethodHandler_InactiveRow_NotFoundAndNothingSent(t *testing.T) {
	db := freshTestDB(t)
	current, other, fake := setupStripeCustomerWithDefault(t, db, "caller")
	require.NoError(t, db.Model(other).Update("is_active", false).Error)
	r := setDefaultRouter(db, "caller")

	assert.Equal(t, http.StatusNotFound, postSetDefault(r, "/payment-methods/"+other.ID.String()+"/set-default"))

	assert.Empty(t, fake.updates())
	assert.Equal(t, []uuid.UUID{current.ID}, defaultPaymentMethodIDs(t, db, "caller"))
}

func TestSetDefaultPaymentMethodHandler_EmptyCustomerID_LocalOnly(t *testing.T) {
	db := freshTestDB(t)
	_, other, fake := setupStripeCustomerWithDefault(t, db, "caller")
	require.NoError(t, db.Model(&models.UserSubscription{}).Where("user_id = ?", "caller").Update("stripe_customer_id", "").Error)
	r := setDefaultRouter(db, "caller")

	assert.Equal(t, http.StatusOK, postSetDefault(r, "/payment-methods/"+other.ID.String()+"/set-default"))

	assert.Empty(t, fake.updates())
	assert.Equal(t, []uuid.UUID{other.ID}, defaultPaymentMethodIDs(t, db, "caller"))
}

// assignLicence turns the caller's subscription into a batch licence bought by purchaser; the
// licence keeps the purchaser's Stripe customer (bulkLicenseService creates it that way).
func assignLicence(t *testing.T, db *gorm.DB, userID, purchaser string) {
	t.Helper()
	require.NoError(t, db.Model(&models.UserSubscription{}).Where("user_id = ?", userID).
		Updates(map[string]any{"purchaser_user_id": purchaser, "subscription_type": "assigned"}).Error)
}

func TestSetDefaultPaymentMethodHandler_LicenceFromAnotherPurchaser_LocalOnly(t *testing.T) {
	db := freshTestDB(t)
	_, other, fake := setupStripeCustomerWithDefault(t, db, "caller")
	assignLicence(t, db, "caller", "purchaser")
	r := setDefaultRouter(db, "caller")

	assert.Equal(t, http.StatusOK, postSetDefault(r, "/payment-methods/"+other.ID.String()+"/set-default"))

	assert.Empty(t, fake.updates(), "a licence holder must not change the purchaser's Stripe customer default")
	assert.Equal(t, []uuid.UUID{other.ID}, defaultPaymentMethodIDs(t, db, "caller"))
}

func TestSetDefaultPaymentMethodHandler_SelfPurchased_SetsStripeDefault(t *testing.T) {
	db := freshTestDB(t)
	_, other, fake := setupStripeCustomerWithDefault(t, db, "caller")
	assignLicence(t, db, "caller", "caller")
	r := setDefaultRouter(db, "caller")

	assert.Equal(t, http.StatusOK, postSetDefault(r, "/payment-methods/"+other.ID.String()+"/set-default"))

	assert.Equal(t, []string{other.StripePaymentMethodID}, fake.updates())
	assert.Equal(t, []uuid.UUID{other.ID}, defaultPaymentMethodIDs(t, db, "caller"))
}
