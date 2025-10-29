package groupRegistration

import (
	"net/http"
	"time"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	authModels "soli/formations/src/auth/models"
	"soli/formations/src/entityManagement/converters"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/groups/dto"
	"soli/formations/src/groups/models"
	"soli/formations/src/utils"
)

type GroupMemberRegistration struct {
	entityManagementInterfaces.AbstractRegistrableInterface
}

// enrichGroupMemberWithUser fetches user data from Casdoor and adds it to the output
func enrichGroupMemberWithUser(output *dto.GroupMemberOutput) *dto.GroupMemberOutput {
	if output.UserID == "" {
		return output
	}

	// Fetch user from Casdoor
	user, err := casdoorsdk.GetUserByUserId(output.UserID)
	if err != nil {
		utils.Debug("Failed to fetch user %s from Casdoor: %v", output.UserID, err)
		return output
	}

	if user == nil {
		utils.Debug("User %s not found in Casdoor", output.UserID)
		return output
	}

	// Populate user summary
	output.User = &dto.UserSummary{
		ID:          user.Id,
		Name:        user.Name,
		DisplayName: user.DisplayName,
		Email:       user.Email,
		Username:    user.Name, // Casdoor uses Name as username
	}

	// Fallback to name if display name is empty
	if output.User.DisplayName == "" {
		output.User.DisplayName = user.Name
	}

	return output
}

func (gm GroupMemberRegistration) GetSwaggerConfig() entityManagementInterfaces.EntitySwaggerConfig {
	return entityManagementInterfaces.EntitySwaggerConfig{
		Tag:        "group-members",
		EntityName: "GroupMember",
		GetAll: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Récupérer tous les membres de groupe",
			Description: "Retourne la liste de tous les membres de groupes",
			Tags:        []string{"group-members"},
			Security:    true,
		},
		GetOne: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Récupérer un membre de groupe",
			Description: "Retourne les détails d'un membre de groupe spécifique",
			Tags:        []string{"group-members"},
			Security:    true,
		},
		Create: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Ajouter un membre à un groupe",
			Description: "Ajoute un utilisateur à un groupe avec un rôle spécifique",
			Tags:        []string{"group-members"},
			Security:    true,
		},
		Delete: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Retirer un membre d'un groupe",
			Description: "Retire un membre d'un groupe",
			Tags:        []string{"group-members"},
			Security:    true,
		},
	}
}

func (gm GroupMemberRegistration) EntityModelToEntityOutput(input any) (any, error) {
	return converters.GenericModelToOutput(input, func(ptr any) (any, error) {
		member := ptr.(*models.GroupMember)
		output := dto.GroupMemberModelToGroupMemberOutput(member)

		// Enrich with user data from Casdoor
		output = enrichGroupMemberWithUser(output)

		return output, nil
	})
}

func (gm GroupMemberRegistration) EntityInputDtoToEntityModel(input any) any {
	inputDto := input.(dto.CreateGroupMemberInput)

	// Default role to "member" if not specified
	role := inputDto.Role
	if role == "" {
		role = models.GroupMemberRoleMember
	}

	return &models.GroupMember{
		GroupID:   inputDto.GroupID,
		UserID:    inputDto.UserID,
		Role:      role,
		InvitedBy: inputDto.InvitedBy,
		JoinedAt:  time.Now(),
		IsActive:  true,
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
			InputCreateDto: dto.CreateGroupMemberInput{},
			OutputDto:      dto.GroupMemberOutput{},
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

	// GroupManagers can view, add, and remove members
	roleMap[string(authModels.GroupManager)] = "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodDelete + ")"

	// Trainers and above have full access
	roleMap[string(authModels.Trainer)] = "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodDelete + ")"
	roleMap[string(authModels.Organization)] = "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodDelete + ")"
	roleMap[string(authModels.Admin)] = "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodDelete + ")"

	return entityManagementInterfaces.EntityRoles{
		Roles: roleMap,
	}
}
