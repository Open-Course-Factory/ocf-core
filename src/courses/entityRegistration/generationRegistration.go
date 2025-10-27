package registration

import (
	"soli/formations/src/courses/dto"
	"soli/formations/src/courses/models"
	"soli/formations/src/entityManagement/converters"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"

	"github.com/google/uuid"
)

type GenerationRegistration struct {
	entityManagementInterfaces.AbstractRegistrableInterface
}

func (s GenerationRegistration) EntityModelToEntityOutput(input any) (any, error) {
	return converters.GenericModelToOutput(input, func(ptr any) (any, error) {
		return dto.GenerationModelToGenerationOutput(*ptr.(*models.Generation)), nil
	})
}

func (s GenerationRegistration) EntityInputDtoToEntityModel(input any) any {

	generationInputDto, ok := input.(dto.GenerationInput)
	if !ok {
		ptrGenerationInputDto := input.(*dto.GenerationInput)
		generationInputDto = *ptrGenerationInputDto
	}

	generationToReturn := &models.Generation{
		Format:   generationInputDto.Format,
		Name:     generationInputDto.Name,
		CourseID: uuid.MustParse(generationInputDto.CourseId),
	}

	themeId, errTheme := uuid.Parse(generationInputDto.ThemeId)
	if errTheme == nil {
		generationToReturn.ThemeID = themeId
	}

	scheduleId, errSchedule := uuid.Parse(generationInputDto.ScheduleId)
	if errSchedule == nil {
		generationToReturn.ScheduleID = scheduleId
	}

	generationToReturn.OwnerIDs = append(generationToReturn.OwnerIDs, generationInputDto.OwnerID)

	return generationToReturn
}

func (s GenerationRegistration) GetEntityRegistrationInput() entityManagementInterfaces.EntityRegistrationInput {
	return entityManagementInterfaces.EntityRegistrationInput{
		EntityInterface: models.Generation{},
		EntityConverters: entityManagementInterfaces.EntityConverters{
			ModelToDto: s.EntityModelToEntityOutput,
			DtoToModel: s.EntityInputDtoToEntityModel,
			DtoToMap:   s.EntityDtoToMap,
		},
		EntityDtos: entityManagementInterfaces.EntityDtos{
			InputCreateDto: dto.GenerationInput{},
			OutputDto:      dto.GenerationOutput{},
		},
	}
}
