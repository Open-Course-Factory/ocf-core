package entityRegistration

import (
	"net/http"

	authModels "soli/formations/src/auth/models"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/organizations/dto"
	"soli/formations/src/organizations/models"

	"github.com/google/uuid"
)

type OrganizationMemberRegistration struct {
	entityManagementInterfaces.AbstractRegistrableInterface
}

func (r OrganizationMemberRegistration) GetSwaggerConfig() entityManagementInterfaces.EntitySwaggerConfig {
	return entityManagementInterfaces.EntitySwaggerConfig{
		Tag:        "organization-members",
		EntityName: "OrganizationMember",
		GetAll: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "List all organization members",
			Description: "Retrieve all members of organizations (system admin only) or members of organizations the user belongs to",
			Security:    true,
		},
		GetOne: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Get organization member details",
			Description: "Retrieve a specific organization member by ID",
			Security:    true,
		},
		Create: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Add a member to an organization",
			Description: "Add a new member to an organization (owner or manager only)",
			Security:    true,
		},
		Update: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Update organization member",
			Description: "Update a member's role or status in an organization (owner or manager only)",
			Security:    true,
		},
		Delete: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Remove a member from an organization",
			Description: "Remove a member from an organization (owner or manager only, cannot remove owner)",
			Security:    true,
		},
	}
}

func (r OrganizationMemberRegistration) EntityModelToEntityOutput(input any) (any, error) {
	member := input.(models.OrganizationMember)

	output := dto.OrganizationMemberOutput{
		ID:             member.ID,
		OrganizationID: member.OrganizationID,
		UserID:         member.UserID,
		Role:           member.Role,
		InvitedBy:      member.InvitedBy,
		JoinedAt:       member.JoinedAt,
		IsActive:       member.IsActive,
		Metadata:       member.Metadata,
		CreatedAt:      member.CreatedAt,
		UpdatedAt:      member.UpdatedAt,
	}

	// Include organization if loaded
	if member.Organization.ID != uuid.Nil {
		orgRegistration := OrganizationRegistration{}
		orgOutput, err := orgRegistration.EntityModelToEntityOutput(member.Organization)
		if err == nil {
			orgOutputTyped := orgOutput.(dto.OrganizationOutput)
			output.Organization = &orgOutputTyped
		}
	}

	return output, nil
}

func (r OrganizationMemberRegistration) EntityInputDtoToEntityModel(input any) any {
	inputDto := input.(dto.CreateOrganizationMemberInput)

	member := models.OrganizationMember{
		UserID:   inputDto.UserID,
		Role:     inputDto.Role,
		Metadata: inputDto.Metadata,
		IsActive: true,
	}

	return member
}

func (r OrganizationMemberRegistration) EntityDtoToMap(input any) map[string]any {
	editDto := input.(dto.EditOrganizationMemberInput)
	updates := make(map[string]any)

	if editDto.Role != nil {
		updates["role"] = *editDto.Role
	}
	if editDto.IsActive != nil {
		updates["is_active"] = *editDto.IsActive
	}
	if editDto.Metadata != nil {
		updates["metadata"] = *editDto.Metadata
	}

	return updates
}

func (r OrganizationMemberRegistration) GetEntityRegistrationInput() entityManagementInterfaces.EntityRegistrationInput {
	return entityManagementInterfaces.EntityRegistrationInput{
		EntityInterface: models.OrganizationMember{},
		EntityConverters: entityManagementInterfaces.EntityConverters{
			ModelToDto: r.EntityModelToEntityOutput,
			DtoToModel: r.EntityInputDtoToEntityModel,
			DtoToMap:   r.EntityDtoToMap,
		},
		EntityDtos: entityManagementInterfaces.EntityDtos{
			InputCreateDto: dto.CreateOrganizationMemberInput{},
			OutputDto:      dto.OrganizationMemberOutput{},
			InputEditDto:   dto.EditOrganizationMemberInput{},
		},
	}
}

func (r OrganizationMemberRegistration) GetEntityRoles() entityManagementInterfaces.EntityRoles {
	roleMap := make(map[string]string)

	// Members can view organization members
	roleMap[string(authModels.Member)] = "(" + http.MethodGet + ")"

	// System admins have full access
	roleMap[string(authModels.Admin)] = "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodPatch + "|" + http.MethodDelete + ")"

	return entityManagementInterfaces.EntityRoles{
		Roles: roleMap,
	}
}
