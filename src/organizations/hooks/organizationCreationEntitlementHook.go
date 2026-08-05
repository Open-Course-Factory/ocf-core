package organizationHooks

import (
	"fmt"

	entityErrors "soli/formations/src/entityManagement/errors"
	"soli/formations/src/entityManagement/hooks"
	"soli/formations/src/organizations/models"
	paymentServices "soli/formations/src/payment/services"

	"gorm.io/gorm"
)

// OrganizationCreationEntitlementHook decides whether the caller may create a team
// organization at all.
//
// Trial exists to try terminals and Solo to use them alone; Formateur is the tier
// that buys the capacity to run classes, and an organization is the container
// classes live in. Team-org creation was nonetheless open to every authenticated
// Member — the generic entity route grants Member POST, and the only hooks on it
// stripped the plan id and set the owner — so a learner on Trial could stand up an
// organization no plan of theirs would ever let them use (#476).
//
// The verdict is not re-derived here. CanRunClassrooms owns it, and this is the
// third caller of the same computation: the personal-to-team conversion (#458) and
// the can_create_group flag (#453) already ask it the same question with the same
// nil organization context. Asking "does this user hold a classroom plan at all"
// is precisely a global question — there is no organization yet to ask it inside.
//
// Platform administrators bypass the check: they provision organizations for
// customers and hold no subscription of their own.
type OrganizationCreationEntitlementHook struct {
	plans    paymentServices.EffectivePlanService
	enabled  bool
	priority int
}

func NewOrganizationCreationEntitlementHook(db *gorm.DB) hooks.Hook {
	return &OrganizationCreationEntitlementHook{
		plans: paymentServices.NewEffectivePlanService(db),
		// Ahead of plan protection (5) and owner setup (10): refusing a creation
		// must not depend on another hook having run first, and there is no point
		// preparing an organization that is about to be rejected.
		priority: 1,
		enabled:  true,
	}
}

func (h *OrganizationCreationEntitlementHook) GetName() string {
	return "organization_creation_entitlement"
}

func (h *OrganizationCreationEntitlementHook) GetEntityName() string {
	return "Organization"
}

func (h *OrganizationCreationEntitlementHook) GetHookTypes() []hooks.HookType {
	return []hooks.HookType{hooks.BeforeCreate}
}

func (h *OrganizationCreationEntitlementHook) IsEnabled() bool  { return h.enabled }
func (h *OrganizationCreationEntitlementHook) GetPriority() int { return h.priority }

func (h *OrganizationCreationEntitlementHook) Execute(ctx *hooks.HookContext) error {
	// An unauthenticated context means this is not a user-facing write (startup
	// seeding, migrations, imports that carry their own authorization). There is
	// no caller to authorize, and failing closed here would break those paths.
	if ctx.UserID == "" {
		return nil
	}

	org, ok := ctx.NewEntity.(*models.Organization)
	if !ok {
		return fmt.Errorf("organization_creation_entitlement: unexpected payload %T", ctx.NewEntity)
	}

	// A personal workspace is not a classroom container, so creating one is not a
	// teaching capability. Stated against the type rather than left implicit in
	// CreateOrganizationInput having no organization_type field: signup's personal
	// org is what a user with no plan is entitled to, and that must stay true of
	// any future path that reaches this hook.
	//
	// Everything else is a team organization — including the empty type the
	// generic route's converter produces, which the model's BeforeSave normalizes
	// to "team".
	if org.IsPersonalOrg() {
		return nil
	}

	// Administrators operate the platform and are not customers of it.
	if ctx.IsAdmin() {
		return nil
	}

	verdict := h.plans.CanRunClassrooms(ctx.UserID, nil)
	if verdict.Allowed {
		return nil
	}

	return refuseOrganizationCreation(ctx.UserID, verdict.Reason)
}

// refuseOrganizationCreation builds the 403 the frontend renders its upgrade
// prompt from.
//
// Structured so WrapHookError preserves the status — a plain error becomes a
// generic ENT007/500, which reads as "we broke" rather than "you may not". The
// reason travels as the ClassroomDenied* code the verdict already carries, under
// the same classroom_denied_reason key the org-subscription DTO exposes, so the
// frontend keeps one mapping from code to upgrade prompt instead of learning a
// second vocabulary for the same refusal.
func refuseOrganizationCreation(userID, reason string) error {
	err := entityErrors.NewUnauthorizedError(userID, "Organization", "create")
	err.Message = "your subscription plan does not allow creating organizations"
	return err.WithDetails("classroom_denied_reason", reason)
}
