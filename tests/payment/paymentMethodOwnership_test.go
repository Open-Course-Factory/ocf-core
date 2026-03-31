// tests/payment/paymentMethodOwnership_test.go
package payment_tests

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	access "soli/formations/src/auth/access"
	"soli/formations/src/entityManagement/hooks"
	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/payment/models"
)

var paymentMethodOwnershipConfig = access.OwnershipConfig{
	OwnerField: "UserID", Operations: []string{"create", "update", "delete"}, AdminBypass: true,
}

// createTestPaymentMethod inserts a PaymentMethod owned by the given userID
func createTestPaymentMethod(t *testing.T, userID string) *models.PaymentMethod {
	t.Helper()
	pm := &models.PaymentMethod{
		BaseModel:             entityManagementModels.BaseModel{ID: uuid.New()},
		UserID:                userID,
		StripePaymentMethodID: "pm_test_" + uuid.New().String()[:8],
		Type:                  "card",
		CardBrand:             "visa",
		CardLast4:             "4242",
		IsDefault:             false,
		IsActive:              true,
	}
	err := sharedTestDB.Create(pm).Error
	require.NoError(t, err)
	return pm
}

// ============================================================================
// PaymentMethod Ownership Hook — BeforeCreate Tests
// ============================================================================

func TestPaymentMethodOwnership_BeforeCreate_SetsUserID(t *testing.T) {
	_ = freshTestDB(t)
	hook := hooks.NewOwnershipHook(sharedTestDB, "PaymentMethod", paymentMethodOwnershipConfig)

	userID := "user-creator-123"
	pm := &models.PaymentMethod{
		StripePaymentMethodID: "pm_test_abc",
		Type:                  "card",
	}

	ctx := &hooks.HookContext{
		EntityName: "PaymentMethod",
		HookType:   hooks.BeforeCreate,
		NewEntity:  pm,
		UserID:     userID,
		UserRoles:  []string{"Member"},
	}

	err := hook.Execute(ctx)
	assert.NoError(t, err)
	assert.Equal(t, userID, pm.UserID, "BeforeCreate should set UserID from authenticated user")
}

func TestPaymentMethodOwnership_BeforeCreate_AdminCanSetAnyUserID(t *testing.T) {
	_ = freshTestDB(t)
	hook := hooks.NewOwnershipHook(sharedTestDB, "PaymentMethod", paymentMethodOwnershipConfig)

	adminID := "admin-user-789"
	targetUserID := "target-user-456"
	pm := &models.PaymentMethod{
		UserID:                targetUserID,
		StripePaymentMethodID: "pm_test_abc",
		Type:                  "card",
	}

	ctx := &hooks.HookContext{
		EntityName: "PaymentMethod",
		HookType:   hooks.BeforeCreate,
		NewEntity:  pm,
		UserID:     adminID,
		UserRoles:  []string{"Administrator"},
	}

	err := hook.Execute(ctx)
	assert.NoError(t, err)
	assert.Equal(t, targetUserID, pm.UserID, "Admin should be able to set any UserID")
}

// ============================================================================
// PaymentMethod Ownership Hook — BeforeUpdate Tests
// ============================================================================

func TestPaymentMethodOwnership_BeforeUpdate_OwnerCanUpdate(t *testing.T) {
	_ = freshTestDB(t)
	hook := hooks.NewOwnershipHook(sharedTestDB, "PaymentMethod", paymentMethodOwnershipConfig)

	ownerID := "user-owner-123"
	pm := createTestPaymentMethod(t, ownerID)

	ctx := &hooks.HookContext{
		EntityName: "PaymentMethod",
		HookType:   hooks.BeforeUpdate,
		EntityID:   pm.ID,
		OldEntity:  pm,
		NewEntity:  map[string]any{"is_default": true},
		UserID:     ownerID,
		UserRoles:  []string{"Member"},
	}

	err := hook.Execute(ctx)
	assert.NoError(t, err, "Owner should be able to update their own payment method")
}

func TestPaymentMethodOwnership_BeforeUpdate_NonOwnerBlocked(t *testing.T) {
	_ = freshTestDB(t)
	hook := hooks.NewOwnershipHook(sharedTestDB, "PaymentMethod", paymentMethodOwnershipConfig)

	ownerID := "user-owner-123"
	attackerID := "user-attacker-456"
	pm := createTestPaymentMethod(t, ownerID)

	ctx := &hooks.HookContext{
		EntityName: "PaymentMethod",
		HookType:   hooks.BeforeUpdate,
		EntityID:   pm.ID,
		OldEntity:  pm,
		NewEntity:  map[string]any{"is_default": true},
		UserID:     attackerID,
		UserRoles:  []string{"Member"},
	}

	err := hook.Execute(ctx)
	assert.Error(t, err, "Non-owner should be blocked from updating payment method")
	assert.Contains(t, err.Error(), "permission", "Error should mention permission denial")
}

func TestPaymentMethodOwnership_BeforeUpdate_AdminCanUpdate(t *testing.T) {
	_ = freshTestDB(t)
	hook := hooks.NewOwnershipHook(sharedTestDB, "PaymentMethod", paymentMethodOwnershipConfig)

	ownerID := "user-owner-123"
	adminID := "admin-user-789"
	pm := createTestPaymentMethod(t, ownerID)

	ctx := &hooks.HookContext{
		EntityName: "PaymentMethod",
		HookType:   hooks.BeforeUpdate,
		EntityID:   pm.ID,
		OldEntity:  pm,
		NewEntity:  map[string]any{"is_default": true},
		UserID:     adminID,
		UserRoles:  []string{"Administrator"},
	}

	err := hook.Execute(ctx)
	assert.NoError(t, err, "Admin should be able to update any payment method")
}

// ============================================================================
// PaymentMethod Ownership Hook — BeforeDelete Tests
// ============================================================================

func TestPaymentMethodOwnership_BeforeDelete_OwnerCanDelete(t *testing.T) {
	_ = freshTestDB(t)
	hook := hooks.NewOwnershipHook(sharedTestDB, "PaymentMethod", paymentMethodOwnershipConfig)

	ownerID := "user-owner-123"
	pm := createTestPaymentMethod(t, ownerID)

	ctx := &hooks.HookContext{
		EntityName: "PaymentMethod",
		HookType:   hooks.BeforeDelete,
		EntityID:   pm.ID,
		NewEntity:  pm,
		UserID:     ownerID,
		UserRoles:  []string{"Member"},
	}

	err := hook.Execute(ctx)
	assert.NoError(t, err, "Owner should be able to delete their own payment method")
}

func TestPaymentMethodOwnership_BeforeDelete_NonOwnerBlocked(t *testing.T) {
	_ = freshTestDB(t)
	hook := hooks.NewOwnershipHook(sharedTestDB, "PaymentMethod", paymentMethodOwnershipConfig)

	ownerID := "user-owner-123"
	attackerID := "user-attacker-456"
	pm := createTestPaymentMethod(t, ownerID)

	ctx := &hooks.HookContext{
		EntityName: "PaymentMethod",
		HookType:   hooks.BeforeDelete,
		EntityID:   pm.ID,
		NewEntity:  pm,
		UserID:     attackerID,
		UserRoles:  []string{"Member"},
	}

	err := hook.Execute(ctx)
	assert.Error(t, err, "Non-owner should be blocked from deleting payment method")
	assert.Contains(t, err.Error(), "permission", "Error should mention permission denial")
}

func TestPaymentMethodOwnership_BeforeDelete_AdminCanDelete(t *testing.T) {
	_ = freshTestDB(t)
	hook := hooks.NewOwnershipHook(sharedTestDB, "PaymentMethod", paymentMethodOwnershipConfig)

	ownerID := "user-owner-123"
	adminID := "admin-user-789"
	pm := createTestPaymentMethod(t, ownerID)

	ctx := &hooks.HookContext{
		EntityName: "PaymentMethod",
		HookType:   hooks.BeforeDelete,
		EntityID:   pm.ID,
		NewEntity:  pm,
		UserID:     adminID,
		UserRoles:  []string{"Administrator"},
	}

	err := hook.Execute(ctx)
	assert.NoError(t, err, "Admin should be able to delete any payment method")
}
