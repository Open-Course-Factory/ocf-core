package groupHooks

import (
	"fmt"
	"log"
	"soli/formations/src/entityManagement/hooks"
	"soli/formations/src/groups/models"
	"soli/formations/src/groups/services"
	"soli/formations/src/utils"

	"gorm.io/gorm"
)

// GroupOwnerSetupHook sets the owner and creates the owner member when a group is created
type GroupOwnerSetupHook struct {
	db           *gorm.DB
	groupService services.GroupService
	enabled      bool
	priority     int
}

func NewGroupOwnerSetupHook(db *gorm.DB) hooks.Hook {
	return &GroupOwnerSetupHook{
		db:           db,
		groupService: services.NewGroupService(db),
		enabled:      true,
		priority:     10,
	}
}

func (h *GroupOwnerSetupHook) GetName() string {
	return "group_owner_setup"
}

func (h *GroupOwnerSetupHook) GetEntityName() string {
	return "ClassGroup"
}

func (h *GroupOwnerSetupHook) GetHookTypes() []hooks.HookType {
	return []hooks.HookType{hooks.BeforeCreate, hooks.AfterCreate}
}

func (h *GroupOwnerSetupHook) IsEnabled() bool {
	return h.enabled
}

func (h *GroupOwnerSetupHook) GetPriority() int {
	return h.priority
}

func (h *GroupOwnerSetupHook) Execute(ctx *hooks.HookContext) error {
	switch ctx.HookType {
	case hooks.BeforeCreate:
		// BeforeCreate receives the model (already converted from DTO)
		group, ok := ctx.NewEntity.(*models.ClassGroup)
		if !ok {
			return fmt.Errorf("expected *models.ClassGroup, got %T", ctx.NewEntity)
		}

		// Set the owner from the authenticated user context
		if ctx.UserID != "" {
			group.OwnerUserID = ctx.UserID
			utils.Debug("Setting group owner to %s", ctx.UserID)
		}

	case hooks.AfterCreate:
		group, ok := ctx.NewEntity.(*models.ClassGroup)
		if !ok {
			return fmt.Errorf("expected *models.ClassGroup, got %T", ctx.NewEntity)
		}
		// Add owner as member and grant permissions
		userID := group.OwnerUserID

		// Add owner as a member with owner role
		member := &models.GroupMember{
			GroupID:   group.ID,
			UserID:    userID,
			Role:      models.GroupMemberRoleOwner,
			InvitedBy: userID,
			JoinedAt:  group.CreatedAt,
			IsActive:  true,
		}

		err := h.db.Create(member).Error
		if err != nil {
			utils.Error("Failed to add owner as member: %v", err)
			return fmt.Errorf("failed to add owner as member: %v", err)
		}

		// Grant permissions to the owner
		err = h.groupService.GrantGroupPermissionsToUser(userID, group.ID)
		if err != nil {
			utils.Warn("Failed to grant permissions to group owner: %v", err)
		}

		utils.Info("Group created: %s (ID: %s) by user %s", group.Name, group.ID, userID)
	}

	return nil
}

// GroupCleanupHook revokes permissions when a group is deleted
type GroupCleanupHook struct {
	db           *gorm.DB
	groupService services.GroupService
	enabled      bool
	priority     int
}

func NewGroupCleanupHook(db *gorm.DB) hooks.Hook {
	return &GroupCleanupHook{
		db:           db,
		groupService: services.NewGroupService(db),
		enabled:      true,
		priority:     10,
	}
}

func (h *GroupCleanupHook) GetName() string {
	return "group_cleanup"
}

func (h *GroupCleanupHook) GetEntityName() string {
	return "ClassGroup"
}

func (h *GroupCleanupHook) GetHookTypes() []hooks.HookType {
	return []hooks.HookType{hooks.BeforeDelete}
}

func (h *GroupCleanupHook) IsEnabled() bool {
	return h.enabled
}

func (h *GroupCleanupHook) GetPriority() int {
	return h.priority
}

func (h *GroupCleanupHook) Execute(ctx *hooks.HookContext) error {
	group, ok := ctx.NewEntity.(*models.ClassGroup)
	if !ok {
		return fmt.Errorf("expected Group, got %T", ctx.NewEntity)
	}

	// Get all members and revoke their permissions
	var members []models.GroupMember
	err := h.db.Where("group_id = ?", group.ID).Find(&members).Error
	if err == nil {
		for _, member := range members {
			err = h.groupService.RevokeGroupPermissionsFromUser(member.UserID, group.ID)
			if err != nil {
				utils.Warn("Failed to revoke permissions from user %s: %v", member.UserID, err)
			}
		}
	}

	log.Printf("Group deleted: %s (ID: %s)", group.Name, group.ID)
	return nil
}

// GroupMemberValidationHook validates business rules when adding a member
type GroupMemberValidationHook struct {
	db           *gorm.DB
	groupService services.GroupService
	enabled      bool
	priority     int
}

func NewGroupMemberValidationHook(db *gorm.DB) hooks.Hook {
	return &GroupMemberValidationHook{
		db:           db,
		groupService: services.NewGroupService(db),
		enabled:      true,
		priority:     10, // Run before creation
	}
}

func (h *GroupMemberValidationHook) GetName() string {
	return "group_member_validation"
}

func (h *GroupMemberValidationHook) GetEntityName() string {
	return "GroupMember"
}

func (h *GroupMemberValidationHook) GetHookTypes() []hooks.HookType {
	return []hooks.HookType{hooks.BeforeCreate}
}

func (h *GroupMemberValidationHook) IsEnabled() bool {
	return h.enabled
}

func (h *GroupMemberValidationHook) GetPriority() int {
	return h.priority
}

func (h *GroupMemberValidationHook) Execute(ctx *hooks.HookContext) error {
	member, ok := ctx.NewEntity.(*models.GroupMember)
	if !ok {
		return fmt.Errorf("expected *models.GroupMember, got %T", ctx.NewEntity)
	}

	// 1. Check if group exists and load it
	var group models.ClassGroup
	err := h.db.Where("id = ?", member.GroupID).Preload("Members").First(&group).Error
	if err != nil {
		return fmt.Errorf("group not found")
	}

	// 2. Check if group is expired
	if group.IsExpired() {
		return fmt.Errorf("group has expired")
	}

	// 3. Check if group is full
	if group.MaxMembers > 0 && len(group.Members) >= group.MaxMembers {
		return utils.CapacityExceededError("group", len(group.Members), group.MaxMembers)
	}

	// 4. Check if user is already a member
	isMember, _ := h.groupService.IsUserInGroup(member.GroupID, member.UserID)
	if isMember {
		return fmt.Errorf("user is already a member of this group")
	}

	// 5. Check if requesting user can manage this group
	if ctx.UserID != "" {
		canManage, err := h.groupService.CanUserManageGroup(member.GroupID, ctx.UserID)
		if err != nil {
			return fmt.Errorf("permission check failed: %v", err)
		}
		if !canManage {
			return utils.PermissionDeniedError("add members to", "group")
		}

		// Set InvitedBy if not already set
		if member.InvitedBy == "" {
			member.InvitedBy = ctx.UserID
		}
	}

	utils.Debug("Validated group member: user %s joining group %s", member.UserID, member.GroupID)
	return nil
}

// GroupMemberPermissionHook grants permissions when a member is added
type GroupMemberPermissionHook struct {
	db           *gorm.DB
	groupService services.GroupService
	enabled      bool
	priority     int
}

func NewGroupMemberPermissionHook(db *gorm.DB) hooks.Hook {
	return &GroupMemberPermissionHook{
		db:           db,
		groupService: services.NewGroupService(db),
		enabled:      true,
		priority:     20, // Run after creation
	}
}

func (h *GroupMemberPermissionHook) GetName() string {
	return "group_member_permission"
}

func (h *GroupMemberPermissionHook) GetEntityName() string {
	return "GroupMember"
}

func (h *GroupMemberPermissionHook) GetHookTypes() []hooks.HookType {
	return []hooks.HookType{hooks.AfterCreate}
}

func (h *GroupMemberPermissionHook) IsEnabled() bool {
	return h.enabled
}

func (h *GroupMemberPermissionHook) GetPriority() int {
	return h.priority
}

func (h *GroupMemberPermissionHook) Execute(ctx *hooks.HookContext) error {
	member, ok := ctx.NewEntity.(*models.GroupMember)
	if !ok {
		return fmt.Errorf("expected *models.GroupMember, got %T", ctx.NewEntity)
	}

	// Grant group permissions to the new member
	err := h.groupService.GrantGroupPermissionsToUser(member.UserID, member.GroupID)
	if err != nil {
		utils.Warn("Failed to grant permissions to user %s: %v", member.UserID, err)
		// Don't fail the creation if permission grant fails
	}

	utils.Info("User %s added to group %s with role %s", member.UserID, member.GroupID, member.Role)
	return nil
}

// GroupMemberCleanupHook revokes permissions when a member is removed
type GroupMemberCleanupHook struct {
	db           *gorm.DB
	groupService services.GroupService
	enabled      bool
	priority     int
}

func NewGroupMemberCleanupHook(db *gorm.DB) hooks.Hook {
	return &GroupMemberCleanupHook{
		db:           db,
		groupService: services.NewGroupService(db),
		enabled:      true,
		priority:     10,
	}
}

func (h *GroupMemberCleanupHook) GetName() string {
	return "group_member_cleanup"
}

func (h *GroupMemberCleanupHook) GetEntityName() string {
	return "GroupMember"
}

func (h *GroupMemberCleanupHook) GetHookTypes() []hooks.HookType {
	return []hooks.HookType{hooks.BeforeDelete}
}

func (h *GroupMemberCleanupHook) IsEnabled() bool {
	return h.enabled
}

func (h *GroupMemberCleanupHook) GetPriority() int {
	return h.priority
}

func (h *GroupMemberCleanupHook) Execute(ctx *hooks.HookContext) error {
	member, ok := ctx.NewEntity.(*models.GroupMember)
	if !ok {
		return fmt.Errorf("expected *models.GroupMember, got %T", ctx.NewEntity)
	}

	// Prevent removing the group owner
	if member.Role == models.GroupMemberRoleOwner {
		return utils.ErrCannotRemoveOwner("group")
	}

	// Revoke permissions from the member
	err := h.groupService.RevokeGroupPermissionsFromUser(member.UserID, member.GroupID)
	if err != nil {
		utils.Warn("Failed to revoke permissions from user %s: %v", member.UserID, err)
	}

	utils.Info("User %s removed from group %s", member.UserID, member.GroupID)
	return nil
}
