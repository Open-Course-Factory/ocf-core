// tests/payment/trialPlanHeal_test.go
// Regression tests for the startup heal that assigns the free Trial plan to
// users who are missing a subscription (issue #244).
//
// The original bug: the heal query tested `status = 'active'` while
// GetActiveUserSubscription tested a wider set, so a user whose subscription
// sat in a non-active-but-live status was handed a duplicate Trial on every
// server restart.
//
// These tests used to work around ensureUsersHaveTrialPlan being unexported and
// calling casdoorsdk.GetUsers() by re-implementing its WHERE clause inline. That
// meant they asserted against a *copy* of the logic: the copy could stay green
// while production drifted, which is exactly what happened. The per-user
// decision now lives in EnsureFreeTrialAssigned, which the heal loop calls, so
// these tests exercise the real code path.
//
// The scenario status is 'past_due' rather than 'trialing': #439 retired
// trialing, and past_due is the surviving entitling-but-not-active case, which
// carries the identical risk.
package payment_tests

import (
	"testing"
	"time"

	entityManagementModels "soli/formations/src/entityManagement/models"
	paymentModels "soli/formations/src/payment/models"
	paymentServices "soli/formations/src/payment/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// seedFreeTrialPlan creates the free plan the heal path looks up by name.
func seedFreeTrialPlan(t *testing.T, db *gorm.DB) *paymentModels.SubscriptionPlan {
	t.Helper()
	plan := &paymentModels.SubscriptionPlan{
		BaseModel:       entityManagementModels.BaseModel{ID: uuid.New()},
		Name:            paymentServices.FreePlanName,
		Description:     "Free trial plan",
		Priority:        0,
		PriceAmount:     0,
		Currency:        "eur",
		BillingInterval: "month",
		IsActive:        true,
	}
	require.NoError(t, db.Create(plan).Error)
	return plan
}

// countSubscriptions returns how many subscription rows a user holds, in any status.
func countSubscriptions(t *testing.T, db *gorm.DB, userID string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&paymentModels.UserSubscription{}).
		Where("user_id = ?", userID).Count(&n).Error)
	return n
}

// TestEnsureUsersHaveTrialPlan_WithDunningStatus_ShouldNotDuplicate is the
// primary regression test for #244: a user holding a subscription that entitles
// them but is not 'active' must not be handed a second one by the heal loop.
func TestEnsureUsersHaveTrialPlan_WithDunningStatus_ShouldNotDuplicate(t *testing.T) {
	db := freshTestDB(t)
	trialPlan := seedFreeTrialPlan(t, db)

	userID := uuid.New().String()
	now := time.Now()
	require.NoError(t, db.Create(&paymentModels.UserSubscription{
		BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
		UserID:             userID,
		SubscriptionPlanID: trialPlan.ID,
		Status:             "past_due", // entitling, but not 'active'
		SubscriptionType:   "personal",
		CurrentPeriodStart: now.AddDate(0, -1, 0),
		CurrentPeriodEnd:   now.AddDate(0, 1, 0),
	}).Error)

	require.Equal(t, int64(1), countSubscriptions(t, db, userID),
		"precondition: user should start with 1 subscription")

	assigned, err := paymentServices.EnsureFreeTrialAssigned(db, userID)
	require.NoError(t, err)

	assert.False(t, assigned,
		"a user in dunning already holds a subscription — the heal must skip them")
	assert.Equal(t, int64(1), countSubscriptions(t, db, userID),
		"REGRESSION: the heal stacked a second subscription onto a past_due one")
}

// TestEnsureUsersHaveTrialPlan_WithActiveStatus_ShouldNotDuplicate is the
// baseline: an already-active user is skipped.
func TestEnsureUsersHaveTrialPlan_WithActiveStatus_ShouldNotDuplicate(t *testing.T) {
	db := freshTestDB(t)
	trialPlan := seedFreeTrialPlan(t, db)

	userID := uuid.New().String()
	now := time.Now()
	require.NoError(t, db.Create(&paymentModels.UserSubscription{
		BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
		UserID:             userID,
		SubscriptionPlanID: trialPlan.ID,
		Status:             "active", // already active — heal must skip
		SubscriptionType:   "personal",
		CurrentPeriodStart: now.AddDate(0, -1, 0),
		CurrentPeriodEnd:   now.AddDate(0, 1, 0),
	}).Error)

	assigned, err := paymentServices.EnsureFreeTrialAssigned(db, userID)
	require.NoError(t, err)

	assert.False(t, assigned, "an active user must be skipped")
	assert.Equal(t, int64(1), countSubscriptions(t, db, userID),
		"user with an active subscription should not get a duplicate")
}

// TestEnsureUsersHaveTrialPlan_NoSubscription_ShouldAssignTrial is the happy
// path the heal exists for.
func TestEnsureUsersHaveTrialPlan_NoSubscription_ShouldAssignTrial(t *testing.T) {
	db := freshTestDB(t)
	trialPlan := seedFreeTrialPlan(t, db)

	userID := uuid.New().String()
	// Deliberately no subscription for this user.

	assigned, err := paymentServices.EnsureFreeTrialAssigned(db, userID)
	require.NoError(t, err)

	assert.True(t, assigned, "a user with no subscription should receive the free plan")
	require.Equal(t, int64(1), countSubscriptions(t, db, userID),
		"user with no prior subscription should end with exactly 1")

	var sub paymentModels.UserSubscription
	require.NoError(t, db.Where("user_id = ?", userID).First(&sub).Error)
	assert.Equal(t, trialPlan.ID, sub.SubscriptionPlanID, "must be the free plan")
	assert.Equal(t, "active", sub.Status)
}

// TestEnsureUsersHaveTrialPlan_TerminalStatusIsNotASubscription pins the other
// side of the predicate: a cancelled subscription does not count as holding one,
// so the user is healed rather than left with no plan at all.
func TestEnsureUsersHaveTrialPlan_TerminalStatusIsNotASubscription(t *testing.T) {
	db := freshTestDB(t)
	trialPlan := seedFreeTrialPlan(t, db)

	userID := uuid.New().String()
	now := time.Now()
	require.NoError(t, db.Create(&paymentModels.UserSubscription{
		BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
		UserID:             userID,
		SubscriptionPlanID: trialPlan.ID,
		Status:             "cancelled",
		SubscriptionType:   "personal",
		CurrentPeriodStart: now.AddDate(0, -2, 0),
		CurrentPeriodEnd:   now.AddDate(0, -1, 0),
	}).Error)

	assigned, err := paymentServices.EnsureFreeTrialAssigned(db, userID)
	require.NoError(t, err)

	assert.True(t, assigned, "a cancelled subscription does not entitle — heal the user")
	assert.Equal(t, int64(2), countSubscriptions(t, db, userID),
		"the cancelled row is kept for history alongside the new free subscription")
}
