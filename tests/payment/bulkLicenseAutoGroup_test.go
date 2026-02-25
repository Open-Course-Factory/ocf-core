package payment_tests

import (
	"testing"
	"time"

	groupModels "soli/formations/src/groups/models"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAssignLicense_WithGroupID_AddsUserToGroup(t *testing.T) {
	db := freshTestDB(t)

	purchaserID := "purchaser-1"
	targetUserID := "target-user-1"
	groupID := uuid.New()

	// Create plan
	plan := &models.SubscriptionPlan{
		Name:     "Test Plan",
		IsActive: true,
	}
	require.NoError(t, db.Create(plan).Error)

	// Create group owned by the purchaser
	group := &groupModels.ClassGroup{
		Name:        "test-group",
		DisplayName: "Test Group",
		OwnerUserID: purchaserID,
		IsActive:    true,
		MaxMembers:  50,
	}
	group.ID = groupID
	require.NoError(t, db.Omit("Metadata").Create(group).Error)

	// Add purchaser as owner member (needed for AddMembersToGroup permission check)
	ownerMember := &groupModels.GroupMember{
		GroupID:  groupID,
		UserID:   purchaserID,
		Role:     groupModels.GroupMemberRoleOwner,
		JoinedAt: time.Now(),
		IsActive: true,
	}
	require.NoError(t, db.Omit("Metadata").Create(ownerMember).Error)

	// Create batch linked to group
	batch := &models.SubscriptionBatch{
		PurchaserUserID:    purchaserID,
		SubscriptionPlanID: plan.ID,
		GroupID:            &groupID,
		TotalQuantity:      5,
		AssignedQuantity:   0,
		Status:             "active",
		CurrentPeriodStart: time.Now(),
		CurrentPeriodEnd:   time.Now().Add(30 * 24 * time.Hour),
	}
	require.NoError(t, db.Create(batch).Error)

	// Create unassigned license
	license := &models.UserSubscription{
		UserID:              "",
		PurchaserUserID:     &purchaserID,
		SubscriptionBatchID: &batch.ID,
		SubscriptionPlanID:  plan.ID,
		Status:              "unassigned",
		CurrentPeriodStart:  time.Now(),
		CurrentPeriodEnd:    time.Now().Add(30 * 24 * time.Hour),
	}
	require.NoError(t, db.Create(license).Error)

	// Assign the license
	svc := services.NewBulkLicenseService(db)
	assigned, err := svc.AssignLicense(batch.ID, purchaserID, targetUserID)
	require.NoError(t, err)
	assert.Equal(t, "active", assigned.Status)

	// Verify user was added to group
	var memberCount int64
	db.Model(&groupModels.GroupMember{}).Where("group_id = ? AND user_id = ? AND is_active = ?", groupID, targetUserID, true).Count(&memberCount)
	assert.Equal(t, int64(1), memberCount, "user should be added to group after license assignment")
}

func TestAssignLicense_WithoutGroupID_NoGroupEffect(t *testing.T) {
	db := freshTestDB(t)

	purchaserID := "purchaser-2"
	targetUserID := "target-user-2"

	// Create plan
	plan := &models.SubscriptionPlan{
		Name:     "Test Plan",
		IsActive: true,
	}
	require.NoError(t, db.Create(plan).Error)

	// Create batch WITHOUT GroupID
	batch := &models.SubscriptionBatch{
		PurchaserUserID:    purchaserID,
		SubscriptionPlanID: plan.ID,
		GroupID:            nil,
		TotalQuantity:      5,
		AssignedQuantity:   0,
		Status:             "active",
		CurrentPeriodStart: time.Now(),
		CurrentPeriodEnd:   time.Now().Add(30 * 24 * time.Hour),
	}
	require.NoError(t, db.Create(batch).Error)

	// Create unassigned license
	license := &models.UserSubscription{
		UserID:              "",
		PurchaserUserID:     &purchaserID,
		SubscriptionBatchID: &batch.ID,
		SubscriptionPlanID:  plan.ID,
		Status:              "unassigned",
		CurrentPeriodStart:  time.Now(),
		CurrentPeriodEnd:    time.Now().Add(30 * 24 * time.Hour),
	}
	require.NoError(t, db.Create(license).Error)

	// Assign the license
	svc := services.NewBulkLicenseService(db)
	assigned, err := svc.AssignLicense(batch.ID, purchaserID, targetUserID)
	require.NoError(t, err)
	assert.Equal(t, "active", assigned.Status)

	// Verify NO group_members rows were created
	var memberCount int64
	db.Model(&groupModels.GroupMember{}).Where("user_id = ?", targetUserID).Count(&memberCount)
	assert.Equal(t, int64(0), memberCount, "no group membership should be created when batch has no GroupID")
}

func TestAssignLicense_UserAlreadyInGroup_NoDuplicate(t *testing.T) {
	db := freshTestDB(t)

	purchaserID := "purchaser-3"
	targetUserID := "target-user-3"
	groupID := uuid.New()

	plan := &models.SubscriptionPlan{Name: "Test Plan", IsActive: true}
	require.NoError(t, db.Create(plan).Error)

	group := &groupModels.ClassGroup{
		Name: "test-group", DisplayName: "Test Group",
		OwnerUserID: purchaserID, IsActive: true, MaxMembers: 50,
	}
	group.ID = groupID
	require.NoError(t, db.Omit("Metadata").Create(group).Error)

	ownerMember := &groupModels.GroupMember{
		GroupID: groupID, UserID: purchaserID,
		Role: groupModels.GroupMemberRoleOwner, JoinedAt: time.Now(), IsActive: true,
	}
	require.NoError(t, db.Omit("Metadata").Create(ownerMember).Error)

	// Pre-add target user to group
	existingMember := &groupModels.GroupMember{
		GroupID: groupID, UserID: targetUserID,
		Role: groupModels.GroupMemberRoleMember, JoinedAt: time.Now(), IsActive: true,
	}
	require.NoError(t, db.Omit("Metadata").Create(existingMember).Error)

	batch := &models.SubscriptionBatch{
		PurchaserUserID: purchaserID, SubscriptionPlanID: plan.ID,
		GroupID: &groupID, TotalQuantity: 5, AssignedQuantity: 0, Status: "active",
		CurrentPeriodStart: time.Now(), CurrentPeriodEnd: time.Now().Add(30 * 24 * time.Hour),
	}
	require.NoError(t, db.Create(batch).Error)

	license := &models.UserSubscription{
		UserID: "", PurchaserUserID: &purchaserID,
		SubscriptionBatchID: &batch.ID, SubscriptionPlanID: plan.ID,
		Status: "unassigned", CurrentPeriodStart: time.Now(),
		CurrentPeriodEnd: time.Now().Add(30 * 24 * time.Hour),
	}
	require.NoError(t, db.Create(license).Error)

	svc := services.NewBulkLicenseService(db)
	_, err := svc.AssignLicense(batch.ID, purchaserID, targetUserID)
	require.NoError(t, err)

	var memberCount int64
	db.Model(&groupModels.GroupMember{}).Where("group_id = ? AND user_id = ? AND is_active = ?", groupID, targetUserID, true).Count(&memberCount)
	assert.Equal(t, int64(1), memberCount, "should not duplicate group membership")
}

func TestAssignLicense_GroupAddFails_LicenseStillAssigned(t *testing.T) {
	db := freshTestDB(t)

	purchaserID := "purchaser-4"
	targetUserID := "target-user-4"
	fakeGroupID := uuid.New()

	// Create plan
	plan := &models.SubscriptionPlan{
		Name:     "Test Plan",
		IsActive: true,
	}
	require.NoError(t, db.Create(plan).Error)

	// Create batch pointing to non-existent group
	batch := &models.SubscriptionBatch{
		PurchaserUserID:    purchaserID,
		SubscriptionPlanID: plan.ID,
		GroupID:            &fakeGroupID,
		TotalQuantity:      5,
		AssignedQuantity:   0,
		Status:             "active",
		CurrentPeriodStart: time.Now(),
		CurrentPeriodEnd:   time.Now().Add(30 * 24 * time.Hour),
	}
	require.NoError(t, db.Create(batch).Error)

	// Create unassigned license
	license := &models.UserSubscription{
		UserID:              "",
		PurchaserUserID:     &purchaserID,
		SubscriptionBatchID: &batch.ID,
		SubscriptionPlanID:  plan.ID,
		Status:              "unassigned",
		CurrentPeriodStart:  time.Now(),
		CurrentPeriodEnd:    time.Now().Add(30 * 24 * time.Hour),
	}
	require.NoError(t, db.Create(license).Error)

	// Assign the license — should succeed even though group add will fail
	svc := services.NewBulkLicenseService(db)
	assigned, err := svc.AssignLicense(batch.ID, purchaserID, targetUserID)
	require.NoError(t, err, "license assignment should succeed even if group add fails")
	assert.Equal(t, "active", assigned.Status)
	assert.Equal(t, targetUserID, assigned.UserID)
}

func TestRevokeLicense_UserStaysInGroup(t *testing.T) {
	db := freshTestDB(t)

	purchaserID := "purchaser-5"
	targetUserID := "target-user-5"
	groupID := uuid.New()

	// Create plan
	plan := &models.SubscriptionPlan{
		Name:     "Test Plan",
		IsActive: true,
	}
	require.NoError(t, db.Create(plan).Error)

	// Create group
	group := &groupModels.ClassGroup{
		Name:        "test-group",
		DisplayName: "Test Group",
		OwnerUserID: purchaserID,
		IsActive:    true,
		MaxMembers:  50,
	}
	group.ID = groupID
	require.NoError(t, db.Omit("Metadata").Create(group).Error)

	// Add purchaser as owner
	ownerMember := &groupModels.GroupMember{
		GroupID:  groupID,
		UserID:   purchaserID,
		Role:     groupModels.GroupMemberRoleOwner,
		JoinedAt: time.Now(),
		IsActive: true,
	}
	require.NoError(t, db.Omit("Metadata").Create(ownerMember).Error)

	// Create batch linked to group
	batch := &models.SubscriptionBatch{
		PurchaserUserID:    purchaserID,
		SubscriptionPlanID: plan.ID,
		GroupID:            &groupID,
		TotalQuantity:      5,
		AssignedQuantity:   0,
		Status:             "active",
		CurrentPeriodStart: time.Now(),
		CurrentPeriodEnd:   time.Now().Add(30 * 24 * time.Hour),
	}
	require.NoError(t, db.Create(batch).Error)

	// Create and assign license
	license := &models.UserSubscription{
		UserID:              "",
		PurchaserUserID:     &purchaserID,
		SubscriptionBatchID: &batch.ID,
		SubscriptionPlanID:  plan.ID,
		Status:              "unassigned",
		CurrentPeriodStart:  time.Now(),
		CurrentPeriodEnd:    time.Now().Add(30 * 24 * time.Hour),
	}
	require.NoError(t, db.Create(license).Error)

	// Assign the license (should auto-add to group)
	svc := services.NewBulkLicenseService(db)
	assigned, err := svc.AssignLicense(batch.ID, purchaserID, targetUserID)
	require.NoError(t, err)

	// Verify user is in group
	var memberCount int64
	db.Model(&groupModels.GroupMember{}).Where("group_id = ? AND user_id = ?", groupID, targetUserID).Count(&memberCount)
	require.Equal(t, int64(1), memberCount, "user should be in group after assignment")

	// Now revoke the license
	err = svc.RevokeLicense(assigned.ID, purchaserID)
	require.NoError(t, err)

	// Verify user is STILL in group
	db.Model(&groupModels.GroupMember{}).Where("group_id = ? AND user_id = ? AND is_active = ?", groupID, targetUserID, true).Count(&memberCount)
	assert.Equal(t, int64(1), memberCount, "user should stay in group after license revocation")
}
