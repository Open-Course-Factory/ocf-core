package services

import (
	"slices"

	"soli/formations/src/auth/access"
	"soli/formations/src/courses/models"
	entityErrors "soli/formations/src/entityManagement/errors"
	genericService "soli/formations/src/entityManagement/services"

	"github.com/google/uuid"
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
