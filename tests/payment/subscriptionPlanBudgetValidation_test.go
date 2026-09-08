// tests/payment/subscriptionPlanBudgetValidation_test.go
//
// A plan must declare a positive CPU and memory budget.
//
// Until now `0` on either axis meant "unlimited", so a plan created with the
// fields left alone silently granted everything — the shape of #481, where a
// soft-deleted plan resolved to a zero-value struct and handed out XL machines.
// With the unlimited state removed, `0` means "no capacity", which is the right
// reading for a value nobody set — but it makes an unset budget a plan nobody
// can launch anything on, administrators included (they do not bypass the
// budget gate).
//
// So the zero has to be refused at the door rather than discovered at 9am with
// a class waiting. binding tags are INERT on the generic entity routes (#390),
// so enforcement lives in the same BeforeCreate/BeforeUpdate hook that already
// caps data_persistence_gb.
//
// Create and update are deliberately asymmetric:
//   - create: the budget must be stated and positive; an absent field is a zero
//     and is rejected.
//   - update: an absent field is a partial patch and is not validated; a stated
//     non-positive one is rejected.
package payment_tests

import (
	"testing"

	entityErrors "soli/formations/src/entityManagement/errors"
	"soli/formations/src/entityManagement/hooks"
	entityServices "soli/formations/src/entityManagement/services"
	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubscriptionPlanValidation_CreateRequiresPositiveBudget(t *testing.T) {
	cases := []struct {
		name        string
		cpu, memory int
		rejected    bool
	}{
		{"both stated", 6000, 6144, false},
		{"smallest real plan", 500, 256, false},
		{"cpu unset", 0, 6144, true},
		{"memory unset", 6000, 0, true},
		{"both unset — the accidental plan", 0, 0, true},
		{"negative cpu", -1, 6144, true},
		{"negative memory", 6000, -1, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := execPlanValidation(hooks.BeforeCreate, &models.SubscriptionPlan{
				Name:        "Test Plan",
				MaxCPU:      tc.cpu,
				MaxMemoryMB: tc.memory,
			})

			if tc.rejected {
				require.Error(t, err, "a non-positive budget must be refused at create")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// The error must name the offending axis: an operator who mistyped one field
// should not have to guess which. EntityError carries the field in Details
// rather than in the message, which is what the controllers serialise.
func rejectedField(t *testing.T, err error) string {
	t.Helper()
	var entityErr *entityErrors.EntityError
	require.ErrorAs(t, err, &entityErr)
	require.NotNil(t, entityErr.Details)
	field, _ := entityErr.Details["field"].(string)
	return field
}

func TestSubscriptionPlanValidation_ErrorNamesTheAxis(t *testing.T) {
	err := execPlanValidation(hooks.BeforeCreate, &models.SubscriptionPlan{
		Name: "No CPU", MaxCPU: 0, MaxMemoryMB: 6144,
	})
	require.Error(t, err)
	assert.Equal(t, "max_cpu", rejectedField(t, err))

	err = execPlanValidation(hooks.BeforeCreate, &models.SubscriptionPlan{
		Name: "No RAM", MaxCPU: 6000, MaxMemoryMB: 0,
	})
	require.Error(t, err)
	assert.Equal(t, "max_memory_mb", rejectedField(t, err))
}

func TestSubscriptionPlanValidation_UpdateRejectsStatedZero(t *testing.T) {
	cases := []struct {
		name     string
		patch    map[string]any
		rejected bool
	}{
		{"raises cpu", map[string]any{"max_cpu": 24000}, false},
		{"zeroes cpu", map[string]any{"max_cpu": 0}, true},
		{"zeroes memory", map[string]any{"max_memory_mb": 0}, true},
		{"negative cpu", map[string]any{"max_cpu": -1}, true},
		{"unrelated field only", map[string]any{"name": "Renamed"}, false},
		{"empty patch", map[string]any{}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := execPlanValidation(hooks.BeforeUpdate, tc.patch)
			if tc.rejected {
				require.Error(t, err)
			} else {
				assert.NoError(t, err, "a patch that does not state a budget is a partial update")
			}
		})
	}
}

// The generic PATCH path decodes the pointer-field Edit DTO via mapstructure,
// so a stated budget arrives as *int. A nil pointer is "not stated".
func TestSubscriptionPlanValidation_UpdateHandlesPointerPatch(t *testing.T) {
	zero, positive := 0, 12000

	require.Error(t, execPlanValidation(hooks.BeforeUpdate,
		map[string]any{"max_cpu": &zero}), "a stated zero must be refused through the pointer shape too")

	assert.NoError(t, execPlanValidation(hooks.BeforeUpdate,
		map[string]any{"max_cpu": &positive}))

	var absent *int
	assert.NoError(t, execPlanValidation(hooks.BeforeUpdate,
		map[string]any{"max_cpu": absent}), "a nil pointer is an omitted field")
}

// End-to-end through the REAL generic service with the payment hooks wired:
// a plan created without budgets is refused and nothing is persisted. This is
// the path an admin actually uses, and the only one that can produce the
// zero-budget row the quota engine now reads as "no capacity".
func TestSubscriptionPlan_CreateWithoutBudget_RejectedEndToEnd(t *testing.T) {
	_ = freshTestDB(t)
	registerSubscriptionPlanForScoping(t)
	withPaymentHooksRegistered(t)

	svc := entityServices.NewGenericService(sharedTestDB, nil)
	_, err := svc.CreateEntityWithUser(dto.CreateSubscriptionPlanInput{
		Name:            "No Budget Plan",
		PriceAmount:     1000,
		Currency:        "eur",
		BillingInterval: "month",
		// MaxCPU and MaxMemoryMB deliberately omitted — the accidental plan.
	}, "SubscriptionPlan", "admin-1")

	require.Error(t, err, "a plan with no budget must be refused")

	var count int64
	require.NoError(t, sharedTestDB.Model(&models.SubscriptionPlan{}).
		Where("name = ?", "No Budget Plan").Count(&count).Error)
	assert.Equal(t, int64(0), count, "the refused plan must not be persisted")
}
