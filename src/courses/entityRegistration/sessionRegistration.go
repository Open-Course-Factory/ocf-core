package registration

import (
	"soli/formations/src/courses/dto"
	"soli/formations/src/courses/models"
	"soli/formations/src/entityManagement/converters"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
)

type SessionRegistration struct {
	entityManagementInterfaces.AbstractRegistrableInterface
}

func (s SessionRegistration) GetSwaggerConfig() entityManagementInterfaces.EntitySwaggerConfig {
	return entityManagementInterfaces.EntitySwaggerConfig{
		Tag:        "sessions",
		EntityName: "Session",
		GetAll: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Récupérer toutes les sessions",
			Description: "Retourne la liste de toutes les sessions disponibles",
			Tags:        []string{"sessions"},
			Security:    true,
		},
		GetOne: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Récupérer une session",
			Description: "Retourne les détails complets d'une session spécifique",
			Tags:        []string{"sessions"},
			Security:    true,
		},
		Create: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Créer une session",
			Description: "Crée une nouvelle session",
			Tags:        []string{"sessions"},
			Security:    true,
		},
		Update: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Mettre à jour une session",
			Description: "Modifie une session existante",
			Tags:        []string{"sessions"},
			Security:    true,
		},
		Delete: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Supprimer une session",
			Description: "Supprime une session",
			Tags:        []string{"sessions"},
			Security:    true,
		},
	}
}

func (s SessionRegistration) EntityModelToEntityOutput(input any) (any, error) {
	return converters.GenericModelToOutput(input, func(ptr any) (any, error) {
		sessionModel := ptr.(*models.Session)
		return &dto.CreateSessionOutput{
			ID:        sessionModel.ID.String(),
			CourseId:  sessionModel.CourseId,
			GroupId:   sessionModel.GroupId,
			StartTime: sessionModel.Beginning,
			EndTime:   sessionModel.End,
		}, nil
	})
}

func (s SessionRegistration) EntityInputDtoToEntityModel(input any) any {

	sessionInputDto := input.(dto.CreateSessionInput)
	return &models.Session{
		CourseId:  sessionInputDto.CourseId,
		Title:     sessionInputDto.Title,
		GroupId:   sessionInputDto.GroupId,
		Beginning: sessionInputDto.StartTime,
		End:       sessionInputDto.EndTime,
	}
}

func (s SessionRegistration) GetEntityRegistrationInput() entityManagementInterfaces.EntityRegistrationInput {
	return entityManagementInterfaces.EntityRegistrationInput{
		EntityInterface: models.Session{},
		EntityConverters: entityManagementInterfaces.EntityConverters{
			ModelToDto: s.EntityModelToEntityOutput,
			DtoToModel: s.EntityInputDtoToEntityModel,
		},
		EntityDtos: entityManagementInterfaces.EntityDtos{
			InputCreateDto: dto.CreateSessionInput{},
			OutputDto:      dto.CreateSessionOutput{},
		},
	}
}
