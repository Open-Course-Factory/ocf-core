// tests/payment/webhookGuards_test.go
//
// What the Stripe webhook route refuses before it processes anything: a request
// that does not look like Stripe, and above all a payload whose signature does
// not verify. Every case drives the real controller and the real StripeService.
package payment_tests

import (
	"bytes"
	"encoding/hex"
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
	stripeWebhook "github.com/stripe/stripe-go/v85/webhook"
	"gorm.io/gorm"
)

// stripeEventPayload wraps a Stripe object in the event envelope the webhook
// route receives.
func stripeEventPayload(eventID, eventType, objectJSON string) []byte {
	return []byte(fmt.Sprintf(`{"id":%q,"object":"event","api_version":%q,"type":%q,"created":%d,"data":{"object":%s}}`,
		eventID, stripe.APIVersion, eventType, time.Now().Unix(), objectJSON))
}

// userSubscriptionStatus reloads a subscription's status from the database.
func userSubscriptionStatus(t *testing.T, db *gorm.DB, id uuid.UUID) string {
	t.Helper()
	var sub models.UserSubscription
	require.NoError(t, db.First(&sub, "id = ?", id).Error)
	return sub.Status
}

// signatureHeader signs payload as Stripe does, at the given time.
func signatureHeader(payload []byte, secret string, at time.Time) string {
	return fmt.Sprintf("t=%d,v1=%s", at.Unix(), hex.EncodeToString(stripeWebhook.ComputeSignature(at, payload, secret)))
}

// Each row sends a "subscription deleted" event for an active subscription.
// The first row is the request Stripe really sends and it cancels the
// subscription; every other row differs from it by one thing only, must be
// refused with the exact status, and must leave the subscription active and no
// trace of the event.
func TestStripeWebhook_RequestNotFromStripe_RefusedBeforeProcessing(t *testing.T) {
	const secret = "whsec_test"

	cases := []struct {
		name       string
		alter      func(req *http.Request, payload []byte)
		padding    int
		wantStatus int
	}{
		{name: "genuine signed request is processed", alter: func(*http.Request, []byte) {}, wantStatus: http.StatusOK},
		{
			name:       "user agent is not Stripe",
			alter:      func(req *http.Request, _ []byte) { req.Header.Set("User-Agent", "curl/8.0") },
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "content type is not JSON",
			alter:      func(req *http.Request, _ []byte) { req.Header.Set("Content-Type", "text/plain") },
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "body is over 1 MB",
			alter:      func(*http.Request, []byte) {},
			padding:    1024*1024 + 1,
			wantStatus: http.StatusRequestEntityTooLarge,
		},
		{
			name:       "signature header is missing",
			alter:      func(req *http.Request, _ []byte) { req.Header.Del("Stripe-Signature") },
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "signature was made with another secret",
			alter: func(req *http.Request, payload []byte) {
				req.Header.Set("Stripe-Signature", signatureHeader(payload, "whsec_other", time.Now()))
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "payload was altered after signing",
			alter: func(req *http.Request, payload []byte) {
				req.Header.Set("Stripe-Signature", signatureHeader(append([]byte(" "), payload...), secret, time.Now()))
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "signature is older than the replay tolerance",
			alter: func(req *http.Request, payload []byte) {
				stale := time.Now().Add(-2 * stripeWebhook.DefaultTolerance)
				req.Header.Set("Stripe-Signature", signatureHeader(payload, secret, stale))
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "signature header is not a signature",
			alter:      func(req *http.Request, _ []byte) { req.Header.Set("Stripe-Signature", "not-a-signature") },
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := freshTestDB(t)
			installStrictStripeSubscriptions(t, nil)
			router := newRouterWithRealService(t, db, secret)
			sub := seedUpgradeSubscription(t, db, "user_guard", seedPlan(t, db, models.SubscriptionPlan{Name: "Guard plan"}))

			eventID := "evt_guard_" + uuid.NewString()
			payload := stripeEventPayload(eventID, "customer.subscription.deleted",
				stripeSubscriptionJSON(*sub.StripeSubscriptionID, *sub.StripeCustomerID, "canceled", nil))
			// Trailing whitespace keeps the JSON valid and is covered by the signature.
			payload = append(payload, bytes.Repeat([]byte(" "), tc.padding)...)
			req := buildSignedWebhookRequest(t, payload, secret)
			tc.alter(req, payload)

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			require.Equal(t, tc.wantStatus, w.Code, "body: %s", w.Body.String())
			if tc.wantStatus == http.StatusOK {
				assert.Equal(t, "cancelled", userSubscriptionStatus(t, db, sub.ID), "the genuine event must be applied")
				return
			}
			assert.Equal(t, "active", userSubscriptionStatus(t, db, sub.ID), "a refused request must change nothing")
			assert.Zero(t, countWebhookEvents(t, db, eventID), "a refused request must not be recorded as an event")
		})
	}
}
