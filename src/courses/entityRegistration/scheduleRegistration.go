package registration

import (
	"soli/formations/src/courses/dto"
	"soli/formations/src/courses/models"
	"soli/formations/src/entityManagement/converters"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
)

type ScheduleRegistration struct {
	entityManagementInterfaces.AbstractRegistrableInterface
}

func (s ScheduleRegistration) GetSwaggerConfig() entityManagementInterfaces.EntitySwaggerConfig {
	return entityManagementInterfaces.EntitySwaggerConfig{
		Tag:        "schedules",
		EntityName: "Schedule",
		GetAll: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Récupérer tous les emploi du temps",
			Description: "Retourne la liste de tous les emploi du temps disponibles",
			Tags:        []string{"schedules"},
			Security:    true,
		},
		GetOne: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Récupérer un emploi du temps",
			Description: "Retourne les détails complets d'un emploi du temps spécifique",
			Tags:        []string{"schedules"},
			Security:    true,
		},
		Create: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Créer un emploi du temps",
			Description: "Crée un nouvel emploi du temps",
			Tags:        []string{"schedules"},
			Security:    true,
		},
		Update: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Mettre à jour un emploi du temps",
			Description: "Modifie un emploi du temps existant",
			Tags:        []string{"schedules"},
			Security:    true,
		},
		Delete: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Supprimer un emploi du temps",
			Description: "Supprime un emploi du temps",
			Tags:        []string{"schedules"},
			Security:    true,
		},
	}
}

func (s ScheduleRegistration) EntityModelToEntityOutput(input any) (any, error) {
	return converters.GenericModelToOutput(input, func(ptr any) (any, error) {
		scheduleModel := ptr.(*models.Schedule)
		return &dto.ScheduleOutput{
			ID:                 scheduleModel.ID.String(),
			Name:               scheduleModel.Name,
			FrontMatterContent: scheduleModel.FrontMatterContent,
			CreatedAt:          scheduleModel.CreatedAt.String(),
			UpdatedAt:          scheduleModel.UpdatedAt.String(),
		}, nil
	})
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
