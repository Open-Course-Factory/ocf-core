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

// Direction 1: typed fields alone govern bulk purchase — the legacy features[]
// string is not consulted. Since #441 the product side is BulkPurchasable and
// the eligibility side is the purchaser's GroupManagementEnabled, so this pins
// both typed fields doing the work with an empty features[].
func TestBulkPurchase_TypedFieldsGate_NoLegacyString_Allowed(t *testing.T) {
	db := freshTestDB(t)
	installFakeCasdoor(t, "boolgate@example.com", "Bool Gate Buyer")
	svc := services.NewBulkLicenseServiceWithDeps(db, &bulkGatesStripeStub{})

	const buyer = "buyer-boolgate"
	seedTrainerWithGroupManagement(t, db, buyer)

	planID := uuid.New()
	plan := &models.SubscriptionPlan{
		BaseModel:       entityManagementModels.BaseModel{ID: planID},
		Name:            "Typed Seat Plan",
		Currency:        "eur",
		IsActive:        true,
		IsCatalog:       true,
		BulkPurchasable: true,
		// No legacy features[] — the typed fields alone must gate.
	}
	require.NoError(t, db.Create(plan).Error)

	batch, licenses, err := svc.PurchaseBulkLicenses(buyer, dto.BulkPurchaseInput{
		SubscriptionPlanID: planID,
		Quantity:           3,
	})

	require.NoError(t, err,
		"a BulkPurchasable plan bought by a trainer whose plan grants group management must "+
			"succeed with an empty features[] — typed fields gate, not the legacy string")
	assert.NotNil(t, batch, "a batch must be created when the typed fields permit bulk purchase")
	assert.NotNil(t, licenses, "licenses must be created when the typed fields permit bulk purchase")
}

// Direction 2: the legacy features[] string confers nothing. A plan carrying
// "group_management" in the raw column is still not a seat product, so it must
// be rejected even for a purchaser who is otherwise eligible — which isolates
// the string as the only thing under test.
func TestBulkPurchase_LegacyStringOnly_NotBulkPurchasable_Rejected(t *testing.T) {
	db := freshTestDB(t)
	installFakeCasdoor(t, "legacygate@example.com", "Legacy Gate Buyer")
	svc := services.NewBulkLicenseServiceWithDeps(db, &bulkGatesStripeStub{})

	const buyer = "buyer-legacygate"
	seedTrainerWithGroupManagement(t, db, buyer)

	planID := uuid.New()
	plan := &models.SubscriptionPlan{
		BaseModel: entityManagementModels.BaseModel{ID: planID},
		Name:      "Legacy String Plan",
		Currency:  "eur",
		IsActive:  true,
		IsCatalog: true,
		// BulkPurchasable defaults to false — the typed product flag is absent.
	}
	require.NoError(t, db.Create(plan).Error)
	// The legacy "group_management" string lives ONLY in the raw features column
	// (seeded directly so this test survives the model.Features field removal).
	// It must NOT re-enable bulk purchase now that the gate is typed.
	seedLegacyFeaturesColumn(t, db, planID, `["group_management"]`)

	batch, licenses, err := svc.PurchaseBulkLicenses(buyer, dto.BulkPurchaseInput{
		SubscriptionPlanID: planID,
		Quantity:           3,
	})

	require.Error(t, err,
		"a plan that is not BulkPurchasable must be rejected even if the legacy features[] still "+
			"lists group_management — the string no longer gates anything")
	assert.Nil(t, batch, "no batch may be returned when the typed product flag is absent")
	assert.Nil(t, licenses, "no licenses may be returned when the typed product flag is absent")

	var batchCount int64
	require.NoError(t, db.Model(&models.SubscriptionBatch{}).Count(&batchCount).Error)
	assert.Equal(t, int64(0), batchCount, "a rejected bulk purchase must persist no batch")
}
