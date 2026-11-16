package courseHooks

import (
	"soli/formations/src/courses/models"
	entityErrors "soli/formations/src/entityManagement/errors"
	"soli/formations/src/entityManagement/hooks"

	"github.com/google/uuid"
)

// GenerationCourseRequiredHook refuses a generic Generation create whose course
// id is missing or malformed: the converter cannot return an error, so it leaves
// CourseID nil and this hook answers the 400.
type GenerationCourseRequiredHook struct {
	hooks.BaseHook
}

func NewGenerationCourseRequiredHook() hooks.Hook {
	return &GenerationCourseRequiredHook{
		BaseHook: hooks.BaseHook{
			Name:       "generation_course_required",
			EntityName: "Generation",
			HookTypes:  []hooks.HookType{hooks.BeforeCreate},
			Enabled:    true,
			Priority:   10,
		},
	}
}

func (h *GenerationCourseRequiredHook) Execute(ctx *hooks.HookContext) error {
	generation, ok := ctx.NewEntity.(*models.Generation)
	if ok && generation.CourseID == uuid.Nil {
		return entityErrors.NewValidationError("courses", "must be a valid course id")
	}
	return nil
}
