// tests/payment/backfillGroupManagement_test.go
//
// RED test for the startup backfill: BackfillGroupManagementEntitlement must set
// GroupManagementEnabled=true on every plan whose legacy features[] still lists
// "group_management", so the typed entitlement matches the historical string
// during the migration. Idempotent, following the ensureXXX/Backfill patterns in
// initialization/database.go.
//
// RED today: the skeleton is a no-op, so the legacy plan's bool stays false.
package payment_tests

import (
	"testing"

	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/initialization"
	"soli/formations/src/payment/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBackfillGroupManagementEntitlement_SetsBoolFromLegacyString(t *testing.T) {
	db := freshTestDB(t)

	// Legacy-style plan: features[] carries "group_management", typed bool false.
	legacy := &models.SubscriptionPlan{
		BaseModel:       entityManagementModels.BaseModel{ID: uuid.New()},
		Name:            "Legacy Group Plan",
		Currency:        "eur",
		BillingInterval: "month",
		IsActive:        true,
		Features:        []string{"group_management", "network_access"},
	}
	require.NoError(t, db.Create(legacy).Error)

	// A plan WITHOUT the legacy string must be left untouched.
	other := &models.SubscriptionPlan{
		BaseModel:       entityManagementModels.BaseModel{ID: uuid.New()},
		Name:            "No Group Plan",
		Currency:        "eur",
		BillingInterval: "month",
		IsActive:        true,
		Features:        []string{"network_access"},
	}
	require.NoError(t, db.Create(other).Error)

	initialization.BackfillGroupManagementEntitlement(db)

	var migrated models.SubscriptionPlan
	require.NoError(t, db.First(&migrated, "id = ?", legacy.ID).Error)
	assert.True(t, migrated.GroupManagementEnabled,
		"backfill must set GroupManagementEnabled=true for a plan whose features[] lists group_management")

	var untouched models.SubscriptionPlan
	require.NoError(t, db.First(&untouched, "id = ?", other.ID).Error)
	assert.False(t, untouched.GroupManagementEnabled,
		"backfill must NOT set the bool on a plan without the legacy group_management string")

	// Idempotent: a second run must not error or change the outcome.
	initialization.BackfillGroupManagementEntitlement(db)
	require.NoError(t, db.First(&migrated, "id = ?", legacy.ID).Error)
	assert.True(t, migrated.GroupManagementEnabled, "backfill must be idempotent")
}
