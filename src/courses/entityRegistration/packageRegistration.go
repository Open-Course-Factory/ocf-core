package registration

import (
	"reflect"
	"soli/formations/src/courses/dto"
	"soli/formations/src/courses/models"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"

	"github.com/google/uuid"
)

type PackageRegistration struct {
	entityManagementInterfaces.AbstractRegistrableInterface
}

func (s PackageRegistration) EntityModelToEntityOutput(input any) (any, error) {
	if reflect.ValueOf(input).Kind() == reflect.Ptr {
		return packagePtrModelToPackageOutputDto(input.(*models.Package))
	} else {
		return packageValueModelToPackageOutputDto(input.(models.Package))
	}
}

func packagePtrModelToPackageOutputDto(packageModel *models.Package) (*dto.PackageOutput, error) {
	return dto.PackageModelToPackageOutput(*packageModel), nil
}

func packageValueModelToPackageOutputDto(packageModel models.Package) (*dto.PackageOutput, error) {
	return dto.PackageModelToPackageOutput(packageModel), nil
}

func (s PackageRegistration) EntityInputDtoToEntityModel(input any) any {

	packageInputDto, ok := input.(dto.PackageInput)
	if !ok {
		ptrPackageInputDto := input.(*dto.PackageInput)
		packageInputDto = *ptrPackageInputDto
	}

	packageToReturn := &models.Package{
		ThemeGitRepository:       packageInputDto.ThemeGitRepository,
		ThemeGitRepositoryBranch: packageInputDto.ThemeGitRepositoryBranch,
		ThemeId:                  packageInputDto.ThemeId,
		ScheduleID:               uuid.MustParse(packageInputDto.ScheduleId),
		CourseID:                 uuid.MustParse(packageInputDto.CourseId),
	}

	packageToReturn.OwnerIDs = append(packageToReturn.OwnerIDs, packageInputDto.OwnerID)

	return packageToReturn
}

func (s PackageRegistration) GetEntityRegistrationInput() entityManagementInterfaces.EntityRegistrationInput {
	return entityManagementInterfaces.EntityRegistrationInput{
		EntityInterface: models.Package{},
		EntityConverters: entityManagementInterfaces.EntityConverters{
			ModelToDto: s.EntityModelToEntityOutput,
			DtoToModel: s.EntityInputDtoToEntityModel,
			DtoToMap:   s.EntityDtoToMap,
		},
		EntityDtos: entityManagementInterfaces.EntityDtos{
			InputCreateDto: dto.PackageInput{},
			OutputDto:      dto.PackageOutput{},
		},
	}
}
