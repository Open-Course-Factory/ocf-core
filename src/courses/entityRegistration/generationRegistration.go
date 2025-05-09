package registration

import (
	"reflect"
	"soli/formations/src/courses/dto"
	"soli/formations/src/courses/models"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"

	"github.com/google/uuid"
)

type GenerationRegistration struct {
	entityManagementInterfaces.AbstractRegistrableInterface
}

func (s GenerationRegistration) EntityModelToEntityOutput(input any) (any, error) {
	if reflect.ValueOf(input).Kind() == reflect.Ptr {
		return generationPtrModelToGenerationOutputDto(input.(*models.Generation))
	} else {
		return packageValueModelToPackageOutputDto(input.(models.Generation))
	}
}

func generationPtrModelToGenerationOutputDto(packageModel *models.Generation) (*dto.GenerationOutput, error) {
	return dto.GenerationModelToGenerationOutput(*packageModel), nil
}

func packageValueModelToPackageOutputDto(packageModel models.Generation) (*dto.GenerationOutput, error) {
	return dto.GenerationModelToGenerationOutput(packageModel), nil
}

func (s GenerationRegistration) EntityInputDtoToEntityModel(input any) any {

	generationInputDto, ok := input.(dto.GenerationInput)
	if !ok {
		ptrGenerationInputDto := input.(*dto.GenerationInput)
		generationInputDto = *ptrGenerationInputDto
	}

	generationToReturn := &models.Generation{
		Format:     generationInputDto.Format,
		ThemeID:    uuid.MustParse(generationInputDto.ThemeId),
		ScheduleID: uuid.MustParse(generationInputDto.ScheduleId),
		CourseID:   uuid.MustParse(generationInputDto.CourseId),
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
