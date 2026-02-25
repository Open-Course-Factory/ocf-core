package groupHooks

import (
	"soli/formations/src/entityManagement/hooks"
	"soli/formations/src/groups/models"
	"soli/formations/src/utils"

	paymentModels "soli/formations/src/payment/models"

	"gorm.io/gorm"
)

// GroupMemberAutoLicenseHook auto-assigns an available license from a linked
// batch when a new member is added to a group. Only fires for the "member"
// role (owners/admins don't consume licenses). Non-blocking: silently returns
// nil if no batch or no available license exists.
type GroupMemberAutoLicenseHook struct {
	db       *gorm.DB
	enabled  bool
	priority int
}

func NewGroupMemberAutoLicenseHook(db *gorm.DB) hooks.Hook {
	return &GroupMemberAutoLicenseHook{
		db:       db,
		enabled:  true,
		priority: 30, // After permission hook (20)
	}
}

func (h *GroupMemberAutoLicenseHook) GetName() string {
	return "group_member_auto_license"
}

func (h *GroupMemberAutoLicenseHook) GetEntityName() string {
	return "GroupMember"
}

func (h *GroupMemberAutoLicenseHook) GetHookTypes() []hooks.HookType {
	return []hooks.HookType{hooks.AfterCreate}
}

func (h *GroupMemberAutoLicenseHook) IsEnabled() bool {
	return h.enabled
}

func (h *GroupMemberAutoLicenseHook) GetPriority() int {
	return h.priority
}

func (h *GroupMemberAutoLicenseHook) Execute(ctx *hooks.HookContext) error {
	member, ok := ctx.NewEntity.(*models.GroupMember)
	if !ok {
		return nil
	}

	// Only auto-assign for regular members, not owners or admins
	if member.Role != models.GroupMemberRoleMember {
		return nil
	}

	// Find an active batch linked to this group
	var batch paymentModels.SubscriptionBatch
	err := h.db.Where("group_id = ? AND status = ?", member.GroupID, "active").
		First(&batch).Error
	if err != nil {
		// No batch linked to group — silently return
		return nil
	}

	// Find an unassigned license in this batch
	var license paymentModels.UserSubscription
	err = h.db.Where("subscription_batch_id = ? AND status = ?", batch.ID, "unassigned").
		First(&license).Error
	if err != nil {
		// No available license — silently return
		return nil
	}

	// Assign the license to the new member
	license.UserID = member.UserID
	license.Status = "active"
	license.SubscriptionType = "assigned"
	if err := h.db.Save(&license).Error; err != nil {
		utils.Warn("Auto-license: failed to assign license %s to user %s: %v", license.ID, member.UserID, err)
		return nil
	}

	// Increment batch assigned quantity
	if err := h.db.Model(&paymentModels.SubscriptionBatch{}).
		Where("id = ?", batch.ID).
		Update("assigned_quantity", gorm.Expr("assigned_quantity + 1")).Error; err != nil {
		utils.Warn("Auto-license: failed to increment batch %s assigned quantity: %v", batch.ID, err)
	}

	utils.Info("Auto-license: assigned license %s to user %s from batch %s (group %s)",
		license.ID, member.UserID, batch.ID, member.GroupID)
	return nil
}
