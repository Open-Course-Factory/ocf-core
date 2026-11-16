package services

import (
	"errors"
	"slices"

	"soli/formations/src/auth/access"
	"soli/formations/src/courses/models"
	entityErrors "soli/formations/src/entityManagement/errors"
	genericService "soli/formations/src/entityManagement/services"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// GetOwnedGeneration loads a generation the caller owns (or any, for an administrator).
// Someone else's generation returns the same error as a missing one, so its existence is not revealed.
func GetOwnedGeneration(gs genericService.GenericService, id uuid.UUID, userID string, roles []string) (*models.Generation, error) {
	entity, err := gs.GetEntity(id, models.Generation{}, "Generation", nil)
	if err != nil {
		return nil, err
	}
	generation := entity.(*models.Generation)
	if userID == "" || (!access.IsAdmin(roles) && !slices.Contains(generation.OwnerIDs, userID)) {
		return nil, entityErrors.NewEntityNotFound("Generation", id)
	}
	return generation, nil
}

// CallerOwnsCourse reports whether the course exists (not soft-deleted) and the caller
// owns it; administrators bypass ownership. A database failure is returned as an error.
func CallerOwnsCourse(db *gorm.DB, courseID uuid.UUID, userID string, roles []string) (bool, error) {
	var course models.Course
	if err := db.Select("id", "owner_ids").First(&course, "id = ?", courseID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	return access.IsAdmin(roles) || (userID != "" && slices.Contains(course.OwnerIDs, userID)), nil
}
