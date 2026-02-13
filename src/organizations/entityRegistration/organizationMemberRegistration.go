package entityRegistration

import (
	"net/http"

	authModels "soli/formations/src/auth/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/organizations/dto"
	"soli/formations/src/organizations/models"

	"github.com/google/uuid"
)

func RegisterOrganizationMember(service *ems.EntityRegistrationService) {
	ems.RegisterTypedEntity[models.OrganizationMember, dto.CreateOrganizationMemberInput, dto.EditOrganizationMemberInput, dto.OrganizationMemberOutput](
		service,
		"OrganizationMember",
		entityManagementInterfaces.TypedEntityRegistration[models.OrganizationMember, dto.CreateOrganizationMemberInput, dto.EditOrganizationMemberInput, dto.OrganizationMemberOutput]{
			Converters: entityManagementInterfaces.TypedEntityConverters[models.OrganizationMember, dto.CreateOrganizationMemberInput, dto.EditOrganizationMemberInput, dto.OrganizationMemberOutput]{
				ModelToDto: func(member *models.OrganizationMember) (dto.OrganizationMemberOutput, error) {
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
						orgReg := OrganizationRegistrationLegacy{}
						orgOutput, err := orgReg.EntityModelToEntityOutput(member.Organization)
						if err == nil {
							orgOutputTyped := orgOutput.(dto.OrganizationOutput)
							output.Organization = &orgOutputTyped
						}
					}

					return output, nil
				},
				DtoToModel: func(input dto.CreateOrganizationMemberInput) *models.OrganizationMember {
					return &models.OrganizationMember{
						UserID:   input.UserID,
						Role:     input.Role,
						Metadata: input.Metadata,
						IsActive: true,
					}
				},
				DtoToMap: func(input dto.EditOrganizationMemberInput) map[string]any {
					updates := make(map[string]any)
					if input.Role != nil {
						updates["role"] = *input.Role
					}
					if input.IsActive != nil {
						updates["is_active"] = *input.IsActive
					}
					if input.Metadata != nil {
						updates["metadata"] = *input.Metadata
					}
					return updates
				},
			},
			Roles: entityManagementInterfaces.EntityRoles{
				Roles: map[string]string{
					string(authModels.Member): "(" + http.MethodGet + ")",
					string(authModels.Admin):  "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodPatch + "|" + http.MethodDelete + ")",
				},
			},
			MembershipConfig: &entityManagementInterfaces.MembershipConfig{
				MemberTable:      "organization_members",
				EntityIDColumn:   "organization_id",
				UserIDColumn:     "user_id",
				RoleColumn:       "role",
				IsActiveColumn:   "is_active",
				OrgAccessEnabled: false,
				ManagerRoles:     []string{"owner", "manager"},
			},
			SwaggerConfig: &entityManagementInterfaces.EntitySwaggerConfig{
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
			},
		},
	)
}

// OrganizationRegistrationLegacy is kept for the OrganizationMember ModelToDto that needs to
// convert nested Organization models. It uses the typed registration's ops via the global service.
type OrganizationRegistrationLegacy struct{}

func (r OrganizationRegistrationLegacy) EntityModelToEntityOutput(input any) (any, error) {
	if ops, ok := ems.GlobalEntityRegistrationService.GetEntityOps("Organization"); ok {
		return ops.ConvertModelToDto(input)
	}
	return nil, nil
}
