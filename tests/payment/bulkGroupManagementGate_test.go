// tests/payment/bulkGroupManagementGate_test.go
//
// RED tests for the bulk-purchase eligibility MIGRATION: the gate must read the
// typed plan.GroupManagementEnabled field instead of
// slices.Contains(plan.Features, "group_management"). Both directions are
// pinned so the legacy string stops gating and the typed bool starts gating.
//
// Drives the REAL PurchaseBulkLicenses via the shared bulkGatesStripeStub +
// installFakeCasdoor (from bulkPurchaseGates_test.go), asserting user-observable
// outcomes (purchase allowed/rejected + row persistence), never a mock call.
package payment_tests

import (
	"testing"

	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Direction 1: the typed bool alone makes a plan bulk-purchasable — the legacy
// features[] string is NO LONGER required. RED today: the gate still requires
// the "group_management" string, so a plan with GroupManagementEnabled=true but
// an empty Features array is wrongly rejected.
func TestBulkPurchase_GroupManagementBoolTrue_NoLegacyString_Allowed(t *testing.T) {
	db := freshTestDB(t)
	installFakeCasdoor(t, "boolgate@example.com", "Bool Gate Buyer")
	svc := services.NewBulkLicenseServiceWithDeps(db, &bulkGatesStripeStub{})

	planID := uuid.New()
	plan := &models.SubscriptionPlan{
		BaseModel: entityManagementModels.BaseModel{ID: planID},
		Name:      "Typed Group Mgmt Plan",
		Currency:  "eur",
		IsActive:  true,
		IsCatalog: true,
		// No legacy features[] — the typed bool alone must gate.
	}
	require.NoError(t, db.Create(plan).Error)
	// GORM skips the zero-value bool on a default field at Create; force it true.
	require.NoError(t, db.Model(plan).Update("group_management_enabled", true).Error)

	batch, licenses, err := svc.PurchaseBulkLicenses("buyer-boolgate", dto.BulkPurchaseInput{
		SubscriptionPlanID: planID,
		Quantity:           3,
	})

	require.NoError(t, err,
		"a plan with GroupManagementEnabled=true must be bulk-purchasable even with an empty features[] — the typed bool now gates, not the legacy string")
	assert.NotNil(t, batch, "a batch must be created when the typed entitlement permits bulk purchase")
	assert.NotNil(t, licenses, "licenses must be created when the typed entitlement permits bulk purchase")
}

// Direction 2: the legacy features[] string alone NO LONGER makes a plan
// bulk-purchasable once the gate is typed. RED today: the gate still passes on
// the string, so a plan with GroupManagementEnabled=false but the legacy string
// present is wrongly allowed.
func TestBulkPurchase_LegacyStringOnly_BoolFalse_Rejected(t *testing.T) {
	db := freshTestDB(t)
	installFakeCasdoor(t, "legacygate@example.com", "Legacy Gate Buyer")
	svc := services.NewBulkLicenseServiceWithDeps(db, &bulkGatesStripeStub{})

	planID := uuid.New()
	plan := &models.SubscriptionPlan{
		BaseModel: entityManagementModels.BaseModel{ID: planID},
		Name:      "Legacy String Plan",
		Currency:  "eur",
		IsActive:  true,
		IsCatalog: true,
		// GroupManagementEnabled defaults to false — the typed entitlement is absent
	}
	require.NoError(t, db.Create(plan).Error)
	// The legacy "group_management" string lives ONLY in the raw features column
	// (seeded directly so this test survives the model.Features field removal).
	// It must NOT re-enable bulk purchase now that the gate is typed.
	seedLegacyFeaturesColumn(t, db, planID, `["group_management"]`)

	batch, licenses, err := svc.PurchaseBulkLicenses("buyer-legacygate", dto.BulkPurchaseInput{
		SubscriptionPlanID: planID,
		Quantity:           3,
	})

	require.Error(t, err,
		"a plan with GroupManagementEnabled=false must be rejected even if the legacy features[] still lists group_management — the string no longer gates")
	assert.Nil(t, batch, "no batch may be returned when the typed entitlement is absent")
	assert.Nil(t, licenses, "no licenses may be returned when the typed entitlement is absent")
	assertNoBulkRowsPersisted(t)
}
