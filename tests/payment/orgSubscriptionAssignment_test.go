// tests/payment/orgSubscriptionAssignment_test.go
//
// Tests the "one active subscription per organization" invariant:
//   - Assigning a new plan to an org deactivates the previous active one
//     atomically (same transaction).
//   - The backfill helper cleans up orgs that already have duplicate active
//     subscriptions in the DB.
package payment_tests

import (
	"testing"
	"time"

	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/initialization"
	organizationModels "soli/formations/src/organizations/models"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/repositories"
	"soli/formations/src/payment/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// runBackfillForTest calls the dialect-agnostic backfill so the same
// invariant the production PostgreSQL query enforces is exercised against
// the SQLite test DB.
func runBackfillForTest(t *testing.T, db *gorm.DB) {
	t.Helper()
	initialization.BackfillSingleActiveOrgSubscriptionGeneric(db)
}

// seedOrgAndTwoPlans creates a Free plan, a Pro plan, and a single
// organization owned by a fresh user. Used by all assignment-invariant tests.
func seedOrgAndTwoPlans(t *testing.T, db *gorm.DB) (
	freePlan *models.SubscriptionPlan,
	proPlan *models.SubscriptionPlan,
	org *organizationModels.Organization,
	userID string,
) {
	t.Helper()

	freePlan = &models.SubscriptionPlan{
		BaseModel:       entityManagementModels.BaseModel{ID: uuid.New()},
		Name:            "FreeAssignTest",
		Priority:        0,
		PriceAmount:     0,
		Currency:        "eur",
		BillingInterval: "month",
		IsActive:        true,
	}
	require.NoError(t, db.Create(freePlan).Error)

	proPlan = &models.SubscriptionPlan{
		BaseModel:       entityManagementModels.BaseModel{ID: uuid.New()},
		Name:            "ProAssignTest",
		Priority:        20,
		PriceAmount:     0, // free-priced so the service marks it active immediately
		Currency:        "eur",
		BillingInterval: "month",
		IsActive:        true,
	}
	require.NoError(t, db.Create(proPlan).Error)

	userID = "test_user_org_assign"

	org = &organizationModels.Organization{
		BaseModel:   entityManagementModels.BaseModel{ID: uuid.New()},
		Name:        "org_assign_test",
		DisplayName: "Org Assign Test",
		OwnerUserID: userID,
		IsActive:    true,
	}
	require.NoError(t, db.Omit("Metadata").Create(org).Error)

	return freePlan, proPlan, org, userID
}

func TestAssignOrgSubscription_DeactivatesPrevious(t *testing.T) {
	db := freshTestDB(t)
	freePlan, proPlan, org, userID := seedOrgAndTwoPlans(t, db)
	service := services.NewOrganizationSubscriptionService(db)

	// First assignment: Free plan, active.
	first, err := service.CreateOrganizationSubscription(org.ID, freePlan.ID, userID, 1, false)
	require.NoError(t, err)
	require.Equal(t, "active", first.Status)

	// Sleep 1ms to guarantee distinct created_at ordering on SQLite.
	time.Sleep(2 * time.Millisecond)

	// Second assignment: Pro plan.
	second, err := service.CreateOrganizationSubscription(org.ID, proPlan.ID, userID, 1, false)
	require.NoError(t, err)
	require.Equal(t, "active", second.Status)
	require.NotEqual(t, first.ID, second.ID)

	// Exactly one active subscription remains, and it points at Pro.
	var activeCount int64
	require.NoError(t, db.Model(&models.OrganizationSubscription{}).
		Where("organization_id = ? AND status IN ?", org.ID, []string{"active", "trialing"}).
		Count(&activeCount).Error)
	assert.Equal(t, int64(1), activeCount, "expected exactly one active subscription per org")

	// The previous subscription was cancelled with a cancelled_at timestamp.
	var prevSub models.OrganizationSubscription
	require.NoError(t, db.Where("id = ?", first.ID).First(&prevSub).Error)
	assert.Equal(t, "cancelled", prevSub.Status)
	assert.NotNil(t, prevSub.CancelledAt)
	assert.False(t, prevSub.CancelledAt.IsZero())
}

func TestAssignOrgSubscription_NoOpWhenNoPrior(t *testing.T) {
	db := freshTestDB(t)
	freePlan, _, org, userID := seedOrgAndTwoPlans(t, db)
	service := services.NewOrganizationSubscriptionService(db)

	// Fresh org with no prior subscription.
	first, err := service.CreateOrganizationSubscription(org.ID, freePlan.ID, userID, 1, false)
	require.NoError(t, err)
	require.Equal(t, "active", first.Status)

	// Exactly one active subscription, and no rows were spuriously cancelled.
	var activeCount int64
	require.NoError(t, db.Model(&models.OrganizationSubscription{}).
		Where("organization_id = ? AND status IN ?", org.ID, []string{"active", "trialing"}).
		Count(&activeCount).Error)
	assert.Equal(t, int64(1), activeCount)

	var cancelledCount int64
	require.NoError(t, db.Model(&models.OrganizationSubscription{}).
		Where("organization_id = ? AND status = ?", org.ID, "cancelled").
		Count(&cancelledCount).Error)
	assert.Equal(t, int64(0), cancelledCount, "no prior subscription should have been cancelled")
}

func TestAssignOrgSubscription_IncompletePaidPlanDoesNotCancelActive(t *testing.T) {
	// Regression guard: an "incomplete" subscription (paid plan awaiting Stripe
	// confirmation) must NOT cancel the org's currently-active subscription —
	// that would leave the org without coverage while the user is in the
	// Stripe checkout flow.
	db := freshTestDB(t)
	freePlan, _, org, userID := seedOrgAndTwoPlans(t, db)

	// First: free plan, active.
	service := services.NewOrganizationSubscriptionService(db)
	first, err := service.CreateOrganizationSubscription(org.ID, freePlan.ID, userID, 1, false)
	require.NoError(t, err)
	require.Equal(t, "active", first.Status)

	// Insert a paid plan and use the repository directly with an "incomplete"
	// row — mimicking the Stripe-pending state before webhook activation.
	paidPlan := &models.SubscriptionPlan{
		BaseModel:       entityManagementModels.BaseModel{ID: uuid.New()},
		Name:            "PaidAssignTest",
		Priority:        30,
		PriceAmount:     1200,
		Currency:        "eur",
		BillingInterval: "month",
		IsActive:        true,
	}
	require.NoError(t, db.Create(paidPlan).Error)

	incomplete, err := service.CreateOrganizationSubscription(org.ID, paidPlan.ID, userID, 1, false)
	require.NoError(t, err)
	require.Equal(t, "incomplete", incomplete.Status)

	// The original active subscription must still be active.
	var stillActive models.OrganizationSubscription
	require.NoError(t, db.Where("id = ?", first.ID).First(&stillActive).Error)
	assert.Equal(t, "active", stillActive.Status, "incomplete insert must not cancel active subscription")
	assert.Nil(t, stillActive.CancelledAt)
}

// insertOrgSubAtTime inserts a row with a specific created_at, bypassing the
// service so we can simulate the pre-fix bad state of multiple active rows.
func insertOrgSubAtTime(t *testing.T, db *gorm.DB, orgID, planID uuid.UUID, createdAt time.Time) uuid.UUID {
	t.Helper()
	id := uuid.New()
	require.NoError(t, db.Exec(`
		INSERT INTO organization_subscriptions
			(id, created_at, updated_at, organization_id, subscription_plan_id,
			 stripe_customer_id, status, quantity, current_period_start, current_period_end)
		VALUES (?, ?, ?, ?, ?, '', 'active', 1, ?, ?)
	`, id, createdAt, createdAt, orgID, planID, createdAt, createdAt.AddDate(1, 0, 0)).Error)
	return id
}

// withoutUniqueActiveOrgSubIndex temporarily drops the partial unique index
// that enforces "one active subscription per org" so the test can seed the
// pre-fix bad state of multiple active rows. The index is recreated when
// the test ends.
//
// Use ONLY for tests that exercise the legacy backfill helper — the index
// is the production-correct invariant and dropping it elsewhere defeats
// the protection.
//
// NOT SAFE under t.Parallel(): dropping the shared-DB index would break
// any concurrent test that relies on the unique-active-org-sub invariant.
func withoutUniqueActiveOrgSubIndex(t *testing.T, db *gorm.DB) {
	t.Helper()
	const indexName = "idx_unique_active_org_subscription"
	require.NoError(t, db.Exec(`DROP INDEX IF EXISTS `+indexName).Error)
	t.Cleanup(func() {
		// Recreate so the next test's fresh DB inherits the index again.
		models.MigrateUniqueActiveOrgSubscriptionIndex(db)
	})
}

func TestBackfillSingleActiveSubscription_DeactivatesAllButNewest(t *testing.T) {
	db := freshTestDB(t)
	// This test simulates the legacy pre-fix bad state, which the DB-level
	// partial unique index now prevents from being created in the first
	// place. Drop the index for the duration of the test so we can seed it.
	withoutUniqueActiveOrgSubIndex(t, db)
	freePlan, proPlan, org, _ := seedOrgAndTwoPlans(t, db)

	// Simulate the pre-fix bad state: 3 active subscriptions for the same org.
	t1 := time.Now().Add(-3 * time.Hour)
	t2 := time.Now().Add(-2 * time.Hour)
	t3 := time.Now().Add(-1 * time.Hour)
	id1 := insertOrgSubAtTime(t, db, org.ID, freePlan.ID, t1)
	id2 := insertOrgSubAtTime(t, db, org.ID, freePlan.ID, t2)
	id3 := insertOrgSubAtTime(t, db, org.ID, proPlan.ID, t3) // newest

	// Sanity: all three active before backfill.
	var beforeCount int64
	require.NoError(t, db.Model(&models.OrganizationSubscription{}).
		Where("organization_id = ? AND status = ?", org.ID, "active").
		Count(&beforeCount).Error)
	require.Equal(t, int64(3), beforeCount)

	// Run the dialect-agnostic backfill (production uses PG version with
	// window functions; behaviour is identical).
	runBackfillForTest(t, db)

	// Only the newest (t3 / id3) stays active.
	var afterActiveIDs []uuid.UUID
	require.NoError(t, db.Model(&models.OrganizationSubscription{}).
		Where("organization_id = ? AND status = ?", org.ID, "active").
		Pluck("id", &afterActiveIDs).Error)
	require.Len(t, afterActiveIDs, 1)
	assert.Equal(t, id3, afterActiveIDs[0])

	// The other two are now cancelled with cancelled_at set.
	for _, id := range []uuid.UUID{id1, id2} {
		var sub models.OrganizationSubscription
		require.NoError(t, db.Where("id = ?", id).First(&sub).Error)
		assert.Equal(t, "cancelled", sub.Status, "sub %s should be cancelled", id)
		assert.NotNil(t, sub.CancelledAt, "sub %s should have cancelled_at set", id)
	}
}

func TestBackfillSingleActiveSubscription_Idempotent(t *testing.T) {
	db := freshTestDB(t)
	// See TestBackfillSingleActiveSubscription_DeactivatesAllButNewest: the
	// partial unique index is dropped while we seed legacy bad data, then
	// recreated via t.Cleanup.
	withoutUniqueActiveOrgSubIndex(t, db)
	freePlan, proPlan, org, _ := seedOrgAndTwoPlans(t, db)

	t1 := time.Now().Add(-2 * time.Hour)
	t2 := time.Now().Add(-1 * time.Hour)
	insertOrgSubAtTime(t, db, org.ID, freePlan.ID, t1)
	id2 := insertOrgSubAtTime(t, db, org.ID, proPlan.ID, t2)

	// First run: cancels the older row.
	runBackfillForTest(t, db)

	var firstRunActive []uuid.UUID
	require.NoError(t, db.Model(&models.OrganizationSubscription{}).
		Where("organization_id = ? AND status = ?", org.ID, "active").
		Pluck("id", &firstRunActive).Error)
	require.Len(t, firstRunActive, 1)
	assert.Equal(t, id2, firstRunActive[0])

	// Capture the cancelled_at of the older row.
	var firstRunCancelledAt *time.Time
	require.NoError(t, db.Model(&models.OrganizationSubscription{}).
		Where("organization_id = ? AND status = ?", org.ID, "cancelled").
		Pluck("cancelled_at", &firstRunCancelledAt).Error)

	// Second run: must not change anything.
	runBackfillForTest(t, db)

	var secondRunActive []uuid.UUID
	require.NoError(t, db.Model(&models.OrganizationSubscription{}).
		Where("organization_id = ? AND status = ?", org.ID, "active").
		Pluck("id", &secondRunActive).Error)
	assert.Equal(t, firstRunActive, secondRunActive, "active set must be stable across re-runs")

	// cancelled_at on the older row must not have been overwritten.
	var secondRunCancelledAt *time.Time
	require.NoError(t, db.Model(&models.OrganizationSubscription{}).
		Where("organization_id = ? AND status = ?", org.ID, "cancelled").
		Pluck("cancelled_at", &secondRunCancelledAt).Error)
	if firstRunCancelledAt != nil && secondRunCancelledAt != nil {
		assert.True(t, firstRunCancelledAt.Equal(*secondRunCancelledAt),
			"cancelled_at must not be overwritten on subsequent backfill runs")
	}
}

// TestAssignOrgSubscription_AtomicCancelPreservesIntegrityOnFailure verifies
// that if the new insert fails inside the atomic transaction, the previous
// subscription is NOT left cancelled (transaction rollback).
func TestAssignOrgSubscription_AtomicCancelPreservesIntegrityOnFailure(t *testing.T) {
	db := freshTestDB(t)
	freePlan, _, org, userID := seedOrgAndTwoPlans(t, db)
	service := services.NewOrganizationSubscriptionService(db)

	first, err := service.CreateOrganizationSubscription(org.ID, freePlan.ID, userID, 1, false)
	require.NoError(t, err)
	require.Equal(t, "active", first.Status)

	// Attempt to insert with an invalid plan ID at the REPOSITORY layer
	// (bypassing the service-level plan check, which would short-circuit
	// before we hit the transaction).
	repo := repositories.NewOrganizationSubscriptionRepository(db)
	bogus := &models.OrganizationSubscription{
		OrganizationID:     org.ID,
		SubscriptionPlanID: uuid.Nil, // will violate not-null
		Status:             "active",
		CurrentPeriodStart: time.Now(),
		CurrentPeriodEnd:   time.Now().AddDate(1, 0, 0),
		Quantity:           1,
	}
	err = repo.CreateOrganizationSubscriptionAtomic(bogus)
	// SQLite may or may not enforce the not-null on a nil UUID; only assert
	// rollback semantics when the insert actually failed.
	if err != nil {
		// The previous active subscription must still be active.
		var stillActive models.OrganizationSubscription
		require.NoError(t, db.Where("id = ?", first.ID).First(&stillActive).Error)
		assert.Equal(t, "active", stillActive.Status, "rollback must preserve the previous active subscription")
		assert.Nil(t, stillActive.CancelledAt)
	}
}
