// tests/payment/bulkLicenseAuthzTightening_test.go
//
// Failing tests (TDD red phase) for GitLab issue #257 — bulk-license
// authorization tightening.
//
// Two layered bugs are exercised here:
//
//  1. Service-level canUserAccessBatch is too permissive on destructive
//     operations: any regular "member" of a shared team organization can
//     permanently delete a batch, mutate its quantity, assign, or revoke
//     licenses. Access to these destructive operations must be restricted
//     to purchaser OR org manager+/owner.
//
//  2. Layer 2 RouteRegistry entries for SubscriptionBatch declare
//     Field: "PurchaserID" but the model field is "PurchaserUserID".
//     The declarative Layer-2 check therefore cannot resolve ownership.
//
// Tests are written at the public-API level (service methods + registry
// inspection) because the planned canUserManageBatch / canUserReadBatch
// helpers are private to the services package.
package payment_tests

import (
	"testing"
	"time"

	access "soli/formations/src/auth/access"
	"soli/formations/src/auth/mocks"
	orgModels "soli/formations/src/organizations/models"
	"soli/formations/src/payment/models"
	paymentController "soli/formations/src/payment/routes"
	"soli/formations/src/payment/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// seedSharedTeamOrg creates a "team" organization with the given purchaser
// (as owner) and an additional member at the specified role.
func seedSharedTeamOrg(t *testing.T, db *gorm.DB, purchaserID, otherUserID string, otherRole orgModels.OrganizationMemberRole) *orgModels.Organization {
	t.Helper()
	org := orgModels.Organization{
		Name:             "team-" + otherUserID,
		DisplayName:      "Team " + otherUserID,
		OwnerUserID:      purchaserID,
		OrganizationType: "team",
		IsActive:         true,
	}
	require.NoError(t, db.Omit("Metadata").Create(&org).Error)

	// Purchaser is the owner of the team org
	require.NoError(t, db.Omit("Metadata").Create(&orgModels.OrganizationMember{
		OrganizationID: org.ID,
		UserID:         purchaserID,
		Role:           orgModels.OrgRoleOwner,
		JoinedAt:       time.Now(),
		IsActive:       true,
	}).Error)

	// Other user joins at the requested role
	require.NoError(t, db.Omit("Metadata").Create(&orgModels.OrganizationMember{
		OrganizationID: org.ID,
		UserID:         otherUserID,
		Role:           otherRole,
		JoinedAt:       time.Now(),
		IsActive:       true,
	}).Error)

	return &org
}

// -----------------------------------------------------------------------------
// Read-lenient baseline (confirm we keep reads open to co-org members)
// -----------------------------------------------------------------------------

// TestCanUserReadBatch_TeamOrgMember_Allowed — a regular member of a shared
// team org can still read the batch (via GetAccessibleBatchByID).
// This documents the intended "lenient read" behavior after the fix.
func TestCanUserReadBatch_TeamOrgMember_Allowed(t *testing.T) {
	db := setupBulkLicenseTestDB(t)
	svc := services.NewBulkLicenseService(db)

	purchaserID := "read-purchaser-01"
	memberID := "read-member-01"

	_, batch, _ := seedBulkLicenseTestData(t, db, purchaserID, 3, 0)
	seedSharedTeamOrg(t, db, purchaserID, memberID, orgModels.OrgRoleMember)

	result, err := svc.GetAccessibleBatchByID(batch.ID, memberID)
	require.NoError(t, err)
	assert.Equal(t, batch.ID, result.ID)
}

// TestCanUserReadBatch_UnrelatedUser_Denied — a user with no link to the
// purchaser cannot read the batch.
func TestCanUserReadBatch_UnrelatedUser_Denied(t *testing.T) {
	db := setupBulkLicenseTestDB(t)
	svc := services.NewBulkLicenseService(db)

	purchaserID := "read-purchaser-02"
	_, batch, _ := seedBulkLicenseTestData(t, db, purchaserID, 3, 0)

	_, err := svc.GetAccessibleBatchByID(batch.ID, "stranger-user")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "access denied")
}

// -----------------------------------------------------------------------------
// Manage-strict tests (these are the real bug fix)
// -----------------------------------------------------------------------------

// TestCanUserManageBatch_Purchaser_Allowed — the purchaser can manage
// (destructive ops) their own batch. Exercised through PermanentlyDeleteBatch.
func TestCanUserManageBatch_Purchaser_Allowed(t *testing.T) {
	db := setupBulkLicenseTestDB(t)
	svc := services.NewBulkLicenseService(db)

	purchaserID := "manage-purchaser-01"
	_, batch, _ := seedBulkLicenseTestData(t, db, purchaserID, 3, 0)

	// UpdateBatchQuantity calls Stripe; reducing below assigned hits the
	// pre-Stripe guard and proves we got past the authorization check.
	// With 0 assigned, newQuantity < assignedQuantity is impossible, so we
	// instead use PermanentlyDeleteBatch with StripeSubscriptionID left as
	// the fake value — the cancel call logs a warning but is non-blocking.
	// To avoid touching Stripe, blank the Stripe ID on the seeded batch.
	require.NoError(t, db.Model(&models.SubscriptionBatch{}).
		Where("id = ?", batch.ID).
		Update("stripe_subscription_id", "").Error)

	err := svc.PermanentlyDeleteBatch(batch.ID, purchaserID)
	require.NoError(t, err)

	// Batch must be fully deleted (Unscoped delete)
	var count int64
	db.Unscoped().Model(&models.SubscriptionBatch{}).
		Where("id = ?", batch.ID).
		Count(&count)
	assert.Equal(t, int64(0), count, "batch should be hard-deleted by purchaser")
}

// TestCanUserManageBatch_TeamOrgOwner_Allowed — org owner (non-purchaser)
// can manage the batch.
func TestCanUserManageBatch_TeamOrgOwner_Allowed(t *testing.T) {
	db := setupBulkLicenseTestDB(t)
	svc := services.NewBulkLicenseService(db)

	purchaserID := "manage-purchaser-owner-01"
	otherOwnerID := "manage-other-owner-01"

	_, batch, _ := seedBulkLicenseTestData(t, db, purchaserID, 3, 0)

	// Shared team org where purchaser joins as manager and "other" is owner.
	org := orgModels.Organization{
		Name:             "team-two-owners",
		DisplayName:      "Team Two Owners",
		OwnerUserID:      otherOwnerID,
		OrganizationType: "team",
		IsActive:         true,
	}
	require.NoError(t, db.Omit("Metadata").Create(&org).Error)
	require.NoError(t, db.Omit("Metadata").Create(&orgModels.OrganizationMember{
		OrganizationID: org.ID,
		UserID:         purchaserID,
		Role:           orgModels.OrgRoleManager,
		JoinedAt:       time.Now(),
		IsActive:       true,
	}).Error)
	require.NoError(t, db.Omit("Metadata").Create(&orgModels.OrganizationMember{
		OrganizationID: org.ID,
		UserID:         otherOwnerID,
		Role:           orgModels.OrgRoleOwner,
		JoinedAt:       time.Now(),
		IsActive:       true,
	}).Error)

	// UpdateBatchQuantity to a value below assignedQuantity triggers a
	// pre-Stripe guard and proves the authorization check passed.
	// With 0 assigned, we test "reduce to current" is a no-op success.
	err := svc.UpdateBatchQuantity(batch.ID, otherOwnerID, 3)
	require.NoError(t, err, "org owner should be allowed to manage a shared-org batch")
}

// TestCanUserManageBatch_TeamOrgManager_Allowed — org manager (non-purchaser)
// can manage the batch.
func TestCanUserManageBatch_TeamOrgManager_Allowed(t *testing.T) {
	db := setupBulkLicenseTestDB(t)
	svc := services.NewBulkLicenseService(db)

	purchaserID := "manage-purchaser-mgr-01"
	managerID := "manage-other-manager-01"

	_, batch, _ := seedBulkLicenseTestData(t, db, purchaserID, 3, 0)
	seedSharedTeamOrg(t, db, purchaserID, managerID, orgModels.OrgRoleManager)

	// Same no-op quantity update strategy: if auth passes, the call hits
	// difference == 0 and returns nil without calling Stripe (see
	// bulkLicenseService.go, UpdateBatchQuantity difference==0 early return).
	err := svc.UpdateBatchQuantity(batch.ID, managerID, 3)
	require.NoError(t, err, "org manager should be allowed to manage a shared-org batch")
}

// TestCanUserManageBatch_TeamOrgMember_Denied — THE BUG: a regular member
// of a shared team org is granted destructive access today. After the fix,
// this must return an authorization error.
func TestCanUserManageBatch_TeamOrgMember_Denied(t *testing.T) {
	db := setupBulkLicenseTestDB(t)
	svc := services.NewBulkLicenseService(db)

	purchaserID := "manage-purchaser-member-01"
	memberID := "manage-other-member-01"

	_, batch, _ := seedBulkLicenseTestData(t, db, purchaserID, 3, 0)
	seedSharedTeamOrg(t, db, purchaserID, memberID, orgModels.OrgRoleMember)

	// No-op quantity update (same qty). Currently: auth passes (bug),
	// difference==0 early return, no error. After fix: auth fails here.
	err := svc.UpdateBatchQuantity(batch.ID, memberID, 3)
	require.Error(t, err, "regular org member must NOT be allowed to manage a batch (bug)")
	assert.Contains(t, err.Error(), "access denied")
}

// TestCanUserManageBatch_UnrelatedUser_Denied — an unrelated user gets a
// plain access denied.
func TestCanUserManageBatch_UnrelatedUser_Denied(t *testing.T) {
	db := setupBulkLicenseTestDB(t)
	svc := services.NewBulkLicenseService(db)

	purchaserID := "manage-purchaser-unrelated-01"
	_, batch, _ := seedBulkLicenseTestData(t, db, purchaserID, 3, 0)

	err := svc.UpdateBatchQuantity(batch.ID, "unrelated-user", 3)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "access denied")
}

// -----------------------------------------------------------------------------
// End-to-end attack demonstrations — full service calls with a "member" user
// -----------------------------------------------------------------------------

// TestBulkLicenseService_MemberCannot_PermanentlyDeleteBatch — a team-org
// "member" (not purchaser, not manager+) must NOT be able to permanently
// delete a batch. Today they can — this is the real-world attack this fix
// closes.
func TestBulkLicenseService_MemberCannot_PermanentlyDeleteBatch(t *testing.T) {
	db := setupBulkLicenseTestDB(t)
	svc := services.NewBulkLicenseService(db)

	purchaserID := "attacked-purchaser-del-01"
	attackerID := "attacker-member-del-01"

	_, batch, _ := seedBulkLicenseTestData(t, db, purchaserID, 5, 0)
	seedSharedTeamOrg(t, db, purchaserID, attackerID, orgModels.OrgRoleMember)

	// Blank the Stripe subscription ID so the (currently-reachable) Stripe
	// cancel call is a no-op. After the fix, the auth check fails BEFORE
	// the Stripe step anyway, so this is harmless either way.
	require.NoError(t, db.Model(&models.SubscriptionBatch{}).
		Where("id = ?", batch.ID).
		Update("stripe_subscription_id", "").Error)

	err := svc.PermanentlyDeleteBatch(batch.ID, attackerID)
	require.Error(t, err, "regular org member must not be allowed to delete")
	assert.Contains(t, err.Error(), "access denied")

	// Verify the batch still exists in the DB (not hard-deleted).
	var count int64
	db.Unscoped().Model(&models.SubscriptionBatch{}).
		Where("id = ?", batch.ID).
		Count(&count)
	assert.Equal(t, int64(1), count, "batch must NOT be deleted when auth fails")
}

// TestBulkLicenseService_MemberCannot_UpdateBatchQuantity — a team-org
// "member" must NOT be able to scale the batch quantity (which would
// inflate the purchaser's Stripe bill).
func TestBulkLicenseService_MemberCannot_UpdateBatchQuantity(t *testing.T) {
	db := setupBulkLicenseTestDB(t)
	svc := services.NewBulkLicenseService(db)

	purchaserID := "attacked-purchaser-qty-01"
	attackerID := "attacker-member-qty-01"

	_, batch, _ := seedBulkLicenseTestData(t, db, purchaserID, 5, 0)
	seedSharedTeamOrg(t, db, purchaserID, attackerID, orgModels.OrgRoleMember)

	originalQty := batch.TotalQuantity

	err := svc.UpdateBatchQuantity(batch.ID, attackerID, 500)
	require.Error(t, err, "regular org member must not be allowed to scale the batch")
	assert.Contains(t, err.Error(), "access denied")

	// Confirm quantity unchanged.
	var fresh models.SubscriptionBatch
	require.NoError(t, db.First(&fresh, batch.ID).Error)
	assert.Equal(t, originalQty, fresh.TotalQuantity, "total quantity must be unchanged")
}

// TestBulkLicenseService_MemberCannot_AssignLicense — a team-org "member"
// must NOT be able to assign licenses from someone else's batch.
func TestBulkLicenseService_MemberCannot_AssignLicense(t *testing.T) {
	db := setupBulkLicenseTestDB(t)
	svc := services.NewBulkLicenseService(db)

	purchaserID := "attacked-purchaser-assign-01"
	attackerID := "attacker-member-assign-01"

	_, batch, _ := seedBulkLicenseTestData(t, db, purchaserID, 5, 0)
	seedSharedTeamOrg(t, db, purchaserID, attackerID, orgModels.OrgRoleMember)

	_, err := svc.AssignLicense(batch.ID, attackerID, "some-random-target")
	require.Error(t, err, "regular org member must not be allowed to assign licenses")
	assert.Contains(t, err.Error(), "access denied")

	// Batch must still show 0 assigned.
	var fresh models.SubscriptionBatch
	require.NoError(t, db.First(&fresh, batch.ID).Error)
	assert.Equal(t, 0, fresh.AssignedQuantity)
}

// TestBulkLicenseService_MemberCannot_RevokeLicense — a team-org "member"
// must NOT be able to revoke an assigned license.
func TestBulkLicenseService_MemberCannot_RevokeLicense(t *testing.T) {
	db := setupBulkLicenseTestDB(t)
	svc := services.NewBulkLicenseService(db)

	purchaserID := "attacked-purchaser-revoke-01"
	attackerID := "attacker-member-revoke-01"

	// 5 licenses, 2 of which are assigned.
	_, _, licenses := seedBulkLicenseTestData(t, db, purchaserID, 5, 2)
	seedSharedTeamOrg(t, db, purchaserID, attackerID, orgModels.OrgRoleMember)

	assignedLicense := licenses[0]
	originalUserID := assignedLicense.UserID
	require.NotEmpty(t, originalUserID, "test precondition: license must be assigned")

	err := svc.RevokeLicense(assignedLicense.ID, attackerID)
	require.Error(t, err, "regular org member must not be allowed to revoke licenses")
	assert.Contains(t, err.Error(), "access denied")

	// Confirm the assignment is still there.
	var fresh models.UserSubscription
	require.NoError(t, db.First(&fresh, assignedLicense.ID).Error)
	assert.Equal(t, originalUserID, fresh.UserID, "license assignment must be unchanged")
	assert.Equal(t, "active", fresh.Status)
}

// TestBulkLicenseService_ManagerCan_UpdateBatchQuantity — a manager of the
// shared team org IS allowed to manage. Demonstrates the fix doesn't
// over-restrict.
func TestBulkLicenseService_ManagerCan_UpdateBatchQuantity(t *testing.T) {
	db := setupBulkLicenseTestDB(t)
	svc := services.NewBulkLicenseService(db)

	purchaserID := "allowed-purchaser-mgr-02"
	managerID := "allowed-manager-mgr-02"

	_, batch, _ := seedBulkLicenseTestData(t, db, purchaserID, 3, 0)
	seedSharedTeamOrg(t, db, purchaserID, managerID, orgModels.OrgRoleManager)

	// No-op quantity (same value) — passes auth, hits difference==0 early
	// return, no Stripe call. If the fix over-restricts and denies the
	// manager, we'll see "access denied" here instead of nil.
	err := svc.UpdateBatchQuantity(batch.ID, managerID, 3)
	require.NoError(t, err, "org manager must retain management rights")
}

// -----------------------------------------------------------------------------
// Layer-2 permissions registry test — field name mismatch
// -----------------------------------------------------------------------------

// TestPermissions_SubscriptionBatch_UsesCorrectFieldName verifies that all
// RouteRegistry entries targeting the SubscriptionBatch entity reference
// "PurchaserUserID" — the actual model field — and never "PurchaserID",
// which does not exist on the model.
func TestPermissions_SubscriptionBatch_UsesCorrectFieldName(t *testing.T) {
	// Reset the shared registry so this test is deterministic regardless
	// of other tests that may have mutated it.
	access.RouteRegistry.Reset()
	t.Cleanup(func() { access.RouteRegistry.Reset() })

	// Register only payment permissions.
	mockEnforcer := mocks.NewMockEnforcer()
	paymentController.RegisterPaymentPermissions(mockEnforcer)

	ref := access.RouteRegistry.GetReference()

	var subscriptionBatchRoutes []access.RoutePermission
	for _, category := range ref.Categories {
		for _, route := range category.Routes {
			if route.Access.Type == access.EntityOwner && route.Access.Entity == "SubscriptionBatch" {
				subscriptionBatchRoutes = append(subscriptionBatchRoutes, route)
			}
		}
	}

	require.NotEmpty(t, subscriptionBatchRoutes,
		"expected at least one RouteRegistry entry for SubscriptionBatch EntityOwner")

	for _, route := range subscriptionBatchRoutes {
		// The model field is PurchaserUserID. Using PurchaserID silently
		// breaks the declarative Layer-2 ownership check.
		assert.NotEqual(t, "PurchaserID", route.Access.Field,
			"route %s %s: Access.Field must not be 'PurchaserID' — use 'PurchaserUserID' (matches model)",
			route.Method, route.Path)
		assert.Equal(t, "PurchaserUserID", route.Access.Field,
			"route %s %s: Access.Field must be 'PurchaserUserID' (matches SubscriptionBatch model)",
			route.Method, route.Path)
	}
}
