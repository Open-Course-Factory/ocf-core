package payment_tests

// #455: the point of stamping ExpiresAt on pack licences is that entitlement
// stops on its own. ScopeEntitling already excludes rows past their deadline, so
// these tests check the wiring end to end rather than the arithmetic — that is
// covered by the unit tests on ResolvePackTerms.

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

// seedAssignedLicence creates an assigned pack licence with the given deadline.
func seedAssignedLicence(t *testing.T, db *gorm.DB, userID string, planID uuid.UUID, expiresAt *time.Time) {
	t.Helper()
	require.NoError(t, db.Create(&models.UserSubscription{
		BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
		UserID:             userID,
		SubscriptionPlanID: planID,
		SubscriptionType:   "assigned",
		Status:             "active",
		CurrentPeriodStart: time.Now().Add(-48 * time.Hour),
		CurrentPeriodEnd:   time.Now().AddDate(0, 1, 0),
		ExpiresAt:          expiresAt,
	}).Error)
}

func seedPackPlan(t *testing.T, db *gorm.DB) *models.SubscriptionPlan {
	t.Helper()
	plan := &models.SubscriptionPlan{
		BaseModel:       entityManagementModels.BaseModel{ID: uuid.New()},
		Name:            "Pack journée",
		Priority:        10,
		IsActive:        true,
		BulkPurchasable: true,
		SeatUnit:        models.SeatUnitLearnerDay,
		MaxCPU:          4000,
		MaxMemoryMB:     4096,
	}
	require.NoError(t, db.Create(plan).Error)
	return plan
}

// A pack licence still inside its window entitles its holder.
func TestPackLicence_WithinItsWindowStillEntitles(t *testing.T) {
	db := freshTestDB(t)
	plan := seedPackPlan(t, db)

	tomorrow := time.Now().Add(24 * time.Hour)
	seedAssignedLicence(t, db, "learner-live", plan.ID, &tomorrow)

	result, err := services.NewEffectivePlanService(db).GetUserEffectivePlan("learner-live", nil)

	require.NoError(t, err)
	require.NotNil(t, result.Plan)
	assert.Equal(t, plan.ID, result.Plan.ID)
}

// Once the pack's days run out the licence stops entitling, with no sweeper and
// no Stripe status change — which is the whole reason ExpiresAt is separate from
// the billing window.
func TestPackLicence_PastItsDeadlineStopsEntitling(t *testing.T) {
	db := freshTestDB(t)
	plan := seedPackPlan(t, db)

	yesterday := time.Now().Add(-24 * time.Hour)
	seedAssignedLicence(t, db, "learner-expired", plan.ID, &yesterday)

	_, err := services.NewEffectivePlanService(db).GetUserEffectivePlan("learner-expired", nil)

	require.Error(t, err,
		"a pack whose days ran out must stop granting access even though its "+
			"CurrentPeriodEnd is still a month away")
}

// A monthly seat carries no deadline, so it must not be swept up by the same
// predicate.
func TestMonthlySeat_WithoutADeadlineKeepsEntitling(t *testing.T) {
	db := freshTestDB(t)

	plan := &models.SubscriptionPlan{
		BaseModel:       entityManagementModels.BaseModel{ID: uuid.New()},
		Name:            "Seat monthly",
		Priority:        10,
		IsActive:        true,
		BulkPurchasable: true,
		SeatUnit:        models.SeatUnitSeatMonth,
	}
	require.NoError(t, db.Create(plan).Error)

	seedAssignedLicence(t, db, "learner-monthly", plan.ID, nil)

	result, err := services.NewEffectivePlanService(db).GetUserEffectivePlan("learner-monthly", nil)

	require.NoError(t, err)
	require.NotNil(t, result.Plan)
	assert.Equal(t, plan.ID, result.Plan.ID)
}

// The expiry is an entitlement boundary, not a deletion: the row survives so the
// trainer can still see what was bought and Stripe reconciliation still matches.
func TestPackLicence_ExpiredRowIsKeptNotDeleted(t *testing.T) {
	db := freshTestDB(t)
	plan := seedPackPlan(t, db)

	yesterday := time.Now().Add(-24 * time.Hour)
	seedAssignedLicence(t, db, "learner-expired", plan.ID, &yesterday)

	var count int64
	require.NoError(t, db.Model(&models.UserSubscription{}).
		Where("user_id = ?", "learner-expired").Count(&count).Error)

	assert.Equal(t, int64(1), count, "an expired licence stops entitling but remains on record")
}
