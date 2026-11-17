// tests/payment/stripeSubscriptionSync_test.go
//
// The admin sync that pulls every Stripe subscription into the database: what
// it reports for each kind of subscription and what it stores.
package payment_tests

import (
	"testing"

	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyncExistingSubscriptions_MixedStripeSubscriptions_CountsAndStoresEachKind(t *testing.T) {
	db := freshTestDB(t)
	plan := seedPlan(t, db, models.SubscriptionPlan{Name: "Sync plan"})
	known := seedUpgradeSubscription(t, db, "user_known", plan)
	ownedBy := func(userID string) map[string]string {
		return map[string]string{"user_id": userID, "subscription_plan_id": plan.ID.String()}
	}
	bulk := ownedBy("user_bulk")
	bulk["bulk_purchase"] = "true"
	installStrictStripeSubscriptions(t, map[string]string{
		*known.StripeSubscriptionID: stripeSubscriptionJSON(*known.StripeSubscriptionID, *known.StripeCustomerID, "past_due", ownedBy("user_known")),
		"sub_new":                   stripeSubscriptionJSON("sub_new", "cus_new", "active", ownedBy("user_new")),
		"sub_new_too":               stripeSubscriptionJSON("sub_new_too", "cus_new_too", "trialing", ownedBy("user_new_too")),
		"sub_bulk":                  stripeSubscriptionJSON("sub_bulk", "cus_bulk", "active", bulk),
		"sub_unowned":               stripeSubscriptionJSON("sub_unowned", "cus_unowned", "active", nil),
	})

	result, err := services.NewStripeService(db).SyncExistingSubscriptions()
	require.NoError(t, err)

	assert.Equal(t, 5, result.ProcessedSubscriptions)
	// Two created against one updated: swapped counters cannot go unnoticed.
	assert.Equal(t, 2, result.CreatedSubscriptions, "sub_new and sub_new_too are unknown locally")
	assert.Equal(t, 1, result.UpdatedSubscriptions, "only the seeded subscription is known locally")
	assert.Equal(t, 2, result.SkippedSubscriptions, "the bulk purchase and the one without metadata")
	require.Len(t, result.FailedSubscriptions, 1, "a bulk purchase is skipped without being a failure")
	assert.Equal(t, "sub_unowned", result.FailedSubscriptions[0].StripeSubscriptionID)

	var stored []models.UserSubscription
	require.NoError(t, db.Order("user_id").Find(&stored).Error)
	require.Len(t, stored, 3, "skipped subscriptions must not be stored")

	assert.Equal(t, known.ID, stored[0].ID)
	assert.Equal(t, "past_due", stored[0].Status, "the known subscription takes Stripe's status")
	assert.Equal(t, fakePeriodEnd, stored[0].CurrentPeriodEnd.Unix())

	assert.Equal(t, "user_new", stored[1].UserID)
	assert.Equal(t, plan.ID, stored[1].SubscriptionPlanID)
	assert.Equal(t, "sub_new", *stored[1].StripeSubscriptionID)
	assert.Equal(t, "cus_new", *stored[1].StripeCustomerID)
	assert.Equal(t, "active", stored[1].Status)
	assert.Equal(t, fakePeriodEnd, stored[1].CurrentPeriodEnd.Unix())

	assert.Equal(t, "user_new_too", stored[2].UserID)
	assert.Equal(t, "sub_new_too", *stored[2].StripeSubscriptionID)
	assert.Equal(t, "trialing", stored[2].Status)
}
