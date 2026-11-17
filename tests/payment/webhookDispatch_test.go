// tests/payment/webhookDispatch_test.go
//
// What each Stripe event type changes once the webhook route accepted it. Every
// case sends a signed event to the real controller and reads the database.
package payment_tests

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"soli/formations/src/payment/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestStripeWebhook_EventType_AppliesItsEffect(t *testing.T) {
	const secret = "whsec_test"
	const userID = "user_dispatch"

	subscriptionEvent := func(status string) func(*testing.T, *gorm.DB, *models.UserSubscription) string {
		return func(_ *testing.T, _ *gorm.DB, sub *models.UserSubscription) string {
			return stripeSubscriptionJSON(*sub.StripeSubscriptionID, *sub.StripeCustomerID, status, nil)
		}
	}
	nothingChanged := func(t *testing.T, db *gorm.DB, sub *models.UserSubscription) {
		assert.Equal(t, "active", userSubscriptionStatus(t, db, sub.ID))
		var paymentMethods int64
		require.NoError(t, db.Model(&models.PaymentMethod{}).Count(&paymentMethods).Error)
		assert.Zero(t, paymentMethods)
	}

	// Each case starts from one active subscription of userID. object seeds
	// what the case needs on top and returns the Stripe object of the event.
	cases := []struct {
		name      string
		eventType string
		object    func(t *testing.T, db *gorm.DB, sub *models.UserSubscription) string
		check     func(t *testing.T, db *gorm.DB, sub *models.UserSubscription)
	}{
		{
			name:      "subscription deleted cancels the subscription",
			eventType: "customer.subscription.deleted",
			object:    subscriptionEvent("canceled"),
			check: func(t *testing.T, db *gorm.DB, sub *models.UserSubscription) {
				var reloaded models.UserSubscription
				require.NoError(t, db.First(&reloaded, "id = ?", sub.ID).Error)
				assert.Equal(t, "cancelled", reloaded.Status)
				assert.NotNil(t, reloaded.CancelledAt)
			},
		},
		{
			name:      "subscription paused pauses the subscription",
			eventType: "customer.subscription.paused",
			object:    subscriptionEvent("paused"),
			check: func(t *testing.T, db *gorm.DB, sub *models.UserSubscription) {
				assert.Equal(t, "paused", userSubscriptionStatus(t, db, sub.ID))
			},
		},
		{
			name:      "subscription resumed takes the status Stripe reports",
			eventType: "customer.subscription.resumed",
			object: func(t *testing.T, db *gorm.DB, sub *models.UserSubscription) string {
				require.NoError(t, db.Model(sub).Update("status", "paused").Error)
				// Not "active": a handler that hard-codes it must fail here.
				return stripeSubscriptionJSON(*sub.StripeSubscriptionID, *sub.StripeCustomerID, "trialing", nil)
			},
			check: func(t *testing.T, db *gorm.DB, sub *models.UserSubscription) {
				assert.Equal(t, "trialing", userSubscriptionStatus(t, db, sub.ID))
			},
		},
		{
			name:      "payment method attached is recorded for the customer's user",
			eventType: "payment_method.attached",
			object: func(t *testing.T, db *gorm.DB, sub *models.UserSubscription) string {
				// Another customer's subscription, sorted first, so a lookup that
				// ignores the customer id picks the wrong user.
				other := seedUpgradeSubscription(t, db, "user_other", seedPlan(t, db, models.SubscriptionPlan{Name: "Other plan"}))
				require.NoError(t, db.Model(other).Update("id", uuid.Nil).Error)
				return fmt.Sprintf(`{"id":"pm_attached","object":"payment_method","type":"card","customer":%q,`+
					`"card":{"brand":"visa","last4":"4242","exp_month":12,"exp_year":2030}}`, *sub.StripeCustomerID)
			},
			check: func(t *testing.T, db *gorm.DB, _ *models.UserSubscription) {
				var pm models.PaymentMethod
				require.NoError(t, db.First(&pm, "stripe_payment_method_id = ?", "pm_attached").Error)
				assert.Equal(t, userID, pm.UserID)
				assert.Equal(t, "4242", pm.CardLast4)
				assert.True(t, pm.IsActive)
			},
		},
		{
			name:      "payment method detached is kept but made inactive",
			eventType: "payment_method.detached",
			object: func(t *testing.T, _ *gorm.DB, _ *models.UserSubscription) string {
				pm := createTestPaymentMethod(t, userID)
				return fmt.Sprintf(`{"id":%q,"object":"payment_method","type":"card"}`, pm.StripePaymentMethodID)
			},
			check: func(t *testing.T, db *gorm.DB, _ *models.UserSubscription) {
				var pm models.PaymentMethod
				require.NoError(t, db.First(&pm, "user_id = ?", userID).Error)
				assert.False(t, pm.IsActive)
			},
		},
		{
			// The handler only rewrites a customer id that already matches.
			name:      "customer updated is acknowledged and changes nothing",
			eventType: "customer.updated",
			object: func(_ *testing.T, _ *gorm.DB, sub *models.UserSubscription) string {
				return fmt.Sprintf(`{"id":%q,"object":"customer","email":"renamed@example.test"}`, *sub.StripeCustomerID)
			},
			check: nothingChanged,
		},
		{
			name:      "unsupported event type is acknowledged and changes nothing",
			eventType: "product.created",
			object: func(*testing.T, *gorm.DB, *models.UserSubscription) string {
				return `{"id":"prod_unsupported","object":"product"}`
			},
			check: nothingChanged,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := freshTestDB(t)
			installStrictStripeSubscriptions(t, nil)
			router := newRouterWithRealService(t, db, secret)
			sub := seedUpgradeSubscription(t, db, userID, seedPlan(t, db, models.SubscriptionPlan{Name: "Dispatch plan"}))

			eventID := "evt_dispatch_" + uuid.NewString()
			payload := stripeEventPayload(eventID, tc.eventType, tc.object(t, db, sub))
			w := httptest.NewRecorder()
			router.ServeHTTP(w, buildSignedWebhookRequest(t, payload, secret))

			require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
			assert.Equal(t, models.WebhookEventStatusProcessed, fetchWebhookEventStatus(t, db, eventID))
			tc.check(t, db, sub)
		})
	}
}
