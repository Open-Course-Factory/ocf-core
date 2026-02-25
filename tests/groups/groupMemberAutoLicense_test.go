package groups_tests

import (
	"testing"
	"time"

	groupHooks "soli/formations/src/groups/hooks"
	groupModels "soli/formations/src/groups/models"
	"soli/formations/src/entityManagement/hooks"
	"soli/formations/src/payment/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAutoLicense_MemberAddedWithLinkedBatch_LicenseAssigned(t *testing.T) {
	db := freshTestDB(t)

	purchaserID := "purchaser-1"
	memberUserID := "member-1"
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

	// Create batch linked to group
	batch := &models.SubscriptionBatch{
		PurchaserUserID:          purchaserID,
		SubscriptionPlanID:       plan.ID,
		GroupID:                  &groupID,
		StripeSubscriptionID:     "sub_test_" + uuid.New().String()[:8],
		StripeSubscriptionItemID: "si_test_" + uuid.New().String()[:8],
		TotalQuantity:            5,
		AssignedQuantity:         0,
		Status:                   "active",
		CurrentPeriodStart:       time.Now(),
		CurrentPeriodEnd:         time.Now().Add(30 * 24 * time.Hour),
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

	// Simulate adding member to group (hook fires)
	hook := groupHooks.NewGroupMemberAutoLicenseHook(db)
	member := &groupModels.GroupMember{
		GroupID:  groupID,
		UserID:   memberUserID,
		Role:     groupModels.GroupMemberRoleMember,
		JoinedAt: time.Now(),
		IsActive: true,
	}
	ctx := &hooks.HookContext{
		NewEntity: member,
		HookType:  hooks.AfterCreate,
	}
	err := hook.Execute(ctx)
	require.NoError(t, err)

	// Verify the license was assigned
	var updated models.UserSubscription
	db.Where("id = ?", license.ID).First(&updated)
	assert.Equal(t, "active", updated.Status)
	assert.Equal(t, memberUserID, updated.UserID)
	assert.Equal(t, "assigned", updated.SubscriptionType)

	// Verify batch assigned quantity incremented
	var updatedBatch models.SubscriptionBatch
	db.Where("id = ?", batch.ID).First(&updatedBatch)
	assert.Equal(t, 1, updatedBatch.AssignedQuantity)
}

func TestAutoLicense_NoLinkedBatch_NoEffect(t *testing.T) {
	db := freshTestDB(t)

	memberUserID := "member-2"
	groupID := uuid.New()

	// Create group with no linked batch
	group := &groupModels.ClassGroup{
		Name:        "test-group",
		DisplayName: "Test Group",
		OwnerUserID: "owner-1",
		IsActive:    true,
		MaxMembers:  50,
	}
	group.ID = groupID
	require.NoError(t, db.Omit("Metadata").Create(group).Error)

	// Simulate adding member (no batch exists for this group)
	hook := groupHooks.NewGroupMemberAutoLicenseHook(db)
	member := &groupModels.GroupMember{
		GroupID:  groupID,
		UserID:   memberUserID,
		Role:     groupModels.GroupMemberRoleMember,
		JoinedAt: time.Now(),
		IsActive: true,
	}
	ctx := &hooks.HookContext{
		NewEntity: member,
		HookType:  hooks.AfterCreate,
	}
	err := hook.Execute(ctx)
	assert.NoError(t, err, "should succeed silently when no batch is linked")

	// No licenses should exist at all
	var licenseCount int64
	db.Model(&models.UserSubscription{}).Where("user_id = ?", memberUserID).Count(&licenseCount)
	assert.Equal(t, int64(0), licenseCount)
}

func TestAutoLicense_AllLicensesAssigned_NoError(t *testing.T) {
	db := freshTestDB(t)

	purchaserID := "purchaser-3"
	memberUserID := "member-3"
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

	// Create batch with all licenses assigned (none available)
	batch := &models.SubscriptionBatch{
		PurchaserUserID:          purchaserID,
		SubscriptionPlanID:       plan.ID,
		GroupID:                  &groupID,
		StripeSubscriptionID:     "sub_test_" + uuid.New().String()[:8],
		StripeSubscriptionItemID: "si_test_" + uuid.New().String()[:8],
		TotalQuantity:            1,
		AssignedQuantity:         1,
		Status:                   "active",
		CurrentPeriodStart:       time.Now(),
		CurrentPeriodEnd:         time.Now().Add(30 * 24 * time.Hour),
	}
	require.NoError(t, db.Create(batch).Error)

	// Create an already-assigned license (no unassigned ones)
	license := &models.UserSubscription{
		UserID:              "existing-user",
		PurchaserUserID:     &purchaserID,
		SubscriptionBatchID: &batch.ID,
		SubscriptionPlanID:  plan.ID,
		Status:              "active",
		SubscriptionType:    "assigned",
		CurrentPeriodStart:  time.Now(),
		CurrentPeriodEnd:    time.Now().Add(30 * 24 * time.Hour),
	}
	require.NoError(t, db.Create(license).Error)

	// Simulate adding member — no license available
	hook := groupHooks.NewGroupMemberAutoLicenseHook(db)
	member := &groupModels.GroupMember{
		GroupID:  groupID,
		UserID:   memberUserID,
		Role:     groupModels.GroupMemberRoleMember,
		JoinedAt: time.Now(),
		IsActive: true,
	}
	ctx := &hooks.HookContext{
		NewEntity: member,
		HookType:  hooks.AfterCreate,
	}
	err := hook.Execute(ctx)
	assert.NoError(t, err, "should succeed silently when no licenses available")

	// Member should NOT have a license
	var licenseCount int64
	db.Model(&models.UserSubscription{}).Where("user_id = ?", memberUserID).Count(&licenseCount)
	assert.Equal(t, int64(0), licenseCount)
}

func TestAutoLicense_InactiveBatch_NoEffect(t *testing.T) {
	db := freshTestDB(t)

	purchaserID := "purchaser-4"
	memberUserID := "member-4"
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

	// Create INACTIVE batch
	batch := &models.SubscriptionBatch{
		PurchaserUserID:          purchaserID,
		SubscriptionPlanID:       plan.ID,
		GroupID:                  &groupID,
		StripeSubscriptionID:     "sub_test_" + uuid.New().String()[:8],
		StripeSubscriptionItemID: "si_test_" + uuid.New().String()[:8],
		TotalQuantity:            5,
		AssignedQuantity:         0,
		Status:                   "cancelled",
		CurrentPeriodStart:       time.Now(),
		CurrentPeriodEnd:         time.Now().Add(30 * 24 * time.Hour),
	}
	require.NoError(t, db.Create(batch).Error)

	// Create unassigned license in inactive batch
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

	// Simulate adding member — batch is inactive
	hook := groupHooks.NewGroupMemberAutoLicenseHook(db)
	member := &groupModels.GroupMember{
		GroupID:  groupID,
		UserID:   memberUserID,
		Role:     groupModels.GroupMemberRoleMember,
		JoinedAt: time.Now(),
		IsActive: true,
	}
	ctx := &hooks.HookContext{
		NewEntity: member,
		HookType:  hooks.AfterCreate,
	}
	err := hook.Execute(ctx)
	assert.NoError(t, err)

	// License should still be unassigned
	var updated models.UserSubscription
	db.Where("id = ?", license.ID).First(&updated)
	assert.Equal(t, "unassigned", updated.Status)
	assert.Equal(t, "", updated.UserID)
}

func TestAutoLicense_OwnerRole_SkipsAutoAssign(t *testing.T) {
	db := freshTestDB(t)

	purchaserID := "purchaser-5"
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

	// Create batch with unassigned license
	batch := &models.SubscriptionBatch{
		PurchaserUserID:          purchaserID,
		SubscriptionPlanID:       plan.ID,
		GroupID:                  &groupID,
		StripeSubscriptionID:     "sub_test_" + uuid.New().String()[:8],
		StripeSubscriptionItemID: "si_test_" + uuid.New().String()[:8],
		TotalQuantity:            5,
		AssignedQuantity:         0,
		Status:                   "active",
		CurrentPeriodStart:       time.Now(),
		CurrentPeriodEnd:         time.Now().Add(30 * 24 * time.Hour),
	}
	require.NoError(t, db.Create(batch).Error)

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

	// Simulate adding OWNER to group — should skip auto-assign
	hook := groupHooks.NewGroupMemberAutoLicenseHook(db)
	member := &groupModels.GroupMember{
		GroupID:  groupID,
		UserID:   purchaserID,
		Role:     groupModels.GroupMemberRoleOwner,
		JoinedAt: time.Now(),
		IsActive: true,
	}
	ctx := &hooks.HookContext{
		NewEntity: member,
		HookType:  hooks.AfterCreate,
	}
	err := hook.Execute(ctx)
	assert.NoError(t, err)

	// License should still be unassigned
	var updated models.UserSubscription
	db.Where("id = ?", license.ID).First(&updated)
	assert.Equal(t, "unassigned", updated.Status, "owner should not auto-consume a license")
	assert.Equal(t, "", updated.UserID)
}
