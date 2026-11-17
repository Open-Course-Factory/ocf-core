package payment_tests

import (
	"net/http"
	"testing"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	authServices "soli/formations/src/auth/services"
	paymentController "soli/formations/src/payment/routes"
)

// setupPaymentTestRouter mounts the real UserSubscriptionRoutes behind the real
// AuthManagement (see installRealGateway). Casdoor knows every caller as an
// account whose email is not verified. gatewayMember holds a subscription:
// without one, GET /current asks Stripe for it.
func setupPaymentTestRouter(t *testing.T) *gin.Engine {
	t.Helper()

	gin.SetMode(gin.TestMode)

	db := freshTestDB(t)
	installRealGateway(t)
	createUserSubscription(t, db, gatewayMember, seedAnyPlan(t, db))
	t.Cleanup(authServices.SwapCasdoorUserReader(func(userID string) (*casdoorsdk.User, error) {
		return &casdoorsdk.User{Id: userID, EmailVerified: false}, nil
	}))

	router := gin.New()
	paymentController.UserSubscriptionRoutes(router.Group("/api/v1"), db)

	return router
}

// TestUnverifiedUser_CanReadSubscriptionInfo verifies that read-only subscription
// endpoints do NOT require email verification.
func TestUnverifiedUser_CanReadSubscriptionInfo(t *testing.T) {
	router := setupPaymentTestRouter(t)

	readEndpoints := []struct {
		caller string
		path   string
	}{
		{gatewayMember, "/api/v1/user-subscriptions/current"},
		{gatewayMember, "/api/v1/user-subscriptions/all"},
		{gatewayMember, "/api/v1/user-subscriptions/usage"},
		{gatewayAdmin, "/api/v1/user-subscriptions/analytics"},
	}

	for _, ep := range readEndpoints {
		t.Run("GET "+ep.path, func(t *testing.T) {
			w := requestAs(router, ep.caller, "GET", ep.path, "")

			assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
			assert.NotContains(t, w.Body.String(), "EMAIL_NOT_VERIFIED",
				"Read endpoint %s should not return EMAIL_NOT_VERIFIED", ep.path)
		})
	}
}

// TestUnverifiedUser_BlockedFromPaymentActions verifies that payment action
// endpoints DO require email verification.
func TestUnverifiedUser_BlockedFromPaymentActions(t *testing.T) {
	router := setupPaymentTestRouter(t)

	paymentEndpoints := []string{
		"/api/v1/user-subscriptions/checkout",
		"/api/v1/user-subscriptions/portal",
		"/api/v1/user-subscriptions/upgrade",
	}

	for _, path := range paymentEndpoints {
		t.Run("POST "+path, func(t *testing.T) {
			w := requestAs(router, gatewayMember, "POST", path, "")

			assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
			assert.Contains(t, w.Body.String(), "EMAIL_NOT_VERIFIED")
		})
	}
}
