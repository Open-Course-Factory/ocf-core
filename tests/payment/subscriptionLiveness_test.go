// tests/payment/subscriptionLiveness_test.go
//
// Pins the canonical definition of "is this subscription live?" (#438).
//
// Before this change the question was answered inline in 22 places with three
// mutually inconsistent status sets, which is a real bug and not merely
// duplication: AssignFreeTrialPlan decided "user already has a subscription"
// with status='active' alone, so a user whose only subscription was past_due
// received a SECOND Trial subscription on top of it.
//
// The fix is one owner for the predicate, not one set: entitlement and billing
// legitimately differ on past_due (a subscription in dunning must keep granting
// access during the grace window, but must not be treated as cleanly paid).
package payment_tests

import (
	"testing"
	"time"

	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEntitlingStatuses_IncludeDunningGrace pins that a subscription in dunning
// still entitles its holder — the behaviour documented at paymentRepository.go:131.
func TestEntitlingStatuses_IncludeDunningGrace(t *testing.T) {
	assert.True(t, models.IsEntitling("active"), "active must entitle")
	assert.True(t, models.IsEntitling("past_due"),
		"past_due must entitle: dunning keeps content and within-grace sessions working")
	assert.False(t, models.IsEntitling("cancelled"), "cancelled must not entitle")
	assert.False(t, models.IsEntitling("unpaid"), "unpaid must not entitle")
	assert.False(t, models.IsEntitling(""), "empty status must not entitle")
}

// TestBillableStatuses_ExcludeDunning pins the narrower predicate used for billing
// operations, where a past_due subscription is explicitly NOT cleanly paid.
func TestBillableStatuses_ExcludeDunning(t *testing.T) {
	assert.True(t, models.IsBillable("active"), "active must be billable")
	assert.False(t, models.IsBillable("past_due"),
		"past_due must not count as billable — it is precisely the not-paid case")
	assert.False(t, models.IsBillable("cancelled"), "cancelled must not be billable")
}

// TestEntitlingIsSupersetOfBillable guards the relationship between the two
// predicates, so a future edit cannot make a subscription billable but not
// entitling — which would charge someone for access they do not have.
func TestEntitlingIsSupersetOfBillable(t *testing.T) {
	for _, status := range models.BillableStatuses() {
		assert.True(t, models.IsEntitling(status),
			"status %q is billable but not entitling — that would charge for absent access", status)
	}
}

// TestScopeEntitling_FiltersAtTheQueryLevel pins the GORM scope both subscription
// tables share, so call sites never re-spell the status list inline.
func TestScopeEntitling_FiltersAtTheQueryLevel(t *testing.T) {
	db := freshTestDB(t)

	plan := &models.SubscriptionPlan{
		Name: "Liveness probe plan", PriceAmount: 1200, Currency: "eur",
		BillingInterval: "month", IsActive: true,
	}
	require.NoError(t, db.Create(plan).Error)
	defer func() {
		db.Where("subscription_plan_id = ?", plan.ID).Unscoped().Delete(&models.UserSubscription{})
		db.Unscoped().Delete(plan)
	}()

	userID := uuid.New().String()
	for _, status := range []string{"active", "past_due", "cancelled", "unpaid"} {
		require.NoError(t, db.Create(&models.UserSubscription{
			UserID: userID, SubscriptionPlanID: plan.ID, Status: status,
			CurrentPeriodStart: time.Now(), CurrentPeriodEnd: time.Now().AddDate(0, 1, 0),
		}).Error)
	}

	var live []models.UserSubscription
	require.NoError(t, db.Scopes(models.ScopeEntitling).
		Where("user_id = ?", userID).Find(&live).Error)

	require.Len(t, live, 2, "only active and past_due entitle")
	got := map[string]bool{}
	for _, s := range live {
		got[s.Status] = true
	}
	assert.True(t, got["active"])
	assert.True(t, got["past_due"])
}

// TestFreeTrialNotAssignedOverDunningSubscription is the regression test for the
// bug the consolidation fixes: the "already has a subscription" check must use the
// canonical entitling predicate, not status='active' alone.
func TestFreeTrialNotAssignedOverDunningSubscription(t *testing.T) {
	db := freshTestDB(t)

	paidPlan := &models.SubscriptionPlan{
		Name: "Solo probe", PriceAmount: 1200, Currency: "eur",
		BillingInterval: "month", IsActive: true, Priority: 10,
	}
	trialPlan := &models.SubscriptionPlan{
		Name: "Trial", PriceAmount: 0, Currency: "eur",
		BillingInterval: "month", IsActive: true, Priority: 0,
	}
	require.NoError(t, db.Create(paidPlan).Error)
	require.NoError(t, db.Create(trialPlan).Error)
	defer func() {
		db.Where("subscription_plan_id IN ?", []uuid.UUID{paidPlan.ID, trialPlan.ID}).
			Unscoped().Delete(&models.UserSubscription{})
		db.Unscoped().Delete(paidPlan)
		db.Unscoped().Delete(trialPlan)
	}()

	userID := uuid.New().String()
	require.NoError(t, db.Create(&models.UserSubscription{
		UserID: userID, SubscriptionPlanID: paidPlan.ID, Status: "past_due",
		CurrentPeriodStart: time.Now().AddDate(0, -1, 0), CurrentPeriodEnd: time.Now().AddDate(0, 0, 5),
	}).Error)

	// The user already holds an entitling subscription, so no Trial may be added.
	assigned, err := services.EnsureFreeTrialAssigned(db, userID)
	require.NoError(t, err)
	assert.False(t, assigned, "a user in dunning already has a subscription — no Trial on top")

	var subs []models.UserSubscription
	require.NoError(t, db.Where("user_id = ?", userID).Find(&subs).Error)
	assert.Len(t, subs, 1, "must not stack a second subscription onto a past_due one")
}

// TestEnsureFreeTrialAssigned_IsIdempotent pins the property the three call sites
// depend on — signup, the startup healing loop, and bulk import all invoke this
// repeatedly over the same users, so a second call must be a no-op rather than a
// second subscription.
func TestEnsureFreeTrialAssigned_IsIdempotent(t *testing.T) {
	db := freshTestDB(t)

	trialPlan := &models.SubscriptionPlan{
		Name: services.FreePlanName, PriceAmount: 0, Currency: "eur",
		BillingInterval: "month", IsActive: true, Priority: 0,
		IsDefaultFree: true, // the election FindFreePlan reads
	}
	require.NoError(t, db.Create(trialPlan).Error)
	defer func() {
		db.Where("subscription_plan_id = ?", trialPlan.ID).Unscoped().Delete(&models.UserSubscription{})
		db.Unscoped().Delete(trialPlan)
	}()

	userID := uuid.New().String()

	assigned, err := services.EnsureFreeTrialAssigned(db, userID)
	require.NoError(t, err)
	assert.True(t, assigned, "first call assigns the free plan")

	assigned, err = services.EnsureFreeTrialAssigned(db, userID)
	require.NoError(t, err)
	assert.False(t, assigned, "second call must be a no-op")

	var subs []models.UserSubscription
	require.NoError(t, db.Where("user_id = ?", userID).Find(&subs).Error)
	require.Len(t, subs, 1, "repeated calls must never stack subscriptions")
	assert.Equal(t, "personal", subs[0].SubscriptionType,
		"the DB default must still apply through the shared path — effectivePlanService filters on it")
}

// TestAdminAssignReplacesADunningSubscription pins that an admin reassignment
// supersedes whatever still entitles the user, not only an 'active' row.
//
// Checking for 'active' alone left the old subscription live alongside the new
// one, and GetPrimaryUserSubscription resolves by plan priority — so a user
// reassigned from a high-priority plan while in dunning kept the old plan.
func TestAdminAssignReplacesADunningSubscription(t *testing.T) {
	db := freshTestDB(t)

	oldPlan := &models.SubscriptionPlan{
		Name: "Old high priority", PriceAmount: 1990, Currency: "eur",
		BillingInterval: "month", IsActive: true, Priority: 20,
	}
	newPlan := &models.SubscriptionPlan{
		Name: "New low priority", PriceAmount: 600, Currency: "eur",
		BillingInterval: "month", IsActive: true, Priority: 5,
	}
	require.NoError(t, db.Create(oldPlan).Error)
	require.NoError(t, db.Create(newPlan).Error)
	defer func() {
		db.Where("subscription_plan_id IN ?", []uuid.UUID{oldPlan.ID, newPlan.ID}).
			Unscoped().Delete(&models.UserSubscription{})
		db.Unscoped().Delete(oldPlan)
		db.Unscoped().Delete(newPlan)
	}()

	userID := uuid.New().String()
	require.NoError(t, db.Create(&models.UserSubscription{
		UserID: userID, SubscriptionPlanID: oldPlan.ID, Status: "past_due",
		SubscriptionType:   "personal",
		CurrentPeriodStart: time.Now().AddDate(0, -1, 0), CurrentPeriodEnd: time.Now().AddDate(0, 0, 5),
	}).Error)

	_, err := services.NewSubscriptionService(db).
		AdminAssignSubscription(userID, newPlan.ID, 30, "admin-under-test")
	require.NoError(t, err)

	var live []models.UserSubscription
	require.NoError(t, db.Scopes(models.ScopeEntitling).
		Where("user_id = ?", userID).Find(&live).Error)

	require.Len(t, live, 1, "the dunning subscription must be superseded, not left alongside")
	assert.Equal(t, newPlan.ID, live[0].SubscriptionPlanID,
		"the newly assigned plan must be the one that survives")
}

// TestFindFreePlan_MissingPlanIsAnError pins that a broken catalogue surfaces as an
// error rather than silently assigning nothing.
func TestFindFreePlan_MissingPlanIsAnError(t *testing.T) {
	db := freshTestDB(t)

	plan, err := services.FindFreePlan(db)
	assert.Error(t, err, "absent free plan must be an error, not a nil plan and nil error")
	assert.Nil(t, plan)
}
