package organizationHooks

import (
	"fmt"
	access "soli/formations/src/auth/access"
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

	// 4. Check if requesting user can manage this organization.
	// Fail-closed: an unknown actor (empty UserID) reaching this externally-triggered
	// member-add hook must be denied, never skipped. (Seeding the org creator as owner
	// goes through the AfterCreate OwnerSetupHook, not this BeforeCreate hook.)
	if ctx.UserID == "" {
		return utils.PermissionDeniedError("add members to", "organization")
	}
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

	// 5. Default role to "member" if not set
	if member.Role == "" {
		member.Role = models.OrgRoleMember
	}

	// 5b. Cap the assigned role at the granter's own role: a granter must not mint a
	// member who outranks them (e.g. a manager cannot create an owner). Platform
	// administrators bypass the cap. The cap lives in the same authenticated branch as
	// the step-4 manage check, so internal flows with an empty actor (e.g. seeding the
	// org creator as owner) are unaffected.
	if ctx.UserID != "" && !ctx.IsAdmin() {
		granterRole, err := h.organizationService.GetUserOrganizationRole(org.ID, ctx.UserID)
		if err != nil {
			return utils.PermissionDeniedError("add members to", "organization")
		}
		if !access.IsRoleAtLeast(string(granterRole), string(member.Role)) {
			return utils.PermissionDeniedError("assign a role higher than your own in", "organization")
		}
	}

	// 6. Set JoinedAt and IsActive
	if member.JoinedAt.IsZero() {
		member.JoinedAt = time.Now()
	}
	member.IsActive = true

	utils.Debug("Validated organization member: user %s joining organization %s", member.UserID, member.OrganizationID)
	return nil
}

// OrganizationMemberUpdateAuthorizationHook authorizes a role/status change on an existing
// OrganizationMember via the generic PATCH route. It mirrors the create-side
// OrganizationMemberValidationHook (manage check + role cap) and the delete-side
// OrganizationMemberDeletionHook (owner protection): only an org manager/owner may change a
// member, no one may raise a member above the granter's own role, and an owner's role may not
// be changed through this path. Platform administrators bypass the role cap.
type OrganizationMemberUpdateAuthorizationHook struct {
	db                  *gorm.DB
	organizationService services.OrganizationService
	enabled             bool
	priority            int
}

func NewOrganizationMemberUpdateAuthorizationHook(db *gorm.DB) hooks.Hook {
	return &OrganizationMemberUpdateAuthorizationHook{
		db:                  db,
		organizationService: services.NewOrganizationService(db),
		enabled:             true,
		priority:            10, // Run before write, alongside the create/delete validation hooks
	}
}

func (h *OrganizationMemberUpdateAuthorizationHook) GetName() string {
	return "organization_member_update_authorization"
}

func (h *OrganizationMemberUpdateAuthorizationHook) GetEntityName() string {
	return "OrganizationMember"
}

func (h *OrganizationMemberUpdateAuthorizationHook) GetHookTypes() []hooks.HookType {
	return []hooks.HookType{hooks.BeforeUpdate}
}

func (h *OrganizationMemberUpdateAuthorizationHook) IsEnabled() bool {
	return h.enabled
}

func (h *OrganizationMemberUpdateAuthorizationHook) GetPriority() int {
	return h.priority
}

func (h *OrganizationMemberUpdateAuthorizationHook) Execute(ctx *hooks.HookContext) error {
	// The loaded current row is the source of truth for org and target identity; never trust
	// the patch for those. The patch only supplies the requested new values.
	current, ok := ctx.OldEntity.(*models.OrganizationMember)
	if !ok {
		return fmt.Errorf("expected *models.OrganizationMember for OldEntity, got %T", ctx.OldEntity)
	}
	patch, ok := ctx.NewEntity.(map[string]any)
	if !ok {
		return fmt.Errorf("expected map[string]any for BeforeUpdate patch, got %T", ctx.NewEntity)
	}

	// Fail-closed: an unknown actor (empty UserID) must never reach the manage check as a
	// skipped case. This mirrors the create/delete member hooks.
	if ctx.UserID == "" {
		return utils.PermissionDeniedError("update members of", "organization")
	}
	canManage, err := h.organizationService.CanUserManageOrganization(current.OrganizationID, ctx.UserID)
	if err != nil {
		return fmt.Errorf("permission check failed: %w", err)
	}
	if !canManage {
		return utils.PermissionDeniedError("update members of", "organization")
	}

	requestedRole, roleChangeRequested, err := requestedRoleFromPatch(patch)
	if err != nil {
		return err
	}
	if !roleChangeRequested {
		// A status/metadata-only patch is authorized by the manage check alone; there is no
		// role transition to cap or owner role to protect.
		return nil
	}

	// Owner protection: an owner's role may not be changed through this path, mirroring the
	// delete hook's cannot-remove-owner guard. Absolute, like that guard.
	if current.Role == models.OrgRoleOwner {
		return utils.PermissionDeniedError("change the role of the owner of", "organization")
	}

	// Role cap: a granter must not raise a member above the granter's own role (e.g. a manager
	// promoting to owner). Platform administrators bypass the cap, matching the create side.
	if !ctx.IsAdmin() {
		granterRole, err := h.organizationService.GetUserOrganizationRole(current.OrganizationID, ctx.UserID)
		if err != nil {
			return utils.PermissionDeniedError("update members of", "organization")
		}
		if !access.IsRoleAtLeast(string(granterRole), string(requestedRole)) {
			return utils.PermissionDeniedError("assign a role higher than your own in", "organization")
		}
	}

	utils.Debug("Authorized role change for member %s in organization %s by %s", current.UserID, current.OrganizationID, ctx.UserID)
	return nil
}

// requestedRoleFromPatch extracts the requested role from a generic update patch. The
// registration's DtoToMap stores the role under "role" as a models.OrganizationMemberRole
// value. Returns (role, true, nil) when a role change is requested, ("", false, nil) when the
// patch touches no role, and an error when the "role" value has an unexpected type.
func requestedRoleFromPatch(patch map[string]any) (models.OrganizationMemberRole, bool, error) {
	raw, present := patch["role"]
	if !present {
		return "", false, nil
	}
	role, ok := raw.(models.OrganizationMemberRole)
	if !ok {
		return "", false, fmt.Errorf("unexpected role type in update payload: %T", raw)
	}
	return role, true, nil
}

// OrganizationMemberPermissionHook keeps the Casbin grouping policies in sync with an
// OrganizationMember's role. On create it grants the base organization grouping and, for a
// manager-grade role, the manager grouping. On a role change it grants or revokes only the
// manager grouping, leaving base membership intact. Grant/revoke failures are logged and do
// not fail the write, mirroring GroupMemberPermissionHook.
type OrganizationMemberPermissionHook struct {
	db                  *gorm.DB
	organizationService services.OrganizationService
	enabled             bool
	priority            int
}

func NewOrganizationMemberPermissionHook(db *gorm.DB) hooks.Hook {
	return &OrganizationMemberPermissionHook{
		db:                  db,
		organizationService: services.NewOrganizationService(db),
		enabled:             true,
		priority:            20, // Run after creation/update
	}
}

func (h *OrganizationMemberPermissionHook) GetName() string {
	return "organization_member_permission"
}

func (h *OrganizationMemberPermissionHook) GetEntityName() string {
	return "OrganizationMember"
}

func (h *OrganizationMemberPermissionHook) GetHookTypes() []hooks.HookType {
	return []hooks.HookType{hooks.AfterCreate, hooks.AfterUpdate}
}

func (h *OrganizationMemberPermissionHook) IsEnabled() bool {
	return h.enabled
}

func (h *OrganizationMemberPermissionHook) GetPriority() int {
	return h.priority
}

func (h *OrganizationMemberPermissionHook) Execute(ctx *hooks.HookContext) error {
	switch ctx.HookType {
	case hooks.AfterCreate:
		return h.grantForNewMember(ctx)
	case hooks.AfterUpdate:
		return h.syncManagerGroupingOnRoleChange(ctx)
	}
	return nil
}

// grantForNewMember grants the base organization grouping to a freshly created member and,
// for a manager-grade role (manager or owner), additionally grants the manager grouping.
func (h *OrganizationMemberPermissionHook) grantForNewMember(ctx *hooks.HookContext) error {
	member, ok := ctx.NewEntity.(*models.OrganizationMember)
	if !ok {
		return fmt.Errorf("expected *models.OrganizationMember, got %T", ctx.NewEntity)
	}

	if err := h.organizationService.GrantOrganizationPermissions(member.UserID, member.OrganizationID); err != nil {
		utils.Warn("Failed to grant organization permissions to user %s: %v", member.UserID, err)
	}

	if member.IsManager() {
		if err := h.organizationService.GrantOrganizationManagerPermissions(member.UserID, member.OrganizationID); err != nil {
			utils.Warn("Failed to grant organization manager permissions to user %s: %v", member.UserID, err)
		}
	}

	utils.Info("Synced permissions for new organization member %s (role %s) in organization %s", member.UserID, member.Role, member.OrganizationID)
	return nil
}

// syncManagerGroupingOnRoleChange grants or revokes the manager grouping when a member's
// role crosses the manager threshold. The base grouping is left untouched: a demoted manager
// remains an organization member.
func (h *OrganizationMemberPermissionHook) syncManagerGroupingOnRoleChange(ctx *hooks.HookContext) error {
	newMember, ok := ctx.NewEntity.(*models.OrganizationMember)
	if !ok {
		return fmt.Errorf("expected *models.OrganizationMember, got %T", ctx.NewEntity)
	}
	oldMember, ok := ctx.OldEntity.(*models.OrganizationMember)
	if !ok {
		// Without the pre-update row we cannot tell whether the role crossed the manager
		// threshold; skip rather than risk an incorrect grant or revoke.
		utils.Warn("Skipping organization member permission sync: missing pre-update member for user %s", newMember.UserID)
		return nil
	}

	if oldMember.IsManager() == newMember.IsManager() {
		return nil
	}

	if newMember.IsManager() {
		if err := h.organizationService.GrantOrganizationManagerPermissions(newMember.UserID, newMember.OrganizationID); err != nil {
			utils.Warn("Failed to grant organization manager permissions to user %s: %v", newMember.UserID, err)
		}
		utils.Info("Promoted organization member %s to manager in organization %s", newMember.UserID, newMember.OrganizationID)
		return nil
	}

	if err := h.organizationService.RevokeOrganizationManagerPermissions(newMember.UserID, newMember.OrganizationID); err != nil {
		utils.Warn("Failed to revoke organization manager permissions from user %s: %v", newMember.UserID, err)
	}
	utils.Info("Demoted organization member %s from manager in organization %s", newMember.UserID, newMember.OrganizationID)
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

	// 2. Check if requesting user can manage this organization.
	// Fail-closed: an unknown actor (empty UserID) reaching this externally-triggered
	// member-removal hook must be denied, never skipped.
	if ctx.UserID == "" {
		return utils.PermissionDeniedError("remove members from", "organization")
	}
	canManage, err := h.organizationService.CanUserManageOrganization(member.OrganizationID, ctx.UserID)
	if err != nil {
		return fmt.Errorf("permission check failed: %w", err)
	}
	if !canManage {
		return utils.PermissionDeniedError("remove members from", "organization")
	}

	// 3. Revoke permissions from the member
	err = h.organizationService.RevokeOrganizationPermissions(member.UserID, member.OrganizationID)
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
