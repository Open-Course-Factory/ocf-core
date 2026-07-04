// tests/auth/rgpdCustomerErasureOrchestration_test.go
//
// Orchestration GUARD for issue #370 / MR !272: the Stripe Customer erasure step
// (RGPD Art. 17) must actually fire through the REAL user-deletion flow
// (userService.DeleteUser), not just when the helper method is called in
// isolation. This pins the wiring so a future refactor of DeleteUser cannot
// silently drop the erasure step.
//
// It drives userService.DeleteUser with the REAL PaymentDeletionHelper (Stripe
// cancellation stubbed via stubStripeService) and a fake Stripe HTTP backend.
// DeleteStripeCustomersForUser deletes via the package-level Stripe client
// (customer.Del), so the fake backend — not the injected stub — captures the
// DELETE. Shared helpers (mockCasdoorUserClient, buildCasdoorUser, newUserID,
// newCallRecorder, anyArg, stubStripeService, setupDeleteMyAccountDB) come from
// userDeletion_test.go / deleteMyAccount_test.go in this package.
//
// GREEN today (the wiring exists as of MR !272). Kept as a regression guard.
package auth_tests

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	entityManagementModels "soli/formations/src/entityManagement/models"
	authServices "soli/formations/src/auth/services"
	paymentModels "soli/formations/src/payment/models"
	paymentServices "soli/formations/src/payment/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v85"
)

// authCustomerDeleteCapture records DELETE /v1/customers/{id} requests reaching
// the fake Stripe backend.
type authCustomerDeleteCapture struct {
	mu      sync.Mutex
	deleted []string
}

func (c *authCustomerDeleteCapture) has(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, got := range c.deleted {
		if got == id {
			return true
		}
	}
	return false
}

// installAuthCustomerDeleteBackend points the global stripe backend at a fake
// server capturing customer DELETEs; globals are restored on cleanup.
func installAuthCustomerDeleteBackend(t *testing.T) *authCustomerDeleteCapture {
	t.Helper()
	cap := &authCustomerDeleteCapture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/v1/customers/") {
			id := strings.TrimPrefix(r.URL.Path, "/v1/customers/")
			cap.mu.Lock()
			cap.deleted = append(cap.deleted, id)
			cap.mu.Unlock()
			_, _ = w.Write([]byte(`{"id":"` + id + `","object":"customer","deleted":true}`))
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))

	prevBackend := stripe.GetBackend(stripe.APIBackend)
	prevKey := stripe.Key
	stripe.SetBackend(stripe.APIBackend, stripe.GetBackendWithConfig(stripe.APIBackend, &stripe.BackendConfig{
		URL: stripe.String(srv.URL),
	}))
	stripe.Key = "sk_test_rgpd_orch"
	t.Cleanup(func() {
		srv.Close()
		stripe.SetBackend(stripe.APIBackend, prevBackend)
		stripe.Key = prevKey
	})
	return cap
}

// TestDeleteUser_Orchestration_ErasesStripeCustomer verifies the erasure step
// fires end-to-end through userService.DeleteUser.
func TestDeleteUser_Orchestration_ErasesStripeCustomer(t *testing.T) {
	db := setupDeleteMyAccountDB(t)
	userID := newUserID()

	// A self-owned active subscription carrying a Stripe customer id.
	stripeSubID := "sub_orch_" + uuid.NewString()
	customerID := "cus_orch_" + uuid.NewString()
	require.NoError(t, db.Create(&paymentModels.UserSubscription{
		BaseModel:            entityManagementModels.BaseModel{ID: uuid.New()},
		UserID:               userID,
		SubscriptionPlanID:   uuid.New(),
		Status:               "active",
		StripeSubscriptionID: &stripeSubID,
		StripeCustomerID:     &customerID,
		CurrentPeriodStart:   time.Now(),
		CurrentPeriodEnd:     time.Now().Add(30 * 24 * time.Hour),
	}).Error)

	rec := newCallRecorder()
	casdoorMock := &mockCasdoorUserClient{recorder: rec}
	casdoorMock.On("GetUserByUserId", userID).Return(buildCasdoorUser(userID), nil)
	casdoorMock.On("DeleteUser", anyArg()).Return(true, nil)

	// Real helper (Stripe cancellation stubbed). The customer DELETE goes to the
	// fake backend below via the package-level Stripe client.
	realHelper := paymentServices.NewPaymentDeletionHelperWithDeps(db, &stubStripeService{})
	userSvc := authServices.NewUserService(casdoorMock, realHelper)

	cap := installAuthCustomerDeleteBackend(t)

	require.NoError(t, userSvc.DeleteUser(userID))

	assert.True(t, cap.has(customerID),
		"WIRING: userService.DeleteUser must invoke the Stripe customer erasure "+
			"step — DELETE /v1/customers/%s should have reached Stripe. If this "+
			"fails, the RGPD erasure step was dropped from the orchestration.", customerID)
}
