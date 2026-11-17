// tests/payment/webhookSecretRequired_test.go
//
// A server with no Stripe webhook secret cannot tell a genuine event from a
// forged one, so the webhook route must refuse every request.
package payment_tests

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"soli/formations/src/payment/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v85"
)

func TestStripeWebhook_NoSecretConfigured_RefusedAsMisconfiguration(t *testing.T) {
	for name, secret := range map[string]string{
		"secret is empty": "",
	} {
		t.Run(name, func(t *testing.T) {
			db := freshTestDB(t)
			router := newRouterWithRealService(t, db, secret)
			sub := seedUpgradeSubscription(t, db, "user_no_secret", seedPlan(t, db, models.SubscriptionPlan{Name: "No secret plan"}))

			eventID := "evt_no_secret_" + uuid.NewString()
			payload := []byte(fmt.Sprintf(`{"id":%q,"object":"event","api_version":%q,"type":"customer.subscription.deleted","created":%d,`+
				`"data":{"object":{"id":%q,"object":"subscription","status":"canceled","metadata":{}}}}`,
				eventID, stripe.APIVersion, time.Now().Unix(), *sub.StripeSubscriptionID))
			// Signed with the very secret the server holds: only its emptiness can refuse it.
			w := httptest.NewRecorder()
			router.ServeHTTP(w, buildSignedWebhookRequest(t, payload, secret))

			assert.Equal(t, http.StatusServiceUnavailable, w.Code, "body: %s", w.Body.String())
			var reloaded models.UserSubscription
			require.NoError(t, db.First(&reloaded, "id = ?", sub.ID).Error)
			assert.Equal(t, "active", reloaded.Status, "a refused event must change nothing")
			assert.Zero(t, countWebhookEvents(t, db, eventID), "a refused event must not be recorded")
		})
	}
}
