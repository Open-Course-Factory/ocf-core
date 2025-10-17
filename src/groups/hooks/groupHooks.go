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
