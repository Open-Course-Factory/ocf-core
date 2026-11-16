package courseHooks

import (
	"errors"
	"slices"

	"soli/formations/src/courses/models"
	entityErrors "soli/formations/src/entityManagement/errors"
	"soli/formations/src/entityManagement/hooks"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// GenerationCourseRequiredHook refuses a generic Generation create whose course
// id is missing, malformed, unknown or not owned by the caller (platform
// administrators bypass ownership). The converter cannot return an error, so it
// leaves CourseID nil and this hook answers the 400. Every refusal is the same
// answer so a caller cannot tell someone else's course from a missing one.
type GenerationCourseRequiredHook struct {
	hooks.BaseHook
	db *gorm.DB
}

func NewGenerationCourseRequiredHook(db *gorm.DB) hooks.Hook {
	return &GenerationCourseRequiredHook{
		BaseHook: hooks.BaseHook{
			Name:       "generation_course_required",
			EntityName: "Generation",
			HookTypes:  []hooks.HookType{hooks.BeforeCreate},
			Enabled:    true,
			Priority:   10,
		},
		db: db,
	}
}

func (h *GenerationCourseRequiredHook) Execute(ctx *hooks.HookContext) error {
	generation, ok := ctx.NewEntity.(*models.Generation)
	if !ok {
		return nil
	}
	refused := entityErrors.NewValidationError("courses", "must be a valid course id")
	if generation.CourseID == uuid.Nil {
		return refused
	}
	var course models.Course
	if err := h.db.Select("id", "owner_ids").First(&course, "id = ?", generation.CourseID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return refused
		}
		return err
	}
	if !ctx.IsAdmin() && (ctx.UserID == "" || !slices.Contains(course.OwnerIDs, ctx.UserID)) {
		return refused
	}
	return nil
}
