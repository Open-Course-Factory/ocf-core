// tests/payment/dropOrphanPlanColumns_test.go
//
// TASK 3 (owner: "small drop migration ok"): drop the subscription_plans columns
// whose Go model fields were removed across prior campaigns but which still exist
// physically in prod — max_concurrent_users, allowed_templates, max_courses,
// planned_features, features, and (once Task 1 removes their fields)
// addon_network_price_id / addon_storage_price_id / addon_terminal_price_id.
//
// CRITICAL COUPLING pinned here: the `features` column is STILL read by
// BackfillGroupManagementEntitlement. A naive drop breaks the backfill. The
// migration must therefore run a FINAL group-management backfill pass (reading
// the raw `features` column while it still exists) BEFORE dropping, and must be
// idempotent. The seam under test is the exported initialization.DropOrphanPlanColumns
// (currently a SKELETON that only runs the pre-existing 6-column drop — these
// tests are RED against it).
//
// ISOLATION: these tests DROP columns, so they use an isolated in-memory DB
// (cappedTestDB) rather than the shared payment DB — dropping `features` from the
// shared DB would corrupt sibling tests (backfill, legacy-string guards) that
// re-create and read that orphan column.
//
// SQLITE / DROP COLUMN — MECHANISM PIN (verified by probe, see below):
//   - raw `ALTER TABLE subscription_plans DROP COLUMN <col>` DOES work on the
//     test-env SQLite (gorm.io/driver/sqlite v1.6.0) and on Postgres.
//   - GORM's `db.Migrator().DropColumn(&model, col)` is a SILENT NO-OP on this
//     SQLite driver (returns nil, column survives) — which is exactly what the
//     pre-existing dropOrphan* migrations call. That is harmless in prod
//     (Postgres) and in the current tests (those legacy columns never exist in
//     the test DB), but it means GREEN CANNOT reuse migrator.DropColumn here and
//     have these SQLite tests pass.
//   So the seam GREEN must implement is: guard on `migrator.HasColumn` (for
//   idempotency, replacing `DROP COLUMN IF EXISTS` which SQLite lacks) then raw
//   `db.Exec("ALTER TABLE subscription_plans DROP COLUMN " + col)`. The final
//   group-management backfill pass is dialect-agnostic (raw SELECT/UPDATE).
//   TestRawDropColumn_MechanismPin anchors this and stays green.
package payment_tests

import (
	"testing"

	"soli/formations/src/initialization"
	"soli/formations/src/payment/models"

	entityManagementModels "soli/formations/src/entityManagement/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// orphanPlanColumns are the 8 columns Task 3 must drop from subscription_plans.
var orphanPlanColumns = []string{
	"max_concurrent_users",
	"allowed_templates",
	"max_courses",
	"planned_features",
	"features",
	"addon_network_price_id",
	"addon_storage_price_id",
	"addon_terminal_price_id",
}

// ensureOrphanColumn adds an orphan column to subscription_plans if the table
// doesn't already have it, mirroring prod (where these columns physically exist
// even though no Go field maps them). Type affinity is irrelevant to SQLite, so
// TEXT is fine for all of them.
func ensureOrphanColumn(t *testing.T, db *gorm.DB, col string) {
	t.Helper()
	if db.Migrator().HasColumn(&models.SubscriptionPlan{}, col) {
		return
	}
	require.NoError(t, db.Exec("ALTER TABLE subscription_plans ADD COLUMN "+col+" TEXT").Error,
		"failed to seed orphan column %q", col)
}

func ensureAllOrphanColumns(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, col := range orphanPlanColumns {
		ensureOrphanColumn(t, db, col)
	}
}

// TestRawDropColumn_MechanismPin anchors the drop mechanism GREEN must use: a
// raw `ALTER TABLE ... DROP COLUMN` works on the test SQLite, whereas GORM's
// migrator.DropColumn silently no-ops on this driver. This test stays GREEN and
// documents the seam (guard with HasColumn, then raw Exec).
func TestRawDropColumn_MechanismPin(t *testing.T) {
	db := cappedTestDB(t, t.Name())

	require.NoError(t, db.Exec("ALTER TABLE subscription_plans ADD COLUMN probe_col TEXT").Error)
	require.True(t, db.Migrator().HasColumn(&models.SubscriptionPlan{}, "probe_col"))

	// The mechanism GREEN must use: raw DROP COLUMN, guarded by HasColumn.
	require.NoError(t, db.Exec("ALTER TABLE subscription_plans DROP COLUMN probe_col").Error,
		"raw ALTER TABLE ... DROP COLUMN must succeed on the test SQLite")
	assert.False(t, db.Migrator().HasColumn(&models.SubscriptionPlan{}, "probe_col"),
		"probe_col must be gone after a raw DROP COLUMN")

	// Contrast: GORM's migrator.DropColumn is a silent no-op on this driver — the
	// column survives with no error. GREEN must NOT rely on it for these drops.
	require.NoError(t, db.Exec("ALTER TABLE subscription_plans ADD COLUMN probe_col2 TEXT").Error)
	_ = db.Migrator().DropColumn(&models.SubscriptionPlan{}, "probe_col2")
	assert.True(t, db.Migrator().HasColumn(&models.SubscriptionPlan{}, "probe_col2"),
		"migrator.DropColumn is a no-op on gorm.io/driver/sqlite here — documenting why GREEN uses raw Exec")
}

// TestDropOrphanPlanColumns_DropsAllOrphanColumns pins that the migration removes
// every orphan column. RED: the skeleton drops none of them.
func TestDropOrphanPlanColumns_DropsAllOrphanColumns(t *testing.T) {
	db := cappedTestDB(t, t.Name())
	ensureAllOrphanColumns(t, db)

	initialization.DropOrphanPlanColumns(db)

	for _, col := range orphanPlanColumns {
		assert.False(t, db.Migrator().HasColumn(&models.SubscriptionPlan{}, col),
			"orphan column subscription_plans.%s must be dropped by the migration", col)
	}
}

// TestDropOrphanPlanColumns_RunsFinalBackfillBeforeDrop pins the critical
// coupling: a plan whose raw `features` still carries "group_management" (bool
// still false) must have GroupManagementEnabled flipped to true by the
// migration's FINAL backfill pass BEFORE `features` is dropped. RED: the skeleton
// neither backfills nor drops `features`.
func TestDropOrphanPlanColumns_RunsFinalBackfillBeforeDrop(t *testing.T) {
	db := cappedTestDB(t, t.Name())
	ensureAllOrphanColumns(t, db)

	legacyID := uuid.New()
	legacy := &models.SubscriptionPlan{
		BaseModel:       entityManagementModels.BaseModel{ID: legacyID},
		Name:            "Legacy Group Plan",
		Currency:        "eur",
		BillingInterval: "month",
		IsActive:        true,
	}
	require.NoError(t, db.Create(legacy).Error)
	// Legacy value lives ONLY in the raw features column; typed bool defaults false.
	seedLegacyFeaturesColumn(t, db, legacyID, `["group_management","network_access"]`)

	// A plan without the legacy string must stay untouched.
	otherID := uuid.New()
	require.NoError(t, db.Create(&models.SubscriptionPlan{
		BaseModel:       entityManagementModels.BaseModel{ID: otherID},
		Name:            "No Group Plan",
		Currency:        "eur",
		BillingInterval: "month",
		IsActive:        true,
	}).Error)
	seedLegacyFeaturesColumn(t, db, otherID, `["network_access"]`)

	initialization.DropOrphanPlanColumns(db)

	// Final backfill pass ran before the drop:
	var migrated models.SubscriptionPlan
	require.NoError(t, db.First(&migrated, "id = ?", legacyID).Error)
	assert.True(t, migrated.GroupManagementEnabled,
		"the migration's final backfill pass must flip GroupManagementEnabled=true from the raw features column before dropping it")

	var untouched models.SubscriptionPlan
	require.NoError(t, db.First(&untouched, "id = ?", otherID).Error)
	assert.False(t, untouched.GroupManagementEnabled,
		"a plan whose raw features lack group_management must be left untouched")

	// ...and the coupled column is now gone.
	assert.False(t, db.Migrator().HasColumn(&models.SubscriptionPlan{}, "features"),
		"features must be dropped after the final backfill pass has read it")
}

// TestDropOrphanPlanColumns_Idempotent pins that a second run is a safe no-op
// (columns already gone → no error), and that an already-migrated plan
// (GroupManagementEnabled=true, no legacy features) is left untouched. RED: the
// skeleton leaves the orphan columns in place, so the post-conditions fail.
func TestDropOrphanPlanColumns_Idempotent(t *testing.T) {
	db := cappedTestDB(t, t.Name())
	ensureAllOrphanColumns(t, db)

	// A plan already migrated to the typed bool must not regress or error.
	doneID := uuid.New()
	require.NoError(t, db.Create(&models.SubscriptionPlan{
		BaseModel:              entityManagementModels.BaseModel{ID: doneID},
		Name:                   "Already Migrated",
		Currency:               "eur",
		BillingInterval:        "month",
		IsActive:               true,
		GroupManagementEnabled: true,
	}).Error)

	initialization.DropOrphanPlanColumns(db)
	// Second run must not panic or error and must leave columns dropped.
	require.NotPanics(t, func() { initialization.DropOrphanPlanColumns(db) },
		"a second migration run must be a safe no-op")

	for _, col := range orphanPlanColumns {
		assert.False(t, db.Migrator().HasColumn(&models.SubscriptionPlan{}, col),
			"orphan column subscription_plans.%s must remain dropped after a second run", col)
	}

	var done models.SubscriptionPlan
	require.NoError(t, db.First(&done, "id = ?", doneID).Error)
	assert.True(t, done.GroupManagementEnabled,
		"an already-migrated plan must be left untouched")
}
