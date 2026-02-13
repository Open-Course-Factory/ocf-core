package groupRegistration

import (
	"net/http"
	"time"

	authModels "soli/formations/src/auth/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/groups/dto"
	"soli/formations/src/groups/models"
	"soli/formations/src/utils"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
)

func enrichGroupMemberWithUser(output *dto.GroupMemberOutput) *dto.GroupMemberOutput {
	if output.UserID == "" {
		return output
	}

	user, err := casdoorsdk.GetUserByUserId(output.UserID)
	if err != nil {
		utils.Debug("Failed to fetch user %s from Casdoor: %v", output.UserID, err)
		return output
	}

	if user == nil {
		utils.Debug("User %s not found in Casdoor", output.UserID)
		return output
	}

	output.User = &dto.UserSummary{
		ID:          user.Id,
		Name:        user.Name,
		DisplayName: user.DisplayName,
		Email:       user.Email,
		Username:    user.Name,
	}

	if output.User.DisplayName == "" {
		output.User.DisplayName = user.Name
	}

	return output
}

func RegisterGroupMember(service *ems.EntityRegistrationService) {
	ems.RegisterTypedEntity[models.GroupMember, dto.CreateGroupMemberInput, dto.CreateGroupMemberInput, dto.GroupMemberOutput](
		service,
		"GroupMember",
		entityManagementInterfaces.TypedEntityRegistration[models.GroupMember, dto.CreateGroupMemberInput, dto.CreateGroupMemberInput, dto.GroupMemberOutput]{
			Converters: entityManagementInterfaces.TypedEntityConverters[models.GroupMember, dto.CreateGroupMemberInput, dto.CreateGroupMemberInput, dto.GroupMemberOutput]{
				ModelToDto: func(model *models.GroupMember) (dto.GroupMemberOutput, error) {
					output := dto.GroupMemberModelToGroupMemberOutput(model)
					output = enrichGroupMemberWithUser(output)
					return *output, nil
				},
				DtoToModel: func(input dto.CreateGroupMemberInput) *models.GroupMember {
					role := input.Role
					if role == "" {
						role = models.GroupMemberRoleMember
					}
					return &models.GroupMember{
						GroupID:   input.GroupID,
						UserID:    input.UserID,
						Role:      role,
						InvitedBy: input.InvitedBy,
						JoinedAt:  time.Now(),
						IsActive:  true,
					}
				},
				DtoToMap: func(input dto.CreateGroupMemberInput) map[string]any {
					return make(map[string]any)
				},
			},
			Roles: entityManagementInterfaces.EntityRoles{
				Roles: map[string]string{
					string(authModels.Member): "(" + http.MethodGet + ")",
					string(authModels.Admin):  "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodDelete + ")",
				},
			},
			MembershipConfig: &entityManagementInterfaces.MembershipConfig{
				MemberTable:      "group_members",
				EntityIDColumn:   "group_id",
				UserIDColumn:     "user_id",
				RoleColumn:       "role",
				IsActiveColumn:   "is_active",
				OrgAccessEnabled: false,
			},
			SwaggerConfig: &entityManagementInterfaces.EntitySwaggerConfig{
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
			},
		},
	)
}
