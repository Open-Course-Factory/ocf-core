package groupHooks

import (
	"errors"
	"fmt"

	"soli/formations/src/entityManagement/hooks"
	"soli/formations/src/groups/models"
	organizationModels "soli/formations/src/organizations/models"
	paymentServices "soli/formations/src/payment/services"
	"soli/formations/src/utils"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// GroupPlacementValidationHook decides whether a group may live in the
// organization it names.
//
// These rules already existed, fully written, in groupService.CreateGroup — which
// nothing ever called. Every group was in fact created through the generic entity
// route, whose only hook set the owner, so the API accepted any organization id a
// caller cared to send. That let a Member create a group inside an organization
// they were not a member of, and let groups land in personal organizations, which
// the product describes as having no collaboration at all (#452).
//
// It runs on BeforeUpdate as well as BeforeCreate because the edit path can MOVE a
// group between organizations: validating only creation would leave the same hole
// one PATCH away.
//
// Platform administrators bypass the permission checks (membership, role,
// entitlement) but NOT the structural ones (the organization must exist, must not
// be personal, must have room). Those are invariants of the data model rather than
// privileges — an administrator who needs more groups raises MaxGroups.
type GroupPlacementValidationHook struct {
	db       *gorm.DB
	plans    paymentServices.EffectivePlanService
	enabled  bool
	priority int
}

func NewGroupPlacementValidationHook(db *gorm.DB) hooks.Hook {
	return &GroupPlacementValidationHook{
		db:    db,
		plans: paymentServices.NewEffectivePlanService(db),
		// Ahead of GroupOwnerSetupHook (10): refusing a placement must not depend
		// on another hook having run first, and there is no point setting up an
		// owner for a group that is about to be rejected.
		priority: 5,
		enabled:  true,
	}
}

func (h *GroupPlacementValidationHook) GetName() string       { return "group_placement_validation" }
func (h *GroupPlacementValidationHook) GetEntityName() string { return "ClassGroup" }
func (h *GroupPlacementValidationHook) GetHookTypes() []hooks.HookType {
	return []hooks.HookType{hooks.BeforeCreate, hooks.BeforeUpdate}
}
func (h *GroupPlacementValidationHook) IsEnabled() bool  { return h.enabled }
func (h *GroupPlacementValidationHook) GetPriority() int { return h.priority }

func (h *GroupPlacementValidationHook) Execute(ctx *hooks.HookContext) error {
	// An unauthenticated context means this is not a user-facing write (startup
	// seeding, migrations, imports that carry their own authorization). There is
	// no caller to authorize, and failing closed here would break those paths.
	if ctx.UserID == "" {
		return nil
	}

	targetOrgID, err := h.targetOrganization(ctx)
	if err != nil {
		return err
	}
	// No organization named on this write — an update that does not touch the
	// placement. Renaming a class is not a placement decision and must not be
	// re-gated on entitlement, or a lapsed plan would freeze existing classes
	// rather than merely stopping new ones.
	if targetOrgID == nil {
		return nil
	}

	org, err := h.loadOrganization(*targetOrgID)
	if err != nil {
		return err
	}

	if err := h.validateStructure(org, h.editedGroupID(ctx)); err != nil {
		return err
	}

	if ctx.IsAdmin() {
		return nil
	}

	return h.validateCaller(ctx.UserID, org)
}

// targetOrganization extracts the organization this write places the group in.
//
// The two lifecycle events carry different payloads: BeforeCreate hands over the
// converted *ClassGroup, while BeforeUpdate hands over the DtoToMap map of
// column updates. Reading only the first shape would have made this hook error on
// every group edit; reading only the second would have left the create path
// unguarded.
//
// A nil return means "this write does not set an organization", which is a normal
// partial update, not a failure.
func (h *GroupPlacementValidationHook) targetOrganization(ctx *hooks.HookContext) (*uuid.UUID, error) {
	switch payload := ctx.NewEntity.(type) {
	case *models.ClassGroup:
		if payload.OrganizationID == nil || *payload.OrganizationID == uuid.Nil {
			return nil, fmt.Errorf("a group must belong to an organization")
		}
		return payload.OrganizationID, nil

	case map[string]any:
		raw, present := payload["organization_id"]
		if !present {
			return nil, nil
		}
		orgID, err := parseOrgID(raw)
		if err != nil {
			return nil, err
		}
		if orgID == uuid.Nil {
			return nil, fmt.Errorf("a group must belong to an organization")
		}
		return &orgID, nil

	default:
		return nil, fmt.Errorf("group_placement_validation: unexpected payload %T", ctx.NewEntity)
	}
}

// parseOrgID accepts the shapes organization_id arrives in: a uuid.UUID from
// DtoToMap, or a string when the update map came from a less typed path.
func parseOrgID(raw any) (uuid.UUID, error) {
	switch v := raw.(type) {
	case uuid.UUID:
		return v, nil
	case *uuid.UUID:
		if v == nil {
			return uuid.Nil, nil
		}
		return *v, nil
	case string:
		parsed, err := uuid.Parse(v)
		if err != nil {
			return uuid.Nil, fmt.Errorf("invalid organization id %q", v)
		}
		return parsed, nil
	default:
		return uuid.Nil, fmt.Errorf("invalid organization id of type %T", raw)
	}
}

// editedGroupID returns the id of the group being updated, or uuid.Nil on create.
// It is used to keep a group from counting against the limit it is already inside.
func (h *GroupPlacementValidationHook) editedGroupID(ctx *hooks.HookContext) uuid.UUID {
	if id, ok := ctx.EntityID.(uuid.UUID); ok {
		return id
	}
	return uuid.Nil
}

func (h *GroupPlacementValidationHook) loadOrganization(orgID uuid.UUID) (*organizationModels.Organization, error) {
	var org organizationModels.Organization
	if err := h.db.Where("id = ?", orgID).First(&org).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.EntityNotFoundError("organization")
		}
		return nil, fmt.Errorf("failed to load organization %s: %w", orgID, err)
	}
	return &org, nil
}

// validateStructure enforces the invariants that hold regardless of who is asking.
func (h *GroupPlacementValidationHook) validateStructure(
	org *organizationModels.Organization,
	editedGroupID uuid.UUID,
) error {
	// A personal organization is a workspace for one person. Advertising "1 member
	// only, collaboration not available" and then accepting classes into it is the
	// contradiction that started this issue.
	if org.IsPersonalOrg() {
		return fmt.Errorf("a personal organization cannot hold groups — convert it to a team organization first")
	}

	// Counted live rather than read off a preloaded association: the association is
	// not loaded on this path, and len(nil) would silently report an empty
	// organization, turning the limit into no limit.
	if org.MaxGroups > 0 {
		current, err := h.countGroups(org.ID, editedGroupID)
		if err != nil {
			return err
		}
		if current >= int64(org.MaxGroups) {
			return fmt.Errorf("organization has reached its group limit (%d groups)", org.MaxGroups)
		}
	}

	return nil
}

// countGroups counts the organization's groups, excluding the group being moved
// so that a PATCH which does not change the organization is not blocked by the
// group's own row.
func (h *GroupPlacementValidationHook) countGroups(orgID, excludeGroupID uuid.UUID) (int64, error) {
	query := h.db.Model(&models.ClassGroup{}).Where("organization_id = ?", orgID)
	if excludeGroupID != uuid.Nil {
		query = query.Where("id <> ?", excludeGroupID)
	}

	var count int64
	if err := query.Count(&count).Error; err != nil {
		return 0, fmt.Errorf("failed to count groups in organization %s: %w", orgID, err)
	}
	return count, nil
}

// validateCaller enforces what this particular user may do with this organization.
//
// One call: CanRunClassrooms owns membership, rank and plan together, resolved in
// the target organization's context — so a trainer gets the plan he actually holds
// there (his own, in the team org he created, which owns no plan; the school's,
// inside a school), and a student in a school is refused on rank even though he
// inherits a plan that grants classrooms.
//
// The hook translates the refusal code into a sentence; it does not re-derive the
// verdict. A second copy of any of these three checks is how this rule drifted
// into five disagreeing versions in the first place.
func (h *GroupPlacementValidationHook) validateCaller(userID string, org *organizationModels.Organization) error {
	verdict := h.plans.CanRunClassrooms(userID, &org.ID)
	if verdict.Allowed {
		return nil
	}

	switch verdict.Reason {
	case paymentServices.ClassroomDeniedNotOrgMember:
		return fmt.Errorf("you are not a member of this organization")
	case paymentServices.ClassroomDeniedInsufficientOrgRole:
		return fmt.Errorf("only organization teachers and managers can create groups in this organization")
	case paymentServices.ClassroomDeniedPersonalOrg:
		return fmt.Errorf("a personal organization cannot hold groups — convert it to a team organization first")
	case paymentServices.ClassroomDeniedNoPlan:
		return fmt.Errorf("no active subscription plan allows managing groups")
	default:
		return fmt.Errorf("your subscription plan does not allow managing groups")
	}
}
