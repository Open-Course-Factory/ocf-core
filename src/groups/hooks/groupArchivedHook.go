package groupHooks

import (
	"fmt"

	entityErrors "soli/formations/src/entityManagement/errors"
	"soli/formations/src/entityManagement/hooks"
	"soli/formations/src/groups/models"
	scenarioModels "soli/formations/src/scenarios/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// GroupArchivedHook refuses the writes that would give an archived class a
// future: enrolling a member (GroupMember.BeforeCreate) and assigning a
// scenario (ScenarioAssignment.BeforeCreate). Both refusals live here, on the
// class side, because the rule is the class's — "archived grants nothing" —
// not the assignment's. Bulk start applies the same rule in
// TeacherDashboardService.BulkStartScenario, which has no hook point.
//
// One hook type, registered once per guarded entity: what differs is only how
// the target class is read off the entity being created.
type GroupArchivedHook struct {
	db         *gorm.DB
	entityName string
}

func NewGroupArchivedHook(db *gorm.DB, entityName string) hooks.Hook {
	return &GroupArchivedHook{db: db, entityName: entityName}
}

func (h *GroupArchivedHook) GetName() string       { return "group_archived_" + h.entityName }
func (h *GroupArchivedHook) GetEntityName() string { return h.entityName }
func (h *GroupArchivedHook) GetHookTypes() []hooks.HookType {
	return []hooks.HookType{hooks.BeforeCreate}
}
func (h *GroupArchivedHook) IsEnabled() bool { return true }

// Before the validation hooks (10): a class that is archived has no capacity,
// expiry or duplicate-member question left to answer.
func (h *GroupArchivedHook) GetPriority() int { return 5 }

func (h *GroupArchivedHook) Execute(ctx *hooks.HookContext) error {
	groupID, err := targetClassOf(ctx.NewEntity)
	if err != nil {
		return err
	}
	if groupID == nil {
		return nil
	}

	var group models.ClassGroup
	if err := h.db.Select("archived_at").First(&group, "id = ?", *groupID).Error; err != nil {
		// Whether the class exists is the validation hooks' question, and
		// they answer it with their own error.
		return nil
	}
	if group.IsArchived() {
		// Structured so the generic create route answers 409, not a hook 500.
		return entityErrors.NewStateConflictError("ClassGroup", models.ErrClassArchived)
	}
	return nil
}

// targetClassOf reads the class a new row points at. nil means the row names
// no class (an organization-scoped assignment) and there is nothing to refuse.
func targetClassOf(entity any) (*uuid.UUID, error) {
	switch e := entity.(type) {
	case *models.GroupMember:
		return &e.GroupID, nil
	case *scenarioModels.ScenarioAssignment:
		return e.GroupID, nil
	default:
		return nil, fmt.Errorf("group_archived: unexpected entity %T", entity)
	}
}
