package scenarioRegistration

import (
	"net/http"

	authModels "soli/formations/src/auth/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"
)

func RegisterProjectFile(service *ems.EntityRegistrationService) {
	ems.RegisterTypedEntity[models.ProjectFile, dto.CreateProjectFileInput, dto.EditProjectFileInput, dto.ProjectFileOutput](
		service,
		"ProjectFile",
		entityManagementInterfaces.TypedEntityRegistration[models.ProjectFile, dto.CreateProjectFileInput, dto.EditProjectFileInput, dto.ProjectFileOutput]{
			Converters: entityManagementInterfaces.TypedEntityConverters[models.ProjectFile, dto.CreateProjectFileInput, dto.EditProjectFileInput, dto.ProjectFileOutput]{
				ModelToDto: func(model *models.ProjectFile) (dto.ProjectFileOutput, error) {
					return dto.ProjectFileOutput{
						ID:          model.ID,
						Filename:    model.Filename,
						RelPath:     model.RelPath,
						ContentType: model.ContentType,
						Content:     model.Content,
						StorageType: model.StorageType,
						StorageRef:  model.StorageRef,
						SizeBytes:   model.SizeBytes,
						CreatedAt:   model.CreatedAt,
						UpdatedAt:   model.UpdatedAt,
					}, nil
				},
				DtoToModel: func(input dto.CreateProjectFileInput) *models.ProjectFile {
					return &models.ProjectFile{
						Filename:    input.Filename,
						RelPath:     input.RelPath,
						ContentType: input.ContentType,
						Content:     input.Content,
						StorageType: input.StorageType,
						StorageRef:  input.StorageRef,
						SizeBytes:   input.SizeBytes,
					}
				},
				DtoToMap: func(input dto.EditProjectFileInput) map[string]any {
					updates := make(map[string]any)
					if input.Filename != nil {
						updates["filename"] = *input.Filename
					}
					if input.RelPath != nil {
						updates["rel_path"] = *input.RelPath
					}
					if input.ContentType != nil {
						updates["content_type"] = *input.ContentType
					}
					if input.Content != nil {
						updates["content"] = *input.Content
					}
					if input.StorageType != nil {
						updates["storage_type"] = *input.StorageType
					}
					if input.StorageRef != nil {
						updates["storage_ref"] = *input.StorageRef
					}
					if input.SizeBytes != nil {
						updates["size_bytes"] = *input.SizeBytes
					}
					return updates
				},
			},
			Roles: entityManagementInterfaces.EntityRoles{
				Roles: map[string]string{
					string(authModels.Admin): "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodPatch + "|" + http.MethodDelete + ")",
				},
			},
			SwaggerConfig: &entityManagementInterfaces.EntitySwaggerConfig{
				Tag:        "project-files",
				EntityName: "ProjectFile",
				GetAll: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "List all project files",
					Description: "Retrieve all project files",
					Tags:        []string{"project-files"},
					Security:    true,
				},
				GetOne: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Get a project file",
					Description: "Retrieve a specific project file by ID",
					Tags:        []string{"project-files"},
					Security:    true,
				},
				Create: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Create a project file",
					Description: "Create a new project file",
					Tags:        []string{"project-files"},
					Security:    true,
				},
				Update: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Update a project file",
					Description: "Update an existing project file",
					Tags:        []string{"project-files"},
					Security:    true,
				},
				Delete: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Delete a project file",
					Description: "Delete a project file",
					Tags:        []string{"project-files"},
					Security:    true,
				},
			},
		},
	)
}
