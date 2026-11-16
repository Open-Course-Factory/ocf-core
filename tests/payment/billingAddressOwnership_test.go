// tests/payment/billingAddressOwnership_test.go
package payment_tests

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	access "soli/formations/src/auth/access"
	ems "soli/formations/src/entityManagement/entityManagementService"
	"soli/formations/src/entityManagement/hooks"
	entityManagementModels "soli/formations/src/entityManagement/models"
	entityServices "soli/formations/src/entityManagement/services"
	paymentDto "soli/formations/src/payment/dto"
	"soli/formations/src/payment/models"
)

var billingAddressOwnershipConfig = access.OwnershipConfig{
	OwnerField: "UserID", Operations: []string{"create", "update", "delete"}, AdminBypass: true,
}

// createTestBillingAddress inserts a BillingAddress owned by the given userID
func createTestBillingAddress(t *testing.T, userID string) *models.BillingAddress {
	t.Helper()
	address := &models.BillingAddress{
		BaseModel:  entityManagementModels.BaseModel{ID: uuid.New()},
		UserID:     userID,
		Line1:      "123 Main St",
		City:       "Paris",
		PostalCode: "75001",
		Country:    "FR",
	}
	err := sharedTestDB.Create(address).Error
	require.NoError(t, err)
	return address
}

// ============================================================================
// BillingAddress Ownership Hook — BeforeCreate Tests
// ============================================================================

func TestBillingAddressOwnership_BeforeCreate_SetsUserID(t *testing.T) {
	_ = freshTestDB(t)
	hook := hooks.NewOwnershipHook(sharedTestDB, "BillingAddress", billingAddressOwnershipConfig)

	userID := "user-creator-123"
	address := &models.BillingAddress{
		Line1:      "123 Main St",
		City:       "Paris",
		PostalCode: "75001",
		Country:    "FR",
	}

	ctx := &hooks.HookContext{
		EntityName: "BillingAddress",
		HookType:   hooks.BeforeCreate,
		NewEntity:  address,
		UserID:     userID,
		UserRoles:  []string{"Member"},
	}

	err := hook.Execute(ctx)
	assert.NoError(t, err)
	assert.Equal(t, userID, address.UserID, "BeforeCreate should set UserID from authenticated user")
}

func TestBillingAddressOwnership_BeforeCreate_AdminCanSetAnyUserID(t *testing.T) {
	_ = freshTestDB(t)
	hook := hooks.NewOwnershipHook(sharedTestDB, "BillingAddress", billingAddressOwnershipConfig)

	adminID := "admin-user-789"
	targetUserID := "target-user-456"
	address := &models.BillingAddress{
		UserID:     targetUserID,
		Line1:      "123 Main St",
		City:       "Paris",
		PostalCode: "75001",
		Country:    "FR",
	}

	ctx := &hooks.HookContext{
		EntityName: "BillingAddress",
		HookType:   hooks.BeforeCreate,
		NewEntity:  address,
		UserID:     adminID,
		UserRoles:  []string{"Administrator"},
	}

	err := hook.Execute(ctx)
	assert.NoError(t, err)
	assert.Equal(t, targetUserID, address.UserID, "Admin should be able to set any UserID")
}

// ============================================================================
// BillingAddress Ownership Hook — BeforeUpdate Tests
// ============================================================================

func TestBillingAddressOwnership_BeforeUpdate_OwnerCanUpdate(t *testing.T) {
	_ = freshTestDB(t)
	hook := hooks.NewOwnershipHook(sharedTestDB, "BillingAddress", billingAddressOwnershipConfig)

	ownerID := "user-owner-123"
	address := createTestBillingAddress(t, ownerID)

	ctx := &hooks.HookContext{
		EntityName: "BillingAddress",
		HookType:   hooks.BeforeUpdate,
		EntityID:   address.ID,
		OldEntity:  address,
		NewEntity:  map[string]any{"line1": "456 New St"},
		UserID:     ownerID,
		UserRoles:  []string{"Member"},
	}

	err := hook.Execute(ctx)
	assert.NoError(t, err, "Owner should be able to update their own billing address")
}

func TestBillingAddressOwnership_BeforeUpdate_NonOwnerBlocked(t *testing.T) {
	_ = freshTestDB(t)
	hook := hooks.NewOwnershipHook(sharedTestDB, "BillingAddress", billingAddressOwnershipConfig)

	ownerID := "user-owner-123"
	attackerID := "user-attacker-456"
	address := createTestBillingAddress(t, ownerID)

	ctx := &hooks.HookContext{
		EntityName: "BillingAddress",
		HookType:   hooks.BeforeUpdate,
		EntityID:   address.ID,
		OldEntity:  address,
		NewEntity:  map[string]any{"line1": "456 Hack St"},
		UserID:     attackerID,
		UserRoles:  []string{"Member"},
	}

	err := hook.Execute(ctx)
	assert.Error(t, err, "Non-owner should be blocked from updating billing address")
	assert.Contains(t, err.Error(), "permission", "Error should mention permission denial")
}

func TestBillingAddressOwnership_BeforeUpdate_AdminCanUpdate(t *testing.T) {
	_ = freshTestDB(t)
	hook := hooks.NewOwnershipHook(sharedTestDB, "BillingAddress", billingAddressOwnershipConfig)

	ownerID := "user-owner-123"
	adminID := "admin-user-789"
	address := createTestBillingAddress(t, ownerID)

	ctx := &hooks.HookContext{
		EntityName: "BillingAddress",
		HookType:   hooks.BeforeUpdate,
		EntityID:   address.ID,
		OldEntity:  address,
		NewEntity:  map[string]any{"line1": "456 Admin St"},
		UserID:     adminID,
		UserRoles:  []string{"Administrator"},
	}

	err := hook.Execute(ctx)
	assert.NoError(t, err, "Admin should be able to update any billing address")
}

// ============================================================================
// BillingAddress Ownership Hook — BeforeDelete Tests
// ============================================================================

func TestBillingAddressOwnership_BeforeDelete_OwnerCanDelete(t *testing.T) {
	_ = freshTestDB(t)
	hook := hooks.NewOwnershipHook(sharedTestDB, "BillingAddress", billingAddressOwnershipConfig)

	ownerID := "user-owner-123"
	address := createTestBillingAddress(t, ownerID)

	ctx := &hooks.HookContext{
		EntityName: "BillingAddress",
		HookType:   hooks.BeforeDelete,
		EntityID:   address.ID,
		NewEntity:  address,
		UserID:     ownerID,
		UserRoles:  []string{"Member"},
	}

	err := hook.Execute(ctx)
	assert.NoError(t, err, "Owner should be able to delete their own billing address")
}

func TestBillingAddressOwnership_BeforeDelete_NonOwnerBlocked(t *testing.T) {
	_ = freshTestDB(t)
	hook := hooks.NewOwnershipHook(sharedTestDB, "BillingAddress", billingAddressOwnershipConfig)

	ownerID := "user-owner-123"
	attackerID := "user-attacker-456"
	address := createTestBillingAddress(t, ownerID)

	ctx := &hooks.HookContext{
		EntityName: "BillingAddress",
		HookType:   hooks.BeforeDelete,
		EntityID:   address.ID,
		NewEntity:  address,
		UserID:     attackerID,
		UserRoles:  []string{"Member"},
	}

	err := hook.Execute(ctx)
	assert.Error(t, err, "Non-owner should be blocked from deleting billing address")
	assert.Contains(t, err.Error(), "permission", "Error should mention permission denial")
}

func TestBillingAddressOwnership_BeforeDelete_AdminCanDelete(t *testing.T) {
	_ = freshTestDB(t)
	hook := hooks.NewOwnershipHook(sharedTestDB, "BillingAddress", billingAddressOwnershipConfig)

	ownerID := "user-owner-123"
	adminID := "admin-user-789"
	address := createTestBillingAddress(t, ownerID)

	ctx := &hooks.HookContext{
		EntityName: "BillingAddress",
		HookType:   hooks.BeforeDelete,
		EntityID:   address.ID,
		NewEntity:  address,
		UserID:     adminID,
		UserRoles:  []string{"Administrator"},
	}

	err := hook.Execute(ctx)
	assert.NoError(t, err, "Admin should be able to delete any billing address")
}

// TestBillingAddressOwnership_AdminGenericCreate_OwnedByAdmin drives the real
// BillingAddress registration and generic create path as an administrator. The
// create DTO has no user_id, so the hook is the only thing that can set the owner.
func TestBillingAddressOwnership_AdminGenericCreate_OwnedByAdmin(t *testing.T) {
	_ = freshTestDB(t)
	setupBillingAddressEntity(t)
	withPaymentHooksRegistered(t)
	ems.RegisterOwnershipHooks(sharedTestDB)

	input := paymentDto.CreateBillingAddressInput{Line1: "1 rue de Rivoli", City: "Paris", PostalCode: "75001", Country: "FR"}
	svc := entityServices.NewGenericService(sharedTestDB, nil)
	created, err := svc.CreateEntityWithUser(input, "BillingAddress", "admin-user-789", "administrator")
	require.NoError(t, err)

	ops, ok := ems.GlobalEntityRegistrationService.GetEntityOps("BillingAddress")
	require.True(t, ok)
	id, err := ops.ExtractID(created)
	require.NoError(t, err)

	var row models.BillingAddress
	require.NoError(t, sharedTestDB.First(&row, "id = ?", id).Error)
	assert.Equal(t, "admin-user-789", row.UserID, "an administrator's billing address must be owned by the administrator")
}
