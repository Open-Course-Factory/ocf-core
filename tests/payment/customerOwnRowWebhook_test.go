// tests/payment/customerOwnRowWebhook_test.go
package payment_tests

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v85"
	"gorm.io/gorm"

	authModels "soli/formations/src/auth/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementModels "soli/formations/src/entityManagement/models"
	registration "soli/formations/src/payment/entityRegistration"
	"soli/formations/src/payment/models"
)

// purchaserWithLicences seeds one Stripe customer shared by the purchaser's own subscription, a
// licence assigned to a learner and an unassigned licence (bulkLicenseService copies the
// purchaser's customer onto licence rows). The licence gets the lowest id, so an unordered
// First() by primary key picks it.
func purchaserWithLicences(t *testing.T, db *gorm.DB) (customerID string, own, assigned *models.UserSubscription) {
	t.Helper()
	plan := createTestSubscriptionPlan(t)
	customerID = "cus_buyer_" + uuid.NewString()
	purchaser := "purchaser-" + uuid.NewString()
	ownStripeSub := "sub_own_" + uuid.NewString()
	row := func(id, userID string, purchaserID *string, status, subType string, stripeSub *string) *models.UserSubscription {
		s := &models.UserSubscription{
			BaseModel: entityManagementModels.BaseModel{ID: uuid.MustParse(id)},
			UserID:    userID, PurchaserUserID: purchaserID, SubscriptionPlanID: plan.ID,
			SubscriptionType: subType, StripeCustomerID: &customerID, StripeSubscriptionID: stripeSub,
			Status: status, CurrentPeriodStart: time.Now(), CurrentPeriodEnd: time.Now().Add(30 * 24 * time.Hour),
		}
		require.NoError(t, db.Create(s).Error)
		return s
	}
	assigned = row("00000000-0000-4000-8000-"+uuid.NewString()[24:], "learner-"+uuid.NewString(), &purchaser, "active", "assigned", nil)
	row("00000001-0000-4000-8000-"+uuid.NewString()[24:], "", &purchaser, "unassigned", "assigned", nil)
	own = row("ffffffff-0000-4000-8000-"+uuid.NewString()[24:], purchaser, nil, "active", "personal", &ownStripeSub)
	return customerID, own, assigned
}

func paymentMethodAttachedWebhook(eventID, pmID, customerID string) []byte {
	return []byte(fmt.Sprintf(`{
		"id": %q, "object": "event", "api_version": %q, "type": "payment_method.attached", "created": %d,
		"data": {"object": {"id": %q, "object": "payment_method", "type": "card", "customer": %q,
			"card": {"brand": "visa", "last4": "4242", "exp_month": 12, "exp_year": 2030}}}
	}`, eventID, stripe.APIVersion, time.Now().Unix(), pmID, customerID))
}

func TestCustomerWebhook_PaymentMethodAttached_SharedCustomer_StoresCardForPurchaser(t *testing.T) {
	db := freshTestDB(t)
	secret := "whsec_own_row_" + uuid.NewString()
	router := newRouterWithRealService(t, db, secret)
	customerID, own, assigned := purchaserWithLicences(t, db)
	pmID := "pm_buyer_" + uuid.NewString()

	w := httptest.NewRecorder()
	router.ServeHTTP(w, buildSignedWebhookRequest(t, paymentMethodAttachedWebhook("evt_pma_"+uuid.NewString(), pmID, customerID), secret))
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var stored models.PaymentMethod
	require.NoError(t, db.Where("stripe_payment_method_id = ?", pmID).First(&stored).Error)
	assert.Equal(t, own.UserID, stored.UserID, "the card belongs to the customer's buyer, not to a licence holder")
	assert.Zero(t, countRows(t, db, &models.PaymentMethod{}, assigned.UserID))
}

func TestCustomerWebhook_InvoicePaymentFailed_SharedCustomer_MarksPurchasersOwnRow(t *testing.T) {
	db := freshTestDB(t)
	secret := "whsec_own_row_" + uuid.NewString()
	router := newRouterWithRealService(t, db, secret)
	customerID, own, assigned := purchaserWithLicences(t, db)

	payload := buildInvoiceWebhook("evt_ipf_"+uuid.NewString(), "invoice.payment_failed", "in_"+uuid.NewString(), customerID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, buildSignedWebhookRequest(t, payload, secret))
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var ownAfter, assignedAfter models.UserSubscription
	require.NoError(t, db.First(&ownAfter, "id = ?", own.ID).Error)
	require.NoError(t, db.First(&assignedAfter, "id = ?", assigned.ID).Error)
	assert.Equal(t, "past_due", ownAfter.Status, "the buyer's own subscription is the one whose payment failed")
	assert.Equal(t, "active", assignedAfter.Status, "a learner's licence must not be marked past_due")
}

// Payment-method rows come from the Stripe sync and webhooks only: a client-supplied
// stripe_payment_method_id is never verified, so the generic create route must not exist.
// The front deletes cards through the generic DELETE, which stays.
func TestPaymentMethodRegistration_NoGenericCreateOrPatchForMembers(t *testing.T) {
	svc := ems.NewEntityRegistrationService()
	registration.RegisterPaymentMethod(svc)

	cfg := svc.GetSwaggerConfig("PaymentMethod")
	require.NotNil(t, cfg)
	assert.Nil(t, cfg.Create, "POST /payment-methods must not be mounted")
	assert.Nil(t, cfg.Update, "PATCH /payment-methods/:id must not be mounted")
	assert.NotNil(t, cfg.Delete, "the front removes a card through DELETE /payment-methods/:id")

	member := svc.GetAllEntityRoles()["PaymentMethod"].Roles[string(authModels.Member)]
	assert.NotContains(t, member, http.MethodPost)
	assert.NotContains(t, member, http.MethodPatch)
	assert.True(t, strings.Contains(member, http.MethodDelete), "member keeps DELETE, got %q", member)
}
