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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	controller "soli/formations/src/entityManagement/routes"
	"soli/formations/src/entityManagement/hooks"
	entityManagementModels "soli/formations/src/entityManagement/models"
	entityServices "soli/formations/src/entityManagement/services"
	"soli/formations/src/payment/dto"
	paymentHooks "soli/formations/src/payment/hooks"
	"soli/formations/src/payment/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
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

// subscriptionPlanEditRouter mounts the REAL generic PATCH handler for
// subscription-plans, so a request exercises the full controller update path:
// BindJSON into the edit DTO → mapstructure decode → ConvertEditDtoToMap →
// EditEntityWithUser (which fires the BeforeUpdate hooks). The caller is a
// platform admin (SubscriptionPlan PATCH is admin-only).
func subscriptionPlanEditRouter(db *gorm.DB, userID string, roles []string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	gc := controller.NewGenericController(db, nil)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if userID != "" {
			c.Set("userId", userID)
		}
		if roles != nil {
			c.Set("userRoles", roles)
		}
		c.Next()
	})
	r.PATCH("/api/v1/subscription-plans/:id", func(c *gin.Context) { gc.EditEntity(c) })
	return r
}

// patchPlanPersistence drives a real PATCH of data_persistence_gb through the
// generic controller and returns the recorder. The body is raw JSON so the whole
// NewEditDto→mapstructure→map path runs — the map shape the hook reads
// (data_persistence_gb as int vs *int) is produced by the real converter, not a
// hand-built map, so a future converter change that alters that shape is caught.
func patchPlanPersistence(t *testing.T, db *gorm.DB, planID uuid.UUID, gb string) *httptest.ResponseRecorder {
	t.Helper()
	r := subscriptionPlanEditRouter(db, "admin-1", []string{"administrator"})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/subscription-plans/"+planID.String(),
		strings.NewReader(`{"data_persistence_gb":`+gb+`}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// seedEditablePlan persists a plan at 100 GB to be updated by the PATCH tests.
func seedEditablePlan(t *testing.T, db *gorm.DB) uuid.UUID {
	t.Helper()
	id := uuid.New()
	require.NoError(t, db.Create(&models.SubscriptionPlan{
		BaseModel:         entityManagementModels.BaseModel{ID: id},
		Name:              "Editable Plan",
		PriceAmount:       1000,
		Currency:          "eur",
		BillingInterval:   "month",
		IsActive:          true,
		IsCatalog:         true,
		DataPersistenceGB: 100,
	}).Error)
	return id
}

// TestSubscriptionPlan_UpdateOver500_RejectedEndToEnd drives a real PATCH raising
// data_persistence_gb to 501 through the generic controller (the actual
// NewEditDto→mapstructure update path, with the payment hooks wired via
// InitPaymentHooks) and asserts it is rejected (400) and nothing is persisted.
// This locks the update-patch map shape the cap hook depends on.
func TestSubscriptionPlan_UpdateOver500_RejectedEndToEnd(t *testing.T) {
	db := freshTestDB(t)
	registerSubscriptionPlanForScoping(t)
	withPaymentHooksRegistered(t)

	planID := seedEditablePlan(t, db)

	w := patchPlanPersistence(t, db, planID, "501")
	require.Equal(t, http.StatusBadRequest, w.Code,
		"a PATCH raising data_persistence_gb above 500 must be rejected through the real edit path; body=%s", w.Body.String())

	var reloaded models.SubscriptionPlan
	require.NoError(t, db.First(&reloaded, "id = ?", planID).Error)
	assert.Equal(t, 100, reloaded.DataPersistenceGB,
		"a rejected over-cap update must not be persisted — the original value must survive")
}

// TestSubscriptionPlan_UpdateAt500_AcceptedEndToEnd is the GREEN-side guard that
// the cap does not over-reject the boundary: a PATCH to exactly 500 GB succeeds
// (204) and persists. Passes today (no enforcement) and must keep passing.
func TestSubscriptionPlan_UpdateAt500_AcceptedEndToEnd(t *testing.T) {
	db := freshTestDB(t)
	registerSubscriptionPlanForScoping(t)
	withPaymentHooksRegistered(t)

	planID := seedEditablePlan(t, db)

	w := patchPlanPersistence(t, db, planID, "500")
	require.Equal(t, http.StatusNoContent, w.Code,
		"a PATCH setting data_persistence_gb to exactly 500 must be accepted; body=%s", w.Body.String())

	var reloaded models.SubscriptionPlan
	require.NoError(t, db.First(&reloaded, "id = ?", planID).Error)
	assert.Equal(t, 500, reloaded.DataPersistenceGB,
		"an at-cap update must be persisted")
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
