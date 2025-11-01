package organizationHooks

import (
	"fmt"
	"soli/formations/src/entityManagement/hooks"
	"soli/formations/src/organizations/models"
	"soli/formations/src/organizations/services"
	"soli/formations/src/utils"

	"gorm.io/gorm"
)

// OrganizationOwnerSetupHook sets the owner and creates the owner member when an organization is created
type OrganizationOwnerSetupHook struct {
	db                  *gorm.DB
	organizationService services.OrganizationService
	enabled             bool
	priority            int
}

func NewOrganizationOwnerSetupHook(db *gorm.DB) hooks.Hook {
	return &OrganizationOwnerSetupHook{
		db:                  db,
		organizationService: services.NewOrganizationService(db),
		enabled:             true,
		priority:            10,
	}
}

func (h *OrganizationOwnerSetupHook) GetName() string {
	return "organization_owner_setup"
}

func (h *OrganizationOwnerSetupHook) GetEntityName() string {
	return "Organization"
}

func (h *OrganizationOwnerSetupHook) GetHookTypes() []hooks.HookType {
	return []hooks.HookType{hooks.BeforeCreate, hooks.AfterCreate}
}

func (h *OrganizationOwnerSetupHook) IsEnabled() bool {
	return h.enabled
}

func (h *OrganizationOwnerSetupHook) GetPriority() int {
	return h.priority
}

func (h *OrganizationOwnerSetupHook) Execute(ctx *hooks.HookContext) error {
	switch ctx.HookType {
	case hooks.BeforeCreate:
		// BeforeCreate receives the model (already converted from DTO)
		org, ok := ctx.NewEntity.(*models.Organization)
		if !ok {
			return fmt.Errorf("expected *models.Organization, got %T", ctx.NewEntity)
		}

		// Set the owner from the authenticated user context
		if ctx.UserID != "" {
			org.OwnerUserID = ctx.UserID
			utils.Debug("Setting organization owner to %s", ctx.UserID)
		}

	case hooks.AfterCreate:
		org, ok := ctx.NewEntity.(*models.Organization)
		if !ok {
			return fmt.Errorf("expected *models.Organization, got %T", ctx.NewEntity)
		}

		// Add owner as member and grant permissions
		// Use ctx.UserID as org.OwnerUserID is not set yet (it's set in addOwnerIDs after the hook runs)
		userID := ctx.UserID

		// Add owner as a member with owner role
		member := &models.OrganizationMember{
			OrganizationID: org.ID,
			UserID:         userID,
			Role:           models.OrgRoleOwner,
			InvitedBy:      userID,
			JoinedAt:       org.CreatedAt,
			IsActive:       true,
		}

		err := h.db.Create(member).Error
		if err != nil {
			utils.Error("Failed to add owner as member: %v", err)
			return fmt.Errorf("failed to add owner as member: %w", err)
		}

		// Grant permissions to the owner (both member and manager permissions)
		err = h.organizationService.GrantOrganizationPermissions(userID, org.ID)
		if err != nil {
			utils.Warn("Failed to grant member permissions to organization owner: %v", err)
		}

		err = h.organizationService.GrantOrganizationManagerPermissions(userID, org.ID)
		if err != nil {
			utils.Warn("Failed to grant manager permissions to organization owner: %v", err)
		}

		utils.Info("Organization created: %s (ID: %s) by user %s", org.Name, org.ID, userID)
	}

	return nil
}

// OrganizationCleanupHook revokes permissions when an organization is deleted
type OrganizationCleanupHook struct {
	db                  *gorm.DB
	organizationService services.OrganizationService
	enabled             bool
	priority            int
}

func NewOrganizationCleanupHook(db *gorm.DB) hooks.Hook {
	return &OrganizationCleanupHook{
		db:                  db,
		organizationService: services.NewOrganizationService(db),
		enabled:             true,
		priority:            10,
	}
}

func (h *OrganizationCleanupHook) GetName() string {
	return "organization_cleanup"
}

func (h *OrganizationCleanupHook) GetEntityName() string {
	return "Organization"
}

func (h *OrganizationCleanupHook) GetHookTypes() []hooks.HookType {
	return []hooks.HookType{hooks.BeforeDelete}
}

func (h *OrganizationCleanupHook) IsEnabled() bool {
	return h.enabled
}

func (h *OrganizationCleanupHook) GetPriority() int {
	return h.priority
}

func (h *OrganizationCleanupHook) Execute(ctx *hooks.HookContext) error {
	org, ok := ctx.NewEntity.(*models.Organization)
	if !ok {
		return fmt.Errorf("expected Organization, got %T", ctx.NewEntity)
	}

	// Prevent deletion of personal organizations
	if org.IsPersonalOrg() {
		return fmt.Errorf("cannot delete personal organization")
	}

	// Get all members and revoke their permissions
	var members []models.OrganizationMember
	err := h.db.Where("organization_id = ?", org.ID).Find(&members).Error
	if err == nil {
		for _, member := range members {
			err = h.organizationService.RevokeOrganizationPermissions(member.UserID, org.ID)
			if err != nil {
				utils.Warn("Failed to revoke member permissions from user %s: %v", member.UserID, err)
			}

			err = h.organizationService.RevokeOrganizationManagerPermissions(member.UserID, org.ID)
			if err != nil {
				utils.Warn("Failed to revoke manager permissions from user %s: %v", member.UserID, err)
			}
		}
	}

	utils.Info("Organization deleted: %s (ID: %s)", org.Name, org.ID)
	return nil
}
