package organizationHooks

import (
	"fmt"
	"soli/formations/src/entityManagement/hooks"
	"soli/formations/src/organizations/models"
	"soli/formations/src/organizations/services"
	"soli/formations/src/utils"
	"time"

	"gorm.io/gorm"
)

// OrganizationPlanProtectionHook strips subscription_plan_id from create/update
// payloads when the caller is not an Administrator. This prevents Members from
// bypassing payment by injecting a paid plan UUID into the request body.
type OrganizationPlanProtectionHook struct {
	enabled  bool
	priority int
}

func NewOrganizationPlanProtectionHook() hooks.Hook {
	return &OrganizationPlanProtectionHook{
		enabled:  true,
		priority: 5, // Run before owner setup (priority 10)
	}
}

func (h *OrganizationPlanProtectionHook) GetName() string {
	return "organization_plan_protection"
}

func (h *OrganizationPlanProtectionHook) GetEntityName() string {
	return "Organization"
}

func (h *OrganizationPlanProtectionHook) GetHookTypes() []hooks.HookType {
	return []hooks.HookType{hooks.BeforeCreate, hooks.BeforeUpdate}
}

func (h *OrganizationPlanProtectionHook) IsEnabled() bool {
	return h.enabled
}

func (h *OrganizationPlanProtectionHook) GetPriority() int {
	return h.priority
}

func (h *OrganizationPlanProtectionHook) Execute(ctx *hooks.HookContext) error {
	if ctx.IsAdmin() {
		return nil
	}

	switch ctx.HookType {
	case hooks.BeforeCreate:
		org, ok := ctx.NewEntity.(*models.Organization)
		if !ok {
			return fmt.Errorf("expected *models.Organization, got %T", ctx.NewEntity)
		}
		if org.SubscriptionPlanID != nil {
			utils.Debug("Stripping member-supplied subscription_plan_id on org create for user %s", ctx.UserID)
			org.SubscriptionPlanID = nil
		}

	case hooks.BeforeUpdate:
		updateMap, ok := ctx.NewEntity.(map[string]any)
		if !ok {
			return fmt.Errorf("expected map[string]any for BeforeUpdate, got %T", ctx.NewEntity)
		}
		if _, present := updateMap["subscription_plan_id"]; present {
			utils.Debug("Stripping member-supplied subscription_plan_id on org update for user %s", ctx.UserID)
			delete(updateMap, "subscription_plan_id")
		}
	}

	return nil
}

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

// OrganizationMemberValidationHook validates business rules when adding a member to an organization
type OrganizationMemberValidationHook struct {
	db                  *gorm.DB
	organizationService services.OrganizationService
	enabled             bool
	priority            int
}

func NewOrganizationMemberValidationHook(db *gorm.DB) hooks.Hook {
	return &OrganizationMemberValidationHook{
		db:                  db,
		organizationService: services.NewOrganizationService(db),
		enabled:             true,
		priority:            10, // Run before creation
	}
}

func (h *OrganizationMemberValidationHook) GetName() string {
	return "organization_member_validation"
}

func (h *OrganizationMemberValidationHook) GetEntityName() string {
	return "OrganizationMember"
}

func (h *OrganizationMemberValidationHook) GetHookTypes() []hooks.HookType {
	return []hooks.HookType{hooks.BeforeCreate}
}

func (h *OrganizationMemberValidationHook) IsEnabled() bool {
	return h.enabled
}

func (h *OrganizationMemberValidationHook) GetPriority() int {
	return h.priority
}

func (h *OrganizationMemberValidationHook) Execute(ctx *hooks.HookContext) error {
	member, ok := ctx.NewEntity.(*models.OrganizationMember)
	if !ok {
		return fmt.Errorf("expected *models.OrganizationMember, got %T", ctx.NewEntity)
	}

	// 1. Check if organization exists
	var org models.Organization
	err := h.db.Where("id = ?", member.OrganizationID).Preload("Members").First(&org).Error
	if err != nil {
		return fmt.Errorf("organization not found")
	}

	// 2. Check if organization is full
	if org.MaxMembers > 0 && len(org.Members) >= org.MaxMembers {
		return utils.CapacityExceededError("organization", len(org.Members), org.MaxMembers)
	}

	// 3. Check if user is already a member
	isMember, _ := h.organizationService.IsUserInOrganization(org.ID, member.UserID)
	if isMember {
		return fmt.Errorf("user is already a member of this organization")
	}

	// 4. Check if requesting user can manage this organization
	if ctx.UserID != "" {
		canManage, err := h.organizationService.CanUserManageOrganization(org.ID, ctx.UserID)
		if err != nil {
			return fmt.Errorf("permission check failed: %w", err)
		}
		if !canManage {
			return utils.PermissionDeniedError("add members to", "organization")
		}

		// Set InvitedBy if not already set
		if member.InvitedBy == "" {
			member.InvitedBy = ctx.UserID
		}
	}

	// 5. Default role to "member" if not set
	if member.Role == "" {
		member.Role = models.OrgRoleMember
	}

	// 6. Set JoinedAt and IsActive
	if member.JoinedAt.IsZero() {
		member.JoinedAt = time.Now()
	}
	member.IsActive = true

	utils.Debug("Validated organization member: user %s joining organization %s", member.UserID, member.OrganizationID)
	return nil
}

// OrganizationMemberDeletionHook validates business rules when removing a member from an organization
type OrganizationMemberDeletionHook struct {
	db                  *gorm.DB
	organizationService services.OrganizationService
	enabled             bool
	priority            int
}

func NewOrganizationMemberDeletionHook(db *gorm.DB) hooks.Hook {
	return &OrganizationMemberDeletionHook{
		db:                  db,
		organizationService: services.NewOrganizationService(db),
		enabled:             true,
		priority:            10,
	}
}

func (h *OrganizationMemberDeletionHook) GetName() string {
	return "organization_member_deletion"
}

func (h *OrganizationMemberDeletionHook) GetEntityName() string {
	return "OrganizationMember"
}

func (h *OrganizationMemberDeletionHook) GetHookTypes() []hooks.HookType {
	return []hooks.HookType{hooks.BeforeDelete}
}

func (h *OrganizationMemberDeletionHook) IsEnabled() bool {
	return h.enabled
}

func (h *OrganizationMemberDeletionHook) GetPriority() int {
	return h.priority
}

func (h *OrganizationMemberDeletionHook) Execute(ctx *hooks.HookContext) error {
	member, ok := ctx.NewEntity.(*models.OrganizationMember)
	if !ok {
		return fmt.Errorf("expected *models.OrganizationMember, got %T", ctx.NewEntity)
	}

	// 1. Prevent removing the organization owner
	if member.Role == models.OrgRoleOwner {
		return utils.ErrCannotRemoveOwner("organization")
	}

	// 2. Check if requesting user can manage this organization
	if ctx.UserID != "" {
		canManage, err := h.organizationService.CanUserManageOrganization(member.OrganizationID, ctx.UserID)
		if err != nil {
			return fmt.Errorf("permission check failed: %w", err)
		}
		if !canManage {
			return utils.PermissionDeniedError("remove members from", "organization")
		}
	}

	// 3. Revoke permissions from the member
	err := h.organizationService.RevokeOrganizationPermissions(member.UserID, member.OrganizationID)
	if err != nil {
		utils.Warn("Failed to revoke member permissions from user %s: %v", member.UserID, err)
	}

	err = h.organizationService.RevokeOrganizationManagerPermissions(member.UserID, member.OrganizationID)
	if err != nil {
		utils.Warn("Failed to revoke manager permissions from user %s: %v", member.UserID, err)
	}

	utils.Info("User %s removed from organization %s", member.UserID, member.OrganizationID)
	return nil
}
