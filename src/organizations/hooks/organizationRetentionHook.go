package organizationHooks

import (
	"fmt"

	"soli/formations/src/entityManagement/hooks"
	"soli/formations/src/organizations/models"
	"soli/formations/src/organizations/services"
	"soli/formations/src/utils"

	"gorm.io/gorm"
)

// OrganizationRetentionAuthorizationHook makes retention_days owner-only on
// PATCH /organizations/:id. Managers may edit the organization (Layer 1 grants
// them PATCH), but how long a departed student's data is kept is the data
// controller's decision, and the UI hiding the field is not a rule.
type OrganizationRetentionAuthorizationHook struct {
	organizationService services.OrganizationService
}

func NewOrganizationRetentionAuthorizationHook(db *gorm.DB) hooks.Hook {
	return &OrganizationRetentionAuthorizationHook{organizationService: services.NewOrganizationService(db)}
}

func (h *OrganizationRetentionAuthorizationHook) GetName() string {
	return "organization_retention_authorization"
}
func (h *OrganizationRetentionAuthorizationHook) GetEntityName() string { return "Organization" }
func (h *OrganizationRetentionAuthorizationHook) GetHookTypes() []hooks.HookType {
	return []hooks.HookType{hooks.BeforeUpdate}
}
func (h *OrganizationRetentionAuthorizationHook) IsEnabled() bool  { return true }
func (h *OrganizationRetentionAuthorizationHook) GetPriority() int { return 5 }

func (h *OrganizationRetentionAuthorizationHook) Execute(ctx *hooks.HookContext) error {
	patch, ok := ctx.NewEntity.(map[string]any)
	if !ok {
		return fmt.Errorf("expected map[string]any for BeforeUpdate, got %T", ctx.NewEntity)
	}
	if _, present := patch["retention_days"]; !present || ctx.IsAdmin() {
		return nil
	}
	org, ok := ctx.OldEntity.(*models.Organization)
	if !ok {
		return fmt.Errorf("expected *models.Organization as OldEntity, got %T", ctx.OldEntity)
	}
	if ctx.UserID == "" {
		return utils.PermissionDeniedError("change the retention period of", "organization")
	}
	if org.IsOwner(ctx.UserID) {
		return nil
	}
	role, err := h.organizationService.GetUserOrganizationRole(org.ID, ctx.UserID)
	if err != nil || role != models.OrgRoleOwner {
		return utils.PermissionDeniedError("change the retention period of", "organization")
	}
	return nil
}
