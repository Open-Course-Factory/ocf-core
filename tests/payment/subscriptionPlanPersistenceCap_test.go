// tests/payment/subscriptionPlanPersistenceCap_test.go
//
// TASK 2 (owner: "put a cap at 500 GB"): a plan create/update must reject
// DataPersistenceGB > 500. binding tags are INERT on the generic entity routes
// (the JSON binds into an `any`, so the struct validator never runs — #390), so
// the enforcement seam is a BeforeCreate/BeforeUpdate validation hook, mirroring
// BillingAddressValidationHook. The seam type lives at
// src/payment/hooks/subscriptionPlanValidationHook.go (currently a SKELETON that
// enforces nothing — these tests are RED against it).
//
// Two levels of observable pin:
//   (1) the hook's Execute returns a validation error for > 500 (create shape and
//       update-patch shape) and nil at/below 500 — the boundary is 500 ok / 501
//       rejected;
//   (2) end-to-end, creating a plan with data_persistence_gb=501 through the REAL
//       generic service (with the payment hooks wired via InitPaymentHooks) is
//       rejected and nothing is persisted.
//
// Scope note: the owner ask is the UPPER cap (> 500). Negative values are NOT
// validated anywhere today (no plan hook existed; binding tags are inert); this
// change is deliberately scoped to the cap, so negatives are left as-is.
package payment_tests

import (
	"testing"

	"soli/formations/src/entityManagement/hooks"
	entityServices "soli/formations/src/entityManagement/services"
	"soli/formations/src/payment/dto"
	paymentHooks "soli/formations/src/payment/hooks"
	"soli/formations/src/payment/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// execPlanValidation runs the plan validation hook's Execute against the given
// lifecycle payload and returns its error (nil = accepted).
func execPlanValidation(hookType hooks.HookType, newEntity any) error {
	h := paymentHooks.NewSubscriptionPlanValidationHook(nil)
	return h.Execute(&hooks.HookContext{
		EntityName: "SubscriptionPlan",
		HookType:   hookType,
		NewEntity:  newEntity,
	})
}

// TestSubscriptionPlanValidation_CreateCapsDataPersistenceGB pins the create
// path: the converted *models.SubscriptionPlan carries DataPersistenceGB and the
// hook rejects > 500. RED: the skeleton hook accepts everything.
func TestSubscriptionPlanValidation_CreateCapsDataPersistenceGB(t *testing.T) {
	cases := []struct {
		name     string
		gb       int
		rejected bool
	}{
		{"zero_ok", 0, false},
		{"mid_ok", 250, false},
		{"boundary_500_ok", 500, false},
		{"boundary_501_rejected", 501, true},
		{"far_over_rejected", 100000, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := execPlanValidation(hooks.BeforeCreate, &models.SubscriptionPlan{
				Name:              "Cap Plan",
				DataPersistenceGB: tc.gb,
			})
			if tc.rejected {
				require.Error(t, err,
					"DataPersistenceGB=%d exceeds the 500 GB cap and must be rejected on create", tc.gb)
			} else {
				require.NoError(t, err,
					"DataPersistenceGB=%d is within the 500 GB cap and must be accepted on create", tc.gb)
			}
		})
	}
}

// TestSubscriptionPlanValidation_UpdatePatchCapsDataPersistenceGB pins the update
// path: the generic PATCH path supplies a patch map keyed by the json/mapstructure
// field name; the hook must read data_persistence_gb from it and reject > 500. A
// patch that omits the key is a partial update and must not be rejected. RED: the
// skeleton hook accepts everything.
func TestSubscriptionPlanValidation_UpdatePatchCapsDataPersistenceGB(t *testing.T) {
	// > 500 rejected
	require.Error(t,
		execPlanValidation(hooks.BeforeUpdate, map[string]any{"data_persistence_gb": 501}),
		"a PATCH raising data_persistence_gb above 500 must be rejected")

	// boundary 500 accepted
	require.NoError(t,
		execPlanValidation(hooks.BeforeUpdate, map[string]any{"data_persistence_gb": 500}),
		"a PATCH setting data_persistence_gb to exactly 500 must be accepted")

	// pointer shape (the generic PATCH decodes pointer-field Edit DTOs, leaving *int)
	over := 501
	require.Error(t,
		execPlanValidation(hooks.BeforeUpdate, map[string]any{"data_persistence_gb": &over}),
		"a PATCH raising data_persistence_gb above 500 (pointer value) must be rejected")

	// absent key = partial update, not validated
	require.NoError(t,
		execPlanValidation(hooks.BeforeUpdate, map[string]any{"name": "Renamed"}),
		"a PATCH that omits data_persistence_gb must not be rejected")
}

// TestSubscriptionPlan_CreateOver500_RejectedEndToEnd drives the REAL generic
// create service with the payment hooks wired (via InitPaymentHooks, exactly as
// production wires them) and asserts a 501 GB plan is rejected and not persisted.
// RED until the validation hook exists AND is registered in InitPaymentHooks.
func TestSubscriptionPlan_CreateOver500_RejectedEndToEnd(t *testing.T) {
	_ = freshTestDB(t)
	registerSubscriptionPlanForScoping(t)
	withPaymentHooksRegistered(t)

	svc := entityServices.NewGenericService(sharedTestDB, nil)
	_, err := svc.CreateEntityWithUser(dto.CreateSubscriptionPlanInput{
		Name:              "Over Cap Plan",
		PriceAmount:       1000,
		Currency:          "eur",
		BillingInterval:   "month",
		DataPersistenceGB: 501,
	}, "SubscriptionPlan", "admin-1")
	require.Error(t, err, "a plan requesting 501 GB persistence must be rejected at create")

	var count int64
	require.NoError(t, sharedTestDB.Model(&models.SubscriptionPlan{}).
		Where("name = ?", "Over Cap Plan").Count(&count).Error)
	assert.Equal(t, int64(0), count, "a rejected over-cap plan must not be persisted")
}

// TestSubscriptionPlan_CreateAt500_AcceptedEndToEnd is the GREEN-side guard that
// the cap does not over-reject: a plan at exactly 500 GB is created. Passes today
// (no enforcement) and must keep passing after the cap lands.
func TestSubscriptionPlan_CreateAt500_AcceptedEndToEnd(t *testing.T) {
	_ = freshTestDB(t)
	registerSubscriptionPlanForScoping(t)
	withPaymentHooksRegistered(t)

	svc := entityServices.NewGenericService(sharedTestDB, nil)
	_, err := svc.CreateEntityWithUser(dto.CreateSubscriptionPlanInput{
		Name:              "At Cap Plan",
		PriceAmount:       1000,
		Currency:          "eur",
		BillingInterval:   "month",
		DataPersistenceGB: 500,
	}, "SubscriptionPlan", "admin-1")
	require.NoError(t, err, "a plan at exactly 500 GB must be accepted")

	var count int64
	require.NoError(t, sharedTestDB.Model(&models.SubscriptionPlan{}).
		Where("name = ?", "At Cap Plan").Count(&count).Error)
	assert.Equal(t, int64(1), count, "an at-cap plan must be persisted")
}
