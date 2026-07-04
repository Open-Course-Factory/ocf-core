// tests/payment/bulkLicenseBackfill_test.go
//
// RED-phase failing tests for issue #368 / MR !270: a one-shot data migration
// that repairs LEGACY bulk batches created before the #367/!269 fix.
//
// Pre-!269 shape: handleBulkSubscriptionCreated created every license row with
// StripeSubscriptionID = the batch's (shared) Stripe subscription id, so the
// partial unique index idx_user_stripe_sub_not_null let only ONE license row
// survive per batch. Production therefore has paid batches with total_quantity=N
// but a single license row carrying that shared stripe id.
//
// The migration lives in a NEW file in src/payment/backfill/ (e.g.
// bulkLicenses.go) mirroring src/payment/backfill/quota.go:
//   - shares backfill.Options{Apply bool} (dry-run is the default),
//   - RunBulkLicenses(db, Options) (*BulkLicenseReport, error),
//   - idempotent, wraps mutations in a transaction when Apply,
//   - invoked by an operator command like cmd/backfill_bulk_licenses/main.go
//     (mirroring cmd/backfill_quota), NOT wired into initialization.
//
// PROPOSED API pinned by these tests (undefined until the dev writes it — the
// package fails to compile with "undefined: backfill.RunBulkLicenses /
// backfill.BulkLicenseReport", which is the intended RED for new-API TDD):
//
//	func RunBulkLicenses(db *gorm.DB, opts backfill.Options) (*BulkLicenseReport, error)
//
//	type BulkLicenseReport struct {
//	    Total       int // legacy batches examined
//	    Skipped     int // healthy batches left untouched
//	    Updated     int // Apply: legacy license ROWS repaired (shared stripe id -> NULL)
//	    WouldUpdate int // dry-run equivalent of Updated
//	    Created     int // Apply: backfilled license ROWS created
//	    WouldCreate int // dry-run equivalent of Created
//	    // (+ optional per-batch detail slice, not asserted here)
//	}
//
// Expected semantics (repair a legacy batch so it is fully provisioned and every
// license is assignable via AssignLicense, which selects
// `subscription_batch_id = ? AND status = 'unassigned'`):
//   - NULL the shared StripeSubscriptionID on existing license rows,
//   - create (total_quantity - existing_count) rows with the created-path shape:
//     UserID "", PurchaserUserID = batch purchaser, SubscriptionBatchID = batch,
//     SubscriptionPlanID = batch plan, StripeSubscriptionID NULL, Status
//     "unassigned", periods copied from the batch,
//   - preserve any already-ASSIGNED license (UserID/status/subscription_type),
//   - be idempotent and skip healthy (already-NULL, fully-provisioned) batches.
package payment_tests

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"soli/formations/src/payment/backfill"
	"soli/formations/src/payment/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// seedLegacyBulkBatch creates a pre-!269 legacy batch: total_quantity=quantity
// but only ONE license row, carrying the batch's shared stripe_subscription_id.
// Returns the batch and the shared stripe subscription id.
func seedLegacyBulkBatch(t *testing.T, db *gorm.DB, quantity int) (*models.SubscriptionBatch, string) {
	t.Helper()
	priceID := "price_backfill_" + uuid.NewString()
	plan := &models.SubscriptionPlan{
		Name:            "Backfill Plan",
		PriceAmount:     1999,
		Currency:        "eur",
		BillingInterval: "month",
		StripePriceID:   &priceID,
		IsActive:        true,
	}
	require.NoError(t, db.Create(plan).Error)

	purchaser := "purchaser_" + uuid.NewString()
	sharedStripeSubID := "sub_legacy_" + uuid.NewString()
	customerID := "cus_legacy_" + uuid.NewString()

	batch := &models.SubscriptionBatch{
		PurchaserUserID:          purchaser,
		SubscriptionPlanID:       plan.ID,
		StripeSubscriptionID:     sharedStripeSubID,
		StripeSubscriptionItemID: "si_legacy",
		TotalQuantity:            quantity,
		AssignedQuantity:         0,
		Status:                   "active",
		CurrentPeriodStart:       time.Now(),
		CurrentPeriodEnd:         time.Now().Add(30 * 24 * time.Hour),
	}
	require.NoError(t, db.Create(batch).Error)

	// The single surviving legacy license, carrying the shared stripe id.
	legacyLicense := &models.UserSubscription{
		UserID:               "",
		PurchaserUserID:      &purchaser,
		SubscriptionBatchID:  &batch.ID,
		SubscriptionPlanID:   plan.ID,
		StripeSubscriptionID: &sharedStripeSubID,
		StripeCustomerID:     &customerID,
		Status:               "unassigned",
		CurrentPeriodStart:   batch.CurrentPeriodStart,
		CurrentPeriodEnd:     batch.CurrentPeriodEnd,
	}
	require.NoError(t, db.Create(legacyLicense).Error)

	return batch, sharedStripeSubID
}

func countBatchLicenses(t *testing.T, db *gorm.DB, batchID uuid.UUID) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&models.UserSubscription{}).
		Where("subscription_batch_id = ?", batchID).Count(&n).Error)
	return n
}

func countBatchLicensesWithStripeID(t *testing.T, db *gorm.DB, batchID uuid.UUID) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&models.UserSubscription{}).
		Where("subscription_batch_id = ? AND stripe_subscription_id IS NOT NULL", batchID).
		Count(&n).Error)
	return n
}

func countAssignableLicenses(t *testing.T, db *gorm.DB, batchID uuid.UUID) int64 {
	t.Helper()
	// Mirror AssignLicense's availability query exactly.
	var n int64
	require.NoError(t, db.Model(&models.UserSubscription{}).
		Where("subscription_batch_id = ? AND status = ?", batchID, "unassigned").
		Count(&n).Error)
	return n
}

// 1. Legacy backfill: 1 shared-id license -> NULL id + full quantity of
// assignable rows.
func TestBulkLicenseBackfill_LegacyBatch_BackfillsMissingAssignableLicenses(t *testing.T) {
	db := freshTestDB(t)
	batch, _ := seedLegacyBulkBatch(t, db, 3)

	require.Equal(t, int64(1), countBatchLicenses(t, db, batch.ID), "precondition: 1 legacy license")
	require.Equal(t, int64(1), countBatchLicensesWithStripeID(t, db, batch.ID), "precondition: it carries the shared stripe id")

	report, err := backfill.RunBulkLicenses(db, backfill.Options{Apply: true})
	require.NoError(t, err)

	// The shared stripe id must be cleared everywhere in the batch.
	assert.Equal(t, int64(0), countBatchLicensesWithStripeID(t, db, batch.ID),
		"backfill must NULL the shared stripe_subscription_id on legacy license rows")

	// The batch must end fully provisioned (total_quantity license rows).
	assert.Equal(t, int64(3), countBatchLicenses(t, db, batch.ID),
		"backfill must create the missing licenses so the batch has total_quantity rows")

	// Every row must be assignable (AssignLicense selects status='unassigned').
	assert.Equal(t, int64(3), countAssignableLicenses(t, db, batch.ID),
		"backfilled rows must be status='unassigned' so AssignLicense can hand them out")

	// Report: 2 created, 1 repaired.
	assert.Equal(t, 2, report.Created, "report.Created = backfilled license rows (3 - 1 existing)")
	assert.Equal(t, 1, report.Updated, "report.Updated = legacy license rows whose shared stripe id was nulled")
}

// 2. Idempotency: a second Apply run is a no-op.
func TestBulkLicenseBackfill_Idempotent_SecondRunIsNoop(t *testing.T) {
	db := freshTestDB(t)
	batch, _ := seedLegacyBulkBatch(t, db, 3)

	_, err := backfill.RunBulkLicenses(db, backfill.Options{Apply: true})
	require.NoError(t, err)
	require.Equal(t, int64(3), countBatchLicenses(t, db, batch.ID), "first run provisions to 3")

	report2, err := backfill.RunBulkLicenses(db, backfill.Options{Apply: true})
	require.NoError(t, err)

	assert.Equal(t, int64(3), countBatchLicenses(t, db, batch.ID),
		"second run must not create more rows")
	assert.Equal(t, 0, report2.Created, "idempotent: second run creates nothing")
	assert.Equal(t, 0, report2.Updated, "idempotent: second run repairs nothing")
}

// 3. Dry-run default (Apply:false): reports intentions, writes nothing.
func TestBulkLicenseBackfill_DryRun_ReportsButDoesNotWrite(t *testing.T) {
	db := freshTestDB(t)
	batch, _ := seedLegacyBulkBatch(t, db, 3)

	report, err := backfill.RunBulkLicenses(db, backfill.Options{Apply: false})
	require.NoError(t, err)

	// Report says what it WOULD do.
	assert.Equal(t, 2, report.WouldCreate, "dry-run: would create the 2 missing licenses")
	assert.Equal(t, 1, report.WouldUpdate, "dry-run: would null the shared stripe id on 1 legacy row")
	assert.Equal(t, 0, report.Created, "dry-run must not actually create")
	assert.Equal(t, 0, report.Updated, "dry-run must not actually update")

	// DB is untouched: still 1 row, still carrying the shared stripe id.
	assert.Equal(t, int64(1), countBatchLicenses(t, db, batch.ID),
		"dry-run must not create license rows")
	assert.Equal(t, int64(1), countBatchLicensesWithStripeID(t, db, batch.ID),
		"dry-run must not null the shared stripe id")
}

// 4. Non-legacy safety: a healthy post-!269 batch is skipped, untouched.
func TestBulkLicenseBackfill_HealthyBatch_SkippedAndUntouched(t *testing.T) {
	db := freshTestDB(t)

	priceID := "price_healthy_" + uuid.NewString()
	plan := &models.SubscriptionPlan{
		Name: "Healthy Plan", PriceAmount: 1999, Currency: "eur",
		BillingInterval: "month", StripePriceID: &priceID, IsActive: true,
	}
	require.NoError(t, db.Create(plan).Error)

	purchaser := "purchaser_" + uuid.NewString()
	batch := &models.SubscriptionBatch{
		PurchaserUserID: purchaser, SubscriptionPlanID: plan.ID,
		StripeSubscriptionID: "sub_healthy_" + uuid.NewString(), StripeSubscriptionItemID: "si_healthy",
		TotalQuantity: 3, AssignedQuantity: 0, Status: "active",
		CurrentPeriodStart: time.Now(), CurrentPeriodEnd: time.Now().Add(30 * 24 * time.Hour),
	}
	require.NoError(t, db.Create(batch).Error)

	// 3 healthy licenses: NULL stripe id, unassigned (the post-!269 shape).
	for i := 0; i < 3; i++ {
		require.NoError(t, db.Create(&models.UserSubscription{
			UserID: "", PurchaserUserID: &purchaser, SubscriptionBatchID: &batch.ID,
			SubscriptionPlanID: plan.ID, Status: "unassigned",
			CurrentPeriodStart: batch.CurrentPeriodStart, CurrentPeriodEnd: batch.CurrentPeriodEnd,
		}).Error)
	}

	report, err := backfill.RunBulkLicenses(db, backfill.Options{Apply: true})
	require.NoError(t, err)

	assert.GreaterOrEqual(t, report.Skipped, 1, "a fully-provisioned NULL-id batch must be skipped")
	assert.Equal(t, 0, report.Created, "healthy batch: nothing to create")
	assert.Equal(t, 0, report.Updated, "healthy batch: nothing to repair")

	assert.Equal(t, int64(3), countBatchLicenses(t, db, batch.ID), "healthy batch row count unchanged")
	assert.Equal(t, int64(0), countBatchLicensesWithStripeID(t, db, batch.ID), "healthy batch has no stripe ids to null")
}

// 5. Assigned-license preservation: an already-assigned legacy license keeps its
// assignment; backfilled rows are unassigned.
func TestBulkLicenseBackfill_AssignedLegacyLicense_PreservesAssignment(t *testing.T) {
	db := freshTestDB(t)
	batch, _ := seedLegacyBulkBatch(t, db, 3)

	// Turn the single legacy license into an ASSIGNED one (as AssignLicense does:
	// UserID set, status active, subscription_type assigned), still carrying the
	// shared stripe id (legacy shape). Bump the batch's assigned counter to match.
	assignee := "learner_" + uuid.NewString()
	require.NoError(t, db.Model(&models.UserSubscription{}).
		Where("subscription_batch_id = ?", batch.ID).
		Updates(map[string]any{
			"user_id":           assignee,
			"status":            "active",
			"subscription_type": "assigned",
		}).Error)
	require.NoError(t, db.Model(&models.SubscriptionBatch{}).
		Where("id = ?", batch.ID).Update("assigned_quantity", 1).Error)

	report, err := backfill.RunBulkLicenses(db, backfill.Options{Apply: true})
	require.NoError(t, err)

	// The assignment must survive: same row still assigned to the same user.
	var assignedCount int64
	require.NoError(t, db.Model(&models.UserSubscription{}).
		Where("subscription_batch_id = ? AND user_id = ? AND status = ?", batch.ID, assignee, "active").
		Count(&assignedCount).Error)
	assert.Equal(t, int64(1), assignedCount,
		"backfill must PRESERVE an already-assigned license (same user, still active)")

	// Batch fully provisioned; the 2 backfilled rows are unassigned/assignable.
	assert.Equal(t, int64(3), countBatchLicenses(t, db, batch.ID),
		"assigned legacy batch must be topped up to total_quantity")
	assert.Equal(t, int64(2), countAssignableLicenses(t, db, batch.ID),
		"the 2 backfilled rows must be unassigned (assignable); the 3rd stays assigned")

	// Shared stripe id cleared everywhere, including on the assigned row.
	assert.Equal(t, int64(0), countBatchLicensesWithStripeID(t, db, batch.ID),
		"backfill must null the shared stripe id even on the assigned legacy row")

	assert.Equal(t, 2, report.Created, "2 backfilled rows")
	assert.Equal(t, 1, report.Updated, "1 legacy row repaired (stripe id nulled)")
}

// -----------------------------------------------------------------------------
// Status-safety follow-up (reviewer): RunBulkLicenses loads batches with NO
// status filter, so a legacy CANCELLED/EXPIRED batch would get its missing seats
// backfilled as status='unassigned' — assignable via AssignLicense — granting
// entitlements on a subscription that is no longer paid. These tests pin that
// only ACTIVE/live batches get seats backfilled; dead batches are repaired (id
// nulled, harmless) but never grown, anomalous batches are flagged, and no path
// ever deletes rows.
// -----------------------------------------------------------------------------

// findBatchReport returns the per-batch report entry for batchID, or nil.
func findBatchReport(report *backfill.BulkLicenseReport, batchID uuid.UUID) *backfill.BulkLicenseBatchReport {
	for i := range report.Batches {
		if report.Batches[i].BatchID == batchID.String() {
			return &report.Batches[i]
		}
	}
	return nil
}

// batchReportStatus reads an optional Status field off the per-batch report via
// reflection, so the test compiles whether or not the dev has added it yet.
// Returns (value, true) only when a string field named "Status" is present.
func batchReportStatus(entry *backfill.BulkLicenseBatchReport) (string, bool) {
	f := reflect.ValueOf(*entry).FieldByName("Status")
	if !f.IsValid() || f.Kind() != reflect.String {
		return "", false
	}
	return f.String(), true
}

// seedBatchWithStatus creates a batch with an explicit status and `licenses`
// license rows carrying the shared stripe id (legacy shape). licenses may be 0.
func seedBatchWithStatus(t *testing.T, db *gorm.DB, status string, totalQuantity, licenses int) (*models.SubscriptionBatch, string) {
	t.Helper()
	priceID := "price_bf_" + uuid.NewString()
	plan := &models.SubscriptionPlan{
		Name: "BF Status Plan", PriceAmount: 1999, Currency: "eur",
		BillingInterval: "month", StripePriceID: &priceID, IsActive: true,
	}
	require.NoError(t, db.Create(plan).Error)

	purchaser := "purchaser_" + uuid.NewString()
	sharedStripeSubID := "sub_bf_" + uuid.NewString()
	customerID := "cus_bf_" + uuid.NewString()

	batch := &models.SubscriptionBatch{
		PurchaserUserID: purchaser, SubscriptionPlanID: plan.ID,
		StripeSubscriptionID: sharedStripeSubID, StripeSubscriptionItemID: "si_bf",
		TotalQuantity: totalQuantity, AssignedQuantity: 0, Status: status,
		CurrentPeriodStart: time.Now(), CurrentPeriodEnd: time.Now().Add(30 * 24 * time.Hour),
	}
	require.NoError(t, db.Create(batch).Error)

	// Only the FIRST license may carry the shared (non-null) stripe id — the
	// partial unique index forbids a second non-null duplicate (that is exactly
	// why legacy batches kept only one row).
	for i := 0; i < licenses; i++ {
		lic := &models.UserSubscription{
			UserID: "", PurchaserUserID: &purchaser, SubscriptionBatchID: &batch.ID,
			SubscriptionPlanID: plan.ID, StripeCustomerID: &customerID, Status: "unassigned",
			CurrentPeriodStart: batch.CurrentPeriodStart, CurrentPeriodEnd: batch.CurrentPeriodEnd,
		}
		if i == 0 {
			lic.StripeSubscriptionID = &sharedStripeSubID
		}
		require.NoError(t, db.Create(lic).Error)
	}
	return batch, sharedStripeSubID
}

// 6. Dead (cancelled/expired) legacy batch: repair the shared id (harmless) but
// NEVER backfill new assignable seats.
func TestBulkLicenseBackfill_CancelledBatch_RepairsButNeverBackfills(t *testing.T) {
	for _, status := range []string{"cancelled", "expired"} {
		t.Run(status, func(t *testing.T) {
			db := freshTestDB(t)
			batch, _ := seedBatchWithStatus(t, db, status, 3, 1)

			require.Equal(t, int64(1), countBatchLicenses(t, db, batch.ID), "precondition: 1 legacy license")
			require.Equal(t, int64(1), countBatchLicensesWithStripeID(t, db, batch.ID), "precondition: shared stripe id present")

			report, err := backfill.RunBulkLicenses(db, backfill.Options{Apply: true})
			require.NoError(t, err)

			// Repair still applies (removes the weird shared id) — harmless.
			assert.Equal(t, int64(0), countBatchLicensesWithStripeID(t, db, batch.ID),
				"a dead batch's shared stripe id may still be nulled (harmless cleanup)")

			// But NO new seats: a %s batch is not paid, so backfilling assignable
			// rows would grant entitlements on an unpaid subscription.
			assert.Equal(t, int64(1), countBatchLicenses(t, db, batch.ID),
				"SAFETY: a "+status+" batch must NEVER be backfilled — no new rows. "+
					"Today RunBulkLicenses loads batches with no status filter and "+
					"creates total_quantity-existing assignable rows.")

			br := findBatchReport(report, batch.ID)
			require.NotNil(t, br, "the batch must appear in the report")
			assert.Equal(t, 0, br.Created, "no seats created for a "+status+" batch")

			// Loosely pin the proposed BulkLicenseBatchReport.Status field: assert
			// it only if the dev has added it (keeps this compiling either way).
			if s, ok := batchReportStatus(br); ok {
				assert.Equal(t, status, s, "per-batch report Status should carry the batch status")
			} else {
				t.Logf("nudge: add a `Status string` field to BulkLicenseBatchReport so "+
					"operators can see WHY a batch was skipped (here: %q)", status)
			}
		})
	}
}

// 7. Anomalous active batch: total_quantity>0 but ZERO license rows (no
// StripeCustomerID source). Must be skipped/flagged, not silently backfilled
// with nil-customer rows.
func TestBulkLicenseBackfill_ZeroLicenseBatch_SkippedAndFlagged(t *testing.T) {
	db := freshTestDB(t)
	batch, _ := seedBatchWithStatus(t, db, "active", 2, 0)

	require.Equal(t, int64(0), countBatchLicenses(t, db, batch.ID), "precondition: zero license rows")

	report, err := backfill.RunBulkLicenses(db, backfill.Options{Apply: true})
	require.NoError(t, err)

	assert.Equal(t, int64(0), countBatchLicenses(t, db, batch.ID),
		"SAFETY: a batch with zero license rows has no StripeCustomerID source and "+
			"must NOT be backfilled with nil-customer rows. Today it creates "+
			"total_quantity anomalous rows.")
	assert.Equal(t, 0, report.Created, "nothing may be created for a zero-license batch")
	assert.GreaterOrEqual(t, report.Skipped, 1, "a zero-license batch must be reported as skipped")

	br := findBatchReport(report, batch.ID)
	require.NotNil(t, br, "the batch must appear in the report")
	assert.Equal(t, 0, br.Created, "zero seats created for a zero-license batch")
	// The skip must be distinguishable from a healthy skip — don't over-pin the
	// wording, just require the reason not to claim the batch is healthy.
	assert.NotContains(t, strings.ToLower(br.Reason), "healthy",
		"a zero-license batch must be flagged with a reason distinct from 'healthy'")
}

// 8. Guard: a shrunk batch (total_quantity < existing rows, from seat removal
// history) must never have rows DELETED. Expected GREEN today (the toCreate
// clamp already prevents negative creation and nothing deletes).
func TestBulkLicenseBackfill_ShrunkBatch_NeverDeletesRows(t *testing.T) {
	db := freshTestDB(t)

	priceID := "price_shrunk_" + uuid.NewString()
	plan := &models.SubscriptionPlan{
		Name: "Shrunk Plan", PriceAmount: 1999, Currency: "eur",
		BillingInterval: "month", StripePriceID: &priceID, IsActive: true,
	}
	require.NoError(t, db.Create(plan).Error)

	purchaser := "purchaser_" + uuid.NewString()
	batch := &models.SubscriptionBatch{
		PurchaserUserID: purchaser, SubscriptionPlanID: plan.ID,
		StripeSubscriptionID: "sub_shrunk_" + uuid.NewString(), StripeSubscriptionItemID: "si_shrunk",
		TotalQuantity: 1, AssignedQuantity: 0, Status: "active",
		CurrentPeriodStart: time.Now(), CurrentPeriodEnd: time.Now().Add(30 * 24 * time.Hour),
	}
	require.NoError(t, db.Create(batch).Error)

	// 2 healthy rows (NULL stripe id, unassigned) but total_quantity is only 1.
	for i := 0; i < 2; i++ {
		require.NoError(t, db.Create(&models.UserSubscription{
			UserID: "", PurchaserUserID: &purchaser, SubscriptionBatchID: &batch.ID,
			SubscriptionPlanID: plan.ID, Status: "unassigned",
			CurrentPeriodStart: batch.CurrentPeriodStart, CurrentPeriodEnd: batch.CurrentPeriodEnd,
		}).Error)
	}

	report, err := backfill.RunBulkLicenses(db, backfill.Options{Apply: true})
	require.NoError(t, err)

	assert.Equal(t, int64(2), countBatchLicenses(t, db, batch.ID),
		"a shrunk batch must NEVER have rows deleted — the migration only ever adds")
	assert.Equal(t, 0, report.Created, "shrunk batch: nothing to create")
}
