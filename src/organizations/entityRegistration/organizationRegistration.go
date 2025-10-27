package entityRegistration

import (
	"net/http"

	authModels "soli/formations/src/auth/models"
	"soli/formations/src/entityManagement/converters"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/organizations/dto"
	"soli/formations/src/organizations/models"
)

type OrganizationRegistration struct {
	entityManagementInterfaces.AbstractRegistrableInterface
}

func (r OrganizationRegistration) GetSwaggerConfig() entityManagementInterfaces.EntitySwaggerConfig {
	return entityManagementInterfaces.EntitySwaggerConfig{
		Tag:        "organizations",
		EntityName: "Organization",
		GetAll: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "List all organizations",
			Description: "Retrieve all organizations (system admin only) or organizations the user is a member of",
			Security:    true,
		},
		GetOne: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Get organization details",
			Description: "Retrieve a specific organization by ID",
			Security:    true,
		},
		Create: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Create a new organization",
			Description: "Create a new organization. The creator becomes the organization owner.",
			Security:    true,
		},
		Update: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Update an organization",
			Description: "Update organization details (owner or manager only)",
			Security:    true,
		},
		Delete: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Delete an organization",
			Description: "Delete an organization (owner only, cannot delete personal organizations)",
			Security:    true,
		},
	}
}

func (r OrganizationRegistration) EntityModelToEntityOutput(input any) (any, error) {
	return converters.GenericModelToOutput(input, func(ptr any) (any, error) {
		org := ptr.(*models.Organization)

		output := dto.OrganizationOutput{
			ID:                 org.ID,
			Name:               org.Name,
			DisplayName:        org.DisplayName,
			Description:        org.Description,
			OwnerUserID:        org.OwnerUserID,
			SubscriptionPlanID: org.SubscriptionPlanID,
			IsPersonal:         org.IsPersonal,
			MaxGroups:          org.MaxGroups,
			MaxMembers:         org.MaxMembers,
			IsActive:           org.IsActive,
			Metadata:           org.Metadata,
			CreatedAt:          org.CreatedAt,
			UpdatedAt:          org.UpdatedAt,
		}

		// Include members if loaded
		if len(org.Members) > 0 {
			members := make([]dto.OrganizationMemberOutput, 0, len(org.Members))
			for _, member := range org.Members {
				memberOutput := dto.OrganizationMemberOutput{
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
				members = append(members, memberOutput)
			}
			output.Members = &members
		}

		// Include groups if loaded
		if len(org.Groups) > 0 {
			groups := make([]dto.GroupSummary, 0, len(org.Groups))
			for _, group := range org.Groups {
				groupSummary := dto.GroupSummary{
					ID:          group.ID,
					Name:        group.Name,
					DisplayName: group.DisplayName,
					MemberCount: group.GetMemberCount(),
					IsActive:    group.IsActive,
				}
				groups = append(groups, groupSummary)
			}
			output.Groups = &groups
		}

		// Add counts
		groupCount := len(org.Groups)
		memberCount := len(org.Members)
		output.GroupCount = &groupCount
		output.MemberCount = &memberCount

		return output, nil
	})
}

func (r OrganizationRegistration) EntityInputDtoToEntityModel(input any) any {
	inputDto := input.(dto.CreateOrganizationInput)

	org := models.Organization{
		Name:               inputDto.Name,
		DisplayName:        inputDto.DisplayName,
		Description:        inputDto.Description,
		SubscriptionPlanID: inputDto.SubscriptionPlanID,
		MaxGroups:          inputDto.MaxGroups,
		MaxMembers:         inputDto.MaxMembers,
		Metadata:           inputDto.Metadata,
		IsActive:           true,
	}

	// Set defaults
	if org.MaxGroups == 0 {
		org.MaxGroups = 10
	}
	if org.MaxMembers == 0 {
		org.MaxMembers = 50
	}

	return &org
}

func (r OrganizationRegistration) EntityDtoToMap(input any) map[string]any {
	editDto := input.(dto.EditOrganizationInput)
	updates := make(map[string]any)

	if editDto.Name != nil {
		updates["name"] = *editDto.Name
	}
	if editDto.DisplayName != nil {
		updates["display_name"] = *editDto.DisplayName
	}
	if editDto.Description != nil {
		updates["description"] = *editDto.Description
	}
	if editDto.SubscriptionPlanID != nil {
		updates["subscription_plan_id"] = *editDto.SubscriptionPlanID
	}
	if editDto.MaxGroups != nil {
		updates["max_groups"] = *editDto.MaxGroups
	}
	if editDto.MaxMembers != nil {
		updates["max_members"] = *editDto.MaxMembers
	}
	if editDto.IsActive != nil {
		updates["is_active"] = *editDto.IsActive
	}
	if editDto.Metadata != nil {
		updates["metadata"] = *editDto.Metadata
	}

	return updates
}

func (r OrganizationRegistration) GetEntityRegistrationInput() entityManagementInterfaces.EntityRegistrationInput {
	return entityManagementInterfaces.EntityRegistrationInput{
		EntityInterface: models.Organization{},
		EntityConverters: entityManagementInterfaces.EntityConverters{
			ModelToDto: r.EntityModelToEntityOutput,
			DtoToModel: r.EntityInputDtoToEntityModel,
			DtoToMap:   r.EntityDtoToMap,
		},
		EntityDtos: entityManagementInterfaces.EntityDtos{
			InputCreateDto: dto.CreateOrganizationInput{},
			OutputDto:      dto.OrganizationOutput{},
			InputEditDto:   dto.EditOrganizationInput{},
		},
		// EntitySubEntities removed - use ?include=Members,Groups query parameter instead
		// The automatic preloading doesn't work correctly when field names don't match pluralized type names
	}
}

func (r OrganizationRegistration) GetEntityRoles() entityManagementInterfaces.EntityRoles {
	roleMap := make(map[string]string)

	// Members can view and create organizations
	roleMap[string(authModels.Member)] = "(" + http.MethodGet + "|" + http.MethodPost + ")"

	// System admins have full access
	roleMap[string(authModels.Admin)] = "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodPatch + "|" + http.MethodDelete + ")"

	return entityManagementInterfaces.EntityRoles{
		Roles: roleMap,
	}
}
