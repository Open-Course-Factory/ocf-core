package groupRegistration

import (
	"net/http"
	"reflect"
	authModels "soli/formations/src/auth/models"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/groups/dto"
	"soli/formations/src/groups/models"
	"time"
)

type GroupMemberRegistration struct {
	entityManagementInterfaces.AbstractRegistrableInterface
}

func (gm GroupMemberRegistration) GetSwaggerConfig() entityManagementInterfaces.EntitySwaggerConfig {
	return entityManagementInterfaces.EntitySwaggerConfig{
		Tag:        "class-group-members",
		EntityName: "ClassGroupMember",
		GetAll: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Récupérer tous les membres de groupe",
			Description: "Retourne la liste de tous les membres de groupes",
			Tags:        []string{"class-group-members"},
			Security:    true,
		},
		GetOne: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Récupérer un membre de groupe",
			Description: "Retourne les détails d'un membre de groupe spécifique",
			Tags:        []string{"class-group-members"},
			Security:    true,
		},
		Delete: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Retirer un membre d'un groupe",
			Description: "Retire un membre d'un groupe",
			Tags:        []string{"class-group-members"},
			Security:    true,
		},
	}
}

func (gm GroupMemberRegistration) EntityModelToEntityOutput(input any) (any, error) {
	if reflect.ValueOf(input).Kind() == reflect.Ptr {
		return groupMemberPtrModelToGroupMemberOutput(input.(*models.GroupMember))
	} else {
		return groupMemberValueModelToGroupMemberOutput(input.(models.GroupMember))
	}
}

func groupMemberPtrModelToGroupMemberOutput(memberModel *models.GroupMember) (*dto.GroupMemberOutput, error) {
	return dto.GroupMemberModelToGroupMemberOutput(memberModel), nil
}

func groupMemberValueModelToGroupMemberOutput(memberModel models.GroupMember) (*dto.GroupMemberOutput, error) {
	return dto.GroupMemberModelToGroupMemberOutput(&memberModel), nil
}

func (gm GroupMemberRegistration) EntityInputDtoToEntityModel(input any) any {
	// This won't be used directly - group members are added via GroupService
	// But we need to implement it for the interface
	return &models.GroupMember{
		Role:     models.GroupMemberRoleMember,
		JoinedAt: time.Now(),
		IsActive: true,
	}
}

func (gm GroupMemberRegistration) GetEntityRegistrationInput() entityManagementInterfaces.EntityRegistrationInput {
	return entityManagementInterfaces.EntityRegistrationInput{
		EntityInterface: models.GroupMember{},
		EntityConverters: entityManagementInterfaces.EntityConverters{
			ModelToDto: gm.EntityModelToEntityOutput,
			DtoToModel: gm.EntityInputDtoToEntityModel,
			DtoToMap:   gm.EntityDtoToMap,
		},
		EntityDtos: entityManagementInterfaces.EntityDtos{
			OutputDto: dto.GroupMemberOutput{},
		},
	}
}

func (gm GroupMemberRegistration) EntityDtoToMap(input any) map[string]any {
	// Group members are managed through the GroupService, not direct updates
	return make(map[string]any)
}

// GetEntityRoles defines permissions for group members
func (gm GroupMemberRegistration) GetEntityRoles() entityManagementInterfaces.EntityRoles {
	roleMap := make(map[string]string)

	// Members can view group members
	roleMap[string(authModels.Member)] = "(" + http.MethodGet + ")"

	// GroupManagers can view and remove members
	roleMap[string(authModels.GroupManager)] = "(" + http.MethodGet + "|" + http.MethodDelete + ")"

	// Trainers and above have full access
	roleMap[string(authModels.Trainer)] = "(" + http.MethodGet + "|" + http.MethodDelete + ")"
	roleMap[string(authModels.Organization)] = "(" + http.MethodGet + "|" + http.MethodDelete + ")"
	roleMap[string(authModels.Admin)] = "(" + http.MethodGet + "|" + http.MethodDelete + ")"

	return entityManagementInterfaces.EntityRoles{
		Roles: roleMap,
	}
}
