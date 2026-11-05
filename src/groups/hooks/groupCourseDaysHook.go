package groupHooks

import (
	entityErrors "soli/formations/src/entityManagement/errors"
	"soli/formations/src/entityManagement/hooks"
	"soli/formations/src/groups/models"

	"github.com/lib/pq"
)

// GroupCourseDaysValidationHook refuses course days that are not dates: binding tags
// are inert on generic entity routes.
type GroupCourseDaysValidationHook struct {
	hooks.BaseHook
}

func NewGroupCourseDaysValidationHook() hooks.Hook {
	return &GroupCourseDaysValidationHook{
		BaseHook: hooks.BaseHook{
			Name:       "group_course_days_validation",
			EntityName: "ClassGroup",
			HookTypes:  []hooks.HookType{hooks.BeforeUpdate},
			Enabled:    true,
			Priority:   6, // after GroupWriteAuthorizationHook: tell nothing to who may not write
		},
	}
}

func (h *GroupCourseDaysValidationHook) Execute(ctx *hooks.HookContext) error {
	patch, _ := ctx.NewEntity.(map[string]any)
	days, present := patch["course_days"].(pq.StringArray)
	if !present {
		return nil
	}
	if err := models.ValidateCourseDays(days); err != nil {
		return entityErrors.NewValidationError("course_days", err.Error())
	}
	return nil
}
