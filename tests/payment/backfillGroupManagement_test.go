// tests/payment/backfillGroupManagement_test.go
//
// Guard for the deletion phase: BackfillGroupManagementEntitlement must keep
// flipping GroupManagementEnabled=true for plans whose legacy "group_management"
// lives in the RAW features column — EVEN AFTER model.SubscriptionPlan.Features
// is removed. Prod rows still carry the JSON array at migration time, so the
// backfill can no longer read a Go struct field; it must SELECT the raw
// `features` column (e.g. db.Raw / db.Table).
//
// This test seeds the legacy value straight into the DB column (never the Go
// Features field), so it compiles and asserts the same behaviour before and
// after the field removal — locking the raw-column read as the seam. It is the
// subtle correctness pin of this phase: a naive field removal that also drops
// the backfill's data source would make this fail.
package payment_tests

import (
	"testing"

	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/initialization"
	"soli/formations/src/payment/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// seedLegacyFeaturesColumn writes a raw JSON array directly into the plan's
// `features` column, bypassing the Go model. Used so tests that depend on legacy
// features[] data survive the removal of the model.Features field — the data
// still exists at the column level, exactly as prod rows do at migration time.
func seedLegacyFeaturesColumn(t *testing.T, db *gorm.DB, planID uuid.UUID, jsonArray string) {
	t.Helper()
	require.NoError(t, db.Exec(
		"UPDATE subscription_plans SET features = ? WHERE id = ?", jsonArray, planID,
	).Error)
}

func TestBackfillGroupManagementEntitlement_SetsBoolFromLegacyRawColumn(t *testing.T) {
	db := freshTestDB(t)

	// Legacy-style plan: the "group_management" string lives ONLY in the raw
	// features column, typed bool defaults false.
	legacyID := uuid.New()
	legacy := &models.SubscriptionPlan{
		BaseModel:       entityManagementModels.BaseModel{ID: legacyID},
		Name:            "Legacy Group Plan",
		Currency:        "eur",
		BillingInterval: "month",
		IsActive:        true,
	}
	require.NoError(t, db.Create(legacy).Error)
	seedLegacyFeaturesColumn(t, db, legacyID, `["group_management","network_access"]`)

	// A plan whose raw features lack the string must be left untouched.
	otherID := uuid.New()
	other := &models.SubscriptionPlan{
		BaseModel:       entityManagementModels.BaseModel{ID: otherID},
		Name:            "No Group Plan",
		Currency:        "eur",
		BillingInterval: "month",
		IsActive:        true,
	}
	require.NoError(t, db.Create(other).Error)
	seedLegacyFeaturesColumn(t, db, otherID, `["network_access"]`)

	initialization.BackfillGroupManagementEntitlement(db)

	var migrated models.SubscriptionPlan
	require.NoError(t, db.First(&migrated, "id = ?", legacyID).Error)
	assert.True(t, migrated.GroupManagementEnabled,
		"backfill must set GroupManagementEnabled=true from the raw features column, even without a Go Features field")

	var untouched models.SubscriptionPlan
	require.NoError(t, db.First(&untouched, "id = ?", otherID).Error)
	assert.False(t, untouched.GroupManagementEnabled,
		"backfill must NOT set the bool on a plan whose raw features lack group_management")

	// Idempotent: a second run must not error or change the outcome.
	initialization.BackfillGroupManagementEntitlement(db)
	require.NoError(t, db.First(&migrated, "id = ?", legacyID).Error)
	assert.True(t, migrated.GroupManagementEnabled, "backfill must be idempotent")
}
