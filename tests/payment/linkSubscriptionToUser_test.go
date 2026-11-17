// tests/payment/linkSubscriptionToUser_test.go
//
// The admin route that attaches an existing Stripe subscription to a user,
// through the real controller and service.
package payment_tests

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"soli/formations/src/payment/models"
	paymentController "soli/formations/src/payment/routes"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLinkSubscriptionToUser_AdminRequest_StoresSubscriptionForNamedUser(t *testing.T) {
	const stripeSubID = "sub_to_link"

	cases := []struct {
		name            string
		userIDField     string // "user_id" JSON member of the request, or empty
		alreadyLinkedTo string // user who already holds the Stripe subscription, if any
		wantStatus      int
		wantOwner       string // owner of the Stripe subscription afterwards, empty = not stored
	}{
		{name: "unlinked subscription goes to the named user", userIDField: `"user_id":"user_named",`, wantStatus: http.StatusOK, wantOwner: "user_named"},
		{name: "already linked subscription stays with its owner", userIDField: `"user_id":"user_named",`, alreadyLinkedTo: "user_first", wantStatus: http.StatusInternalServerError, wantOwner: "user_first"},
		{name: "request without a user is refused", wantStatus: http.StatusBadRequest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := freshTestDB(t)
			plan := seedPlan(t, db, models.SubscriptionPlan{Name: "Link plan"})
			if tc.alreadyLinkedTo != "" {
				held := seedUpgradeSubscription(t, db, tc.alreadyLinkedTo, plan)
				require.NoError(t, db.Model(held).Update("stripe_subscription_id", stripeSubID).Error)
			}
			installStrictStripeSubscriptions(t, map[string]string{
				stripeSubID: stripeSubscriptionJSON(stripeSubID, "cus_to_link", "trialing", nil),
			})
			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.POST("/user-subscriptions/link/:subscription_id", paymentController.NewSubscriptionController(db).LinkSubscriptionToUser)

			body := fmt.Sprintf(`{%s"subscription_plan_id":%q}`, tc.userIDField, plan.ID)
			req := httptest.NewRequest(http.MethodPost, "/user-subscriptions/link/"+stripeSubID, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			require.Equal(t, tc.wantStatus, w.Code, "body: %s", w.Body.String())
			var stored []models.UserSubscription
			require.NoError(t, db.Find(&stored, "stripe_subscription_id = ?", stripeSubID).Error)
			if tc.wantOwner == "" {
				assert.Empty(t, stored)
				return
			}
			require.Len(t, stored, 1)
			assert.Equal(t, tc.wantOwner, stored[0].UserID)
			if tc.wantStatus == http.StatusOK {
				assert.Equal(t, plan.ID, stored[0].SubscriptionPlanID)
				assert.Equal(t, "cus_to_link", *stored[0].StripeCustomerID)
				assert.Equal(t, "trialing", stored[0].Status)
				assert.Equal(t, fakePeriodEnd, stored[0].CurrentPeriodEnd.Unix())
			}
		})
	}
}
