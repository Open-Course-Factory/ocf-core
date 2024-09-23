package registration

import (
	"reflect"
	"soli/formations/src/courses/dto"
	"soli/formations/src/courses/models"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
)

type ScheduleRegistration struct {
	entityManagementInterfaces.AbstractRegistrableInterface
}

func (s ScheduleRegistration) EntityModelToEntityOutput(input any) (any, error) {
	if reflect.ValueOf(input).Kind() == reflect.Ptr {
		return schedulePtrModelToScheduleOutputDto(input.(*models.Schedule))
	} else {
		return scheduleValueModelToScheduleOutputDto(input.(models.Schedule))
	}
}

func schedulePtrModelToScheduleOutputDto(scheduleModel *models.Schedule) (*dto.ScheduleOutput, error) {

	return &dto.ScheduleOutput{
		ID:                 scheduleModel.ID.String(),
		Name:               scheduleModel.Name,
		FrontMatterContent: scheduleModel.FrontMatterContent,
		CreatedAt:          scheduleModel.CreatedAt.String(),
		UpdatedAt:          scheduleModel.UpdatedAt.String(),
	}, nil
}

func scheduleValueModelToScheduleOutputDto(scheduleModel models.Schedule) (*dto.ScheduleOutput, error) {

	return &dto.ScheduleOutput{
		ID:                 scheduleModel.ID.String(),
		Name:               scheduleModel.Name,
		FrontMatterContent: scheduleModel.FrontMatterContent,
		CreatedAt:          scheduleModel.CreatedAt.String(),
		UpdatedAt:          scheduleModel.UpdatedAt.String(),
	}, nil
}

func (s ScheduleRegistration) EntityInputDtoToEntityModel(input any) any {

	scheduleInputDto := input.(dto.ScheduleInput)
	return &models.Schedule{
		Name:               scheduleInputDto.Name,
		FrontMatterContent: scheduleInputDto.FrontMatterContent,
	}
}

func (s ScheduleRegistration) GetEntityRegistrationInput() entityManagementInterfaces.EntityRegistrationInput {
	return entityManagementInterfaces.EntityRegistrationInput{
		EntityInterface: models.Schedule{},
		EntityConverters: entityManagementInterfaces.EntityConverters{
			ModelToDto: s.EntityModelToEntityOutput,
			DtoToModel: s.EntityInputDtoToEntityModel,
			DtoToMap:   s.EntityDtoToMap,
		},
		EntityDtos: entityManagementInterfaces.EntityDtos{
			InputCreateDto: dto.ScheduleInput{},
			OutputDto:      dto.ScheduleOutput{},
			InputEditDto:   dto.EditScheduleInput{},
		},
	}
}
