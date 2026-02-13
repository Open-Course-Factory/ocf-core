package registration

import (
	"soli/formations/src/courses/dto"
	"soli/formations/src/courses/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"

	"github.com/google/uuid"
)

func RegisterGeneration(service *ems.EntityRegistrationService) {
	ems.RegisterTypedEntity[models.Generation, dto.GenerationInput, dto.GenerationInput, dto.GenerationOutput](
		service,
		"Generation",
		entityManagementInterfaces.TypedEntityRegistration[models.Generation, dto.GenerationInput, dto.GenerationInput, dto.GenerationOutput]{
			Converters: entityManagementInterfaces.TypedEntityConverters[models.Generation, dto.GenerationInput, dto.GenerationInput, dto.GenerationOutput]{
				ModelToDto: func(model *models.Generation) (dto.GenerationOutput, error) {
					return *dto.GenerationModelToGenerationOutput(*model), nil
				},
				DtoToModel: func(input dto.GenerationInput) *models.Generation {
					gen := &models.Generation{
						Format:   input.Format,
						Name:     input.Name,
						CourseID: uuid.MustParse(input.CourseId),
					}
					themeId, errTheme := uuid.Parse(input.ThemeId)
					if errTheme == nil {
						gen.ThemeID = themeId
					}
					scheduleId, errSchedule := uuid.Parse(input.ScheduleId)
					if errSchedule == nil {
						gen.ScheduleID = scheduleId
					}
					gen.OwnerIDs = append(gen.OwnerIDs, input.OwnerID)
					return gen
				},
			},
		},
	)
}
