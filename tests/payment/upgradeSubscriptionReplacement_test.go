// tests/payment/upgradeSubscriptionReplacement_test.go
//
// Regression test for the free→paid upgrade replacement path (review of !284).
// When a user upgrades from a free plan to a paid one, the checkout carries
// `replace_subscription_id` metadata and the webhook handler must delete the old
// free UserSubscription so the user is not left with a stale free sub beside the
// paid one.
//
// This is an OUTCOME-based regression: it drives the real signed webhook with the
// payment hooks installed and asserts the old free row is gone afterwards,
// regardless of the internal reason it currently isn't. Driving the real path
// surfaced that the replace-delete is broken by TWO independent defects (see the
// message to the reviewer):
//
//  1. `genericService.DeleteEntity(replaceID, models.UserSubscription{}, false)`
//     passes NO user context, so the now-enforcing ownership BeforeDelete hook
//     ("" != owner) rejects it (issue #391 made Before* errors actually abort).
//  2. Before that hook even runs, the generic delete's pre-fetch preloads the
//     relation "SubscriptionPlans" (getPreloadString pluralizes the SubEntity),
//     which does not exist — the field is `SubscriptionPlan` — so GetEntity fails
//     with ENT005 first. This is DB-agnostic (fails on Postgres too).
//
// So the outcome stays broken until BOTH are fixed: pass the owner's UserID
// (DeleteEntityWithUser) AND make the pre-fetch not preload a bad relation. The
// payment hooks MUST be installed or an ownership-less delete would false-green.
// (A separate latent nil-deref at stripeService.go:1581 dereferences
// session.Subscription.ID unconditionally; the payload below carries an empty
// subscription object to sidestep it and keep this test on the replace path.)
package payment_tests

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	stripe "github.com/stripe/stripe-go/v85"
	"github.com/stripe/stripe-go/v85/webhook"

	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/mocks"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementModels "soli/formations/src/entityManagement/models"
	registration "soli/formations/src/payment/entityRegistration"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"
)

// signStripeWebhook returns the payload and a valid Stripe-Signature header for
// the given secret, matching what ss.ValidateWebhookSignature verifies.
func signStripeWebhook(t *testing.T, payload []byte, secret string) string {
	t.Helper()
	ts := time.Now()
	mac := webhook.ComputeSignature(ts, payload, secret)
	return fmt.Sprintf("t=%d,v1=%s", ts.Unix(), hex.EncodeToString(mac))
}

// TestCheckoutUpgrade_ReplacesFreeSubscription_OldSubDeleted pins that a
// checkout.session.completed carrying replace_subscription_id deletes the old
// free UserSubscription even with the ownership hook active. RED until the
// handler passes the owner's UserID to the delete.
func TestCheckoutUpgrade_ReplacesFreeSubscription_OldSubDeleted(t *testing.T) {
	_ = freshTestDB(t)
	casdoor.Enforcer = mocks.NewMockEnforcer()

	// Register the UserSubscription entity so the generic delete resolves its
	// ops and reaches the ownership BeforeDelete hook — otherwise the delete
	// fails with "entity not registered" and the test would be red for the
	// wrong reason (masking the ownership-rejection bug under test).
	if _, ok := ems.GlobalEntityRegistrationService.GetEntityOps("UserSubscription"); !ok {
		registration.RegisterUserSubscription(ems.GlobalEntityRegistrationService)
		t.Cleanup(func() { ems.GlobalEntityRegistrationService.UnregisterEntity("UserSubscription") })
	}

	const userID = "upgrading-user-123"

	// Seed a FREE plan and a free UserSubscription owned by the user. Direct
	// GORM writes bypass the hook registry, so seeding never trips the hooks.
	freePlan := &models.SubscriptionPlan{
		BaseModel:       entityManagementModels.BaseModel{ID: uuid.New()},
		Name:            "Free",
		PriceAmount:     0,
		Currency:        "eur",
		BillingInterval: "month",
		IsActive:        true,
	}
	require.NoError(t, sharedTestDB.Create(freePlan).Error)

	oldSub := &models.UserSubscription{
		BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
		UserID:             userID,
		SubscriptionPlanID: freePlan.ID,
		Status:             "active",
		CurrentPeriodStart: time.Now(),
		CurrentPeriodEnd:   time.Now().Add(30 * 24 * time.Hour),
	}
	require.NoError(t, sharedTestDB.Create(oldSub).Error)

	// Install the real payment hooks (ownership hook must be active).
	withPaymentHooksRegistered(t)

	// Build a signed checkout.session.completed event. session.Subscription is
	// omitted so the handler runs the replace-delete then returns without any
	// Stripe API call.
	secret := "whsec_test_upgrade_secret"
	t.Setenv("STRIPE_WEBHOOK_SECRET", secret)

	event := map[string]any{
		"id":          "evt_test_upgrade",
		"object":      "event",
		"api_version": stripe.APIVersion,
		"type":        "checkout.session.completed",
		"data": map[string]any{
			"object": map[string]any{
				"id":     "cs_test_upgrade",
				"object": "checkout.session",
				// Empty-id subscription object: non-nil so the handler's final
				// session.Subscription.ID access doesn't nil-deref, but with an
				// empty id it skips the Stripe metadata-update call (no API key
				// needed). Keeps this test focused on the replace-delete.
				"subscription": map[string]any{"id": ""},
				"metadata": map[string]any{
					"user_id":                 userID,
					"replace_subscription_id": oldSub.ID.String(),
				},
			},
		},
	}
	payload, err := json.Marshal(event)
	require.NoError(t, err)

	svc := services.NewStripeService(sharedTestDB)
	err = svc.ProcessWebhook(payload, signStripeWebhook(t, payload, secret))
	require.NoError(t, err, "webhook must be accepted (signature valid, handler completes)")

	// The old free subscription must be gone (hard delete on the replacement path).
	var count int64
	require.NoError(t, sharedTestDB.Model(&models.UserSubscription{}).Where("id = ?", oldSub.ID).Count(&count).Error)
	assert.Equal(t, int64(0), count, "old free subscription must be deleted when replaced by the paid upgrade")
}
