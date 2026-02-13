package registration

import (
	"soli/formations/src/courses/dto"
	"soli/formations/src/courses/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
)

func RegisterSession(service *ems.EntityRegistrationService) {
	ems.RegisterTypedEntity[models.Session, dto.CreateSessionInput, dto.CreateSessionInput, dto.CreateSessionOutput](
		service,
		"Session",
		entityManagementInterfaces.TypedEntityRegistration[models.Session, dto.CreateSessionInput, dto.CreateSessionInput, dto.CreateSessionOutput]{
			Converters: entityManagementInterfaces.TypedEntityConverters[models.Session, dto.CreateSessionInput, dto.CreateSessionInput, dto.CreateSessionOutput]{
				ModelToDto: func(model *models.Session) (dto.CreateSessionOutput, error) {
					return dto.CreateSessionOutput{
						ID:        model.ID.String(),
						CourseId:  model.CourseId,
						GroupId:   model.GroupId,
						StartTime: model.Beginning,
						EndTime:   model.End,
					}, nil
				},
				DtoToModel: func(input dto.CreateSessionInput) *models.Session {
					return &models.Session{
						CourseId:  input.CourseId,
						Title:     input.Title,
						GroupId:   input.GroupId,
						Beginning: input.StartTime,
						End:       input.EndTime,
					}
				},
			},
			SwaggerConfig: &entityManagementInterfaces.EntitySwaggerConfig{
				Tag: "sessions", EntityName: "Session",
				GetAll:  &entityManagementInterfaces.SwaggerOperation{Summary: "Récupérer toutes les sessions", Description: "Retourne la liste de toutes les sessions disponibles", Tags: []string{"sessions"}, Security: true},
				GetOne:  &entityManagementInterfaces.SwaggerOperation{Summary: "Récupérer une session", Description: "Retourne les détails complets d'une session spécifique", Tags: []string{"sessions"}, Security: true},
				Create:  &entityManagementInterfaces.SwaggerOperation{Summary: "Créer une session", Description: "Crée une nouvelle session", Tags: []string{"sessions"}, Security: true},
				Update:  &entityManagementInterfaces.SwaggerOperation{Summary: "Mettre à jour une session", Description: "Modifie une session existante", Tags: []string{"sessions"}, Security: true},
				Delete:  &entityManagementInterfaces.SwaggerOperation{Summary: "Supprimer une session", Description: "Supprime une session", Tags: []string{"sessions"}, Security: true},
			},
		},
	)
}
