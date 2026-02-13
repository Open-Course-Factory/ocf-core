package registration

import (
	"soli/formations/src/courses/dto"
	"soli/formations/src/courses/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
)

func RegisterSchedule(service *ems.EntityRegistrationService) {
	ems.RegisterTypedEntity[models.Schedule, dto.ScheduleInput, dto.EditScheduleInput, dto.ScheduleOutput](
		service,
		"Schedule",
		entityManagementInterfaces.TypedEntityRegistration[models.Schedule, dto.ScheduleInput, dto.EditScheduleInput, dto.ScheduleOutput]{
			Converters: entityManagementInterfaces.TypedEntityConverters[models.Schedule, dto.ScheduleInput, dto.EditScheduleInput, dto.ScheduleOutput]{
				ModelToDto: func(model *models.Schedule) (dto.ScheduleOutput, error) {
					return dto.ScheduleOutput{
						ID:                 model.ID.String(),
						Name:               model.Name,
						FrontMatterContent: model.FrontMatterContent,
						CreatedAt:          model.CreatedAt.String(),
						UpdatedAt:          model.UpdatedAt.String(),
					}, nil
				},
				DtoToModel: func(input dto.ScheduleInput) *models.Schedule {
					return &models.Schedule{
						Name:               input.Name,
						FrontMatterContent: input.FrontMatterContent,
					}
				},
			},
			SwaggerConfig: &entityManagementInterfaces.EntitySwaggerConfig{
				Tag: "schedules", EntityName: "Schedule",
				GetAll:  &entityManagementInterfaces.SwaggerOperation{Summary: "Récupérer tous les emploi du temps", Description: "Retourne la liste de tous les emploi du temps disponibles", Tags: []string{"schedules"}, Security: true},
				GetOne:  &entityManagementInterfaces.SwaggerOperation{Summary: "Récupérer un emploi du temps", Description: "Retourne les détails complets d'un emploi du temps spécifique", Tags: []string{"schedules"}, Security: true},
				Create:  &entityManagementInterfaces.SwaggerOperation{Summary: "Créer un emploi du temps", Description: "Crée un nouvel emploi du temps", Tags: []string{"schedules"}, Security: true},
				Update:  &entityManagementInterfaces.SwaggerOperation{Summary: "Mettre à jour un emploi du temps", Description: "Modifie un emploi du temps existant", Tags: []string{"schedules"}, Security: true},
				Delete:  &entityManagementInterfaces.SwaggerOperation{Summary: "Supprimer un emploi du temps", Description: "Supprime un emploi du temps", Tags: []string{"schedules"}, Security: true},
			},
		},
	)
}
