// tests/payment/bulkPurchasableBackfill_test.go
//
// #441 introduced BulkPurchasable with a default of false, so a backfill replays
// the rule it replaced — otherwise every bulk purchase already in flight would
// stop working on deploy.
//
// The replay has to reproduce the OLD gate exactly. That gate was
// `IsActive AND IsCatalog AND GroupManagementEnabled`, all three. Replaying only
// the group-management half marks plans that were never sellable: a hidden
// bespoke plan carrying group management was rejected before and would silently
// become bulk-purchasable by any eligible trainer.
//
// Caught on a real dev database, where the bespoke "École / OF" plan came back
// from a restart marked sellable.
package payment_tests

import (
	"testing"

	"soli/formations/src/initialization"
	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/payment/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// seedPlanForBackfill creates a plan with explicit catalog and group-management
// state. Both booleans need a follow-up Update: IsCatalog carries
// gorm:"default:true" and GORM omits zero-value bools on Create, so the DB
// default would otherwise win.
func seedPlanForBackfill(t *testing.T, db *gorm.DB, name string, catalog, groupMgmt bool) *models.SubscriptionPlan {
	t.Helper()
	plan := &models.SubscriptionPlan{
		BaseModel:       entityManagementModels.BaseModel{ID: uuid.New()},
		Name:            name,
		Currency:        "eur",
		PriceAmount:     1990,
		BillingInterval: "month",
		IsActive:        true,
	}
	require.NoError(t, db.Create(plan).Error)
	require.NoError(t, db.Model(plan).Updates(map[string]interface{}{
		"is_catalog":               catalog,
		"group_management_enabled": groupMgmt,
		"bulk_purchasable":         false,
	}).Error)
	return plan
}

func bulkPurchasable(t *testing.T, db *gorm.DB, id uuid.UUID) bool {
	t.Helper()
	var plan models.SubscriptionPlan
	require.NoError(t, db.Where("id = ?", id).First(&plan).Error)
	return plan.BulkPurchasable
}

// TestBulkPurchasableBackfill_MarksOnlyWhatWasAlreadySellable pins the replay
// against the old three-part gate.
func TestBulkPurchasableBackfill_MarksOnlyWhatWasAlreadySellable(t *testing.T) {
	db := freshTestDB(t)

	sellable := seedPlanForBackfill(t, db, "Formateur (catalog + group mgmt)", true, true)
	hidden := seedPlanForBackfill(t, db, "Bespoke org plan (hidden + group mgmt)", false, true)
	plain := seedPlanForBackfill(t, db, "Solo (catalog, no group mgmt)", true, false)

	initialization.BackfillBulkPurchasableFromGroupManagement(db)

	assert.True(t, bulkPurchasable(t, db, sellable.ID),
		"a catalog plan granting group management was bulk-purchasable before and must stay so")
	assert.False(t, bulkPurchasable(t, db, hidden.ID),
		"a HIDDEN plan was rejected by the old IsCatalog gate — the replay must not make it sellable")
	assert.False(t, bulkPurchasable(t, db, plain.ID),
		"a plan without group management was never bulk-purchasable")
}

// TestBulkPurchasableBackfill_DoesNotFightTheAdministrator is the property the
// backfill must have and did not: it seeds the flag ONCE. "Where
// bulk_purchasable = false" is not a once-guard — it is the condition that
// re-marks — so an admin who deliberately unmarked a legacy plan would find it
// sellable again after the next restart.
func TestBulkPurchasableBackfill_DoesNotFightTheAdministrator(t *testing.T) {
	db := freshTestDB(t)

	keep := seedPlanForBackfill(t, db, "Formateur", true, true)
	unmarked := seedPlanForBackfill(t, db, "Legacy plan the admin retires", true, true)

	initialization.BackfillBulkPurchasableFromGroupManagement(db)
	require.True(t, bulkPurchasable(t, db, keep.ID), "first run seeds the flag")
	require.True(t, bulkPurchasable(t, db, unmarked.ID))

	// The admin decides this one should no longer be sold in bulk.
	require.NoError(t, db.Model(unmarked).Update("bulk_purchasable", false).Error)

	initialization.BackfillBulkPurchasableFromGroupManagement(db)

	assert.False(t, bulkPurchasable(t, db, unmarked.ID),
		"a deliberately unmarked plan must stay unmarked across restarts")
	assert.True(t, bulkPurchasable(t, db, keep.ID), "and the others are left alone")
}
