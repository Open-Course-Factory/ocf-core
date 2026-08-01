// tests/payment/subscriptionExpiry_test.go
//
// #440: nothing enforced expiry. CurrentPeriodEnd was stored on both
// subscription tables and returned in three DTOs, but never appeared in a single
// WHERE clause — entitlement was decided purely by status.
//
// For recurring Stripe subscriptions that worked by accident: Stripe keeps the
// time and its webhooks flip the status. It breaks for anything Stripe is not
// driving — an admin assignment with duration_days, or a prepaid day pack, which
// has no Stripe subscription to flip anything and would therefore grant access
// forever.
//
// WHY A NEW COLUMN rather than reading CurrentPeriodEnd:
//
//   - CurrentPeriodEnd is a non-pointer time.Time mirroring Stripe's billing
//     window, and is ZERO on rows Stripe has not filled in. Making the predicate
//     read it would have instantly un-entitled every such row — silent mass
//     revocation, the exact failure this codebase keeps producing on empty input.
//   - They mean different things. One is Stripe's billing period; the other is
//     OCF's entitlement deadline. One column with two meanings is how the
//     twenty-two liveness predicates got into trouble in the first place.
//
// ExpiresAt is therefore nullable and means exactly one thing: NULL = no deadline.
package payment_tests

import (
	"testing"
	"time"

	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func seedExpiringSub(t *testing.T, db *gorm.DB, userID string, planID uuid.UUID, expiresAt *time.Time) uuid.UUID {
	t.Helper()
	sub := &models.UserSubscription{
		BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
		UserID:             userID,
		SubscriptionPlanID: planID,
		Status:             "active",
		SubscriptionType:   "personal",
		CurrentPeriodStart: time.Now().AddDate(0, 0, -1),
		CurrentPeriodEnd:   time.Now().AddDate(0, 1, 0),
		ExpiresAt:          expiresAt,
	}
	require.NoError(t, db.Create(sub).Error)
	return sub.ID
}

func seedAnyPlan(t *testing.T, db *gorm.DB) *models.SubscriptionPlan {
	t.Helper()
	plan := &models.SubscriptionPlan{
		BaseModel:       entityManagementModels.BaseModel{ID: uuid.New()},
		Name:            "Seat under expiry test",
		Currency:        "eur",
		BillingInterval: "month",
		IsActive:        true,
		PriceAmount:     600,
	}
	require.NoError(t, db.Create(plan).Error)
	return plan
}

// TestExpiry_LapsedSubscriptionStopsEntitling is the point of the issue: a
// prepaid seat must stop granting access when its window closes.
func TestExpiry_LapsedSubscriptionStopsEntitling(t *testing.T) {
	db := freshTestDB(t)
	plan := seedAnyPlan(t, db)
	userID := uuid.New().String()

	past := time.Now().Add(-1 * time.Hour)
	seedExpiringSub(t, db, userID, plan.ID, &past)

	var live []models.UserSubscription
	require.NoError(t, db.Scopes(models.ScopeEntitling).
		Where("user_id = ?", userID).Find(&live).Error)

	assert.Empty(t, live,
		"a subscription whose window has closed must not entitle, even while its status is active — "+
			"a day pack has no Stripe subscription to flip the status for it")
}

// TestExpiry_FutureAndAbsentDeadlinesStillEntitle covers both the live pack and
// the ordinary recurring subscription, which has no deadline at all.
func TestExpiry_FutureAndAbsentDeadlinesStillEntitle(t *testing.T) {
	db := freshTestDB(t)
	plan := seedAnyPlan(t, db)

	future := time.Now().Add(48 * time.Hour)
	withDeadline := uuid.New().String()
	seedExpiringSub(t, db, withDeadline, plan.ID, &future)

	noDeadline := uuid.New().String()
	seedExpiringSub(t, db, noDeadline, plan.ID, nil)

	for _, tc := range []struct {
		user string
		why  string
	}{
		{withDeadline, "a pack still inside its window entitles"},
		{noDeadline, "NULL means no deadline — a recurring subscription must be unaffected"},
	} {
		var live []models.UserSubscription
		require.NoError(t, db.Scopes(models.ScopeEntitling).
			Where("user_id = ?", tc.user).Find(&live).Error)
		assert.Len(t, live, 1, tc.why)
	}
}

// TestExpiry_ExistingRowsAreUnaffected is the migration-safety guard. Every row
// that exists today has no ExpiresAt, and none of them may lose entitlement when
// this ships — including rows whose CurrentPeriodEnd was never filled in.
func TestExpiry_ExistingRowsAreUnaffected(t *testing.T) {
	db := freshTestDB(t)
	plan := seedAnyPlan(t, db)
	userID := uuid.New().String()

	legacy := &models.UserSubscription{
		BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
		UserID:             userID,
		SubscriptionPlanID: plan.ID,
		Status:             "active",
		SubscriptionType:   "personal",
		// Neither ExpiresAt nor CurrentPeriodEnd set — exactly the shape of a row
		// Stripe has not filled in.
	}
	require.NoError(t, db.Create(legacy).Error)

	var live []models.UserSubscription
	require.NoError(t, db.Scopes(models.ScopeEntitling).
		Where("user_id = ?", userID).Find(&live).Error)

	require.Len(t, live, 1,
		"a row with no deadline must keep entitling — reading the zero CurrentPeriodEnd instead "+
			"would have silently revoked every such subscription")
}

// TestExpiry_AppliesToOrganizationSubscriptionsToo pins that the shared scope
// still works against the other subscription table. ScopeEntitling is used on
// both, so a column present on only one would break org plan resolution outright.
func TestExpiry_AppliesToOrganizationSubscriptionsToo(t *testing.T) {
	db := freshTestDB(t)
	plan := seedAnyPlan(t, db)
	orgID := uuid.New()

	past := time.Now().Add(-1 * time.Hour)
	require.NoError(t, db.Create(&models.OrganizationSubscription{
		BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
		OrganizationID:     orgID,
		SubscriptionPlanID: plan.ID,
		Status:             "active",
		CurrentPeriodStart: time.Now().AddDate(0, 0, -1),
		CurrentPeriodEnd:   time.Now().AddDate(0, 1, 0),
		ExpiresAt:          &past,
	}).Error)

	var live []models.OrganizationSubscription
	require.NoError(t, db.Scopes(models.ScopeEntitling).
		Where("organization_id = ?", orgID).Find(&live).Error)

	assert.Empty(t, live, "an expired organization subscription must not entitle either")
}

// TestExpiry_AdminAssignHonoursDurationDays closes the loop on the endpoint that
// already accepted a duration and quietly ignored it.
func TestExpiry_AdminAssignHonoursDurationDays(t *testing.T) {
	db := freshTestDB(t)
	plan := seedAnyPlan(t, db)
	userID := uuid.New().String()

	before := time.Now()
	sub, err := services.NewSubscriptionService(db).
		AdminAssignSubscription(userID, plan.ID, 3, "admin-under-test")
	require.NoError(t, err)
	require.NotNil(t, sub)

	require.NotNil(t, sub.ExpiresAt,
		"duration_days was accepted and stored nowhere that mattered — it must now set a deadline")
	assert.WithinDuration(t, before.AddDate(0, 0, 3), *sub.ExpiresAt, time.Minute,
		"the deadline must be duration_days from assignment")
}

// TestExpiry_AdminAssignWithoutDurationHasNoDeadline keeps the open-ended case
// open-ended: zero means "no expiry", not "expires immediately".
func TestExpiry_AdminAssignWithoutDurationHasNoDeadline(t *testing.T) {
	db := freshTestDB(t)
	plan := seedAnyPlan(t, db)
	userID := uuid.New().String()

	sub, err := services.NewSubscriptionService(db).
		AdminAssignSubscription(userID, plan.ID, 0, "admin-under-test")
	require.NoError(t, err)
	require.NotNil(t, sub)

	assert.Nil(t, sub.ExpiresAt,
		"a zero duration must mean no deadline — an immediate expiry would revoke on assignment")
}
