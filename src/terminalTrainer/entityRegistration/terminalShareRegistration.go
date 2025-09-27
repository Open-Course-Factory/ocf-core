package terminalRegistration

import (
	"net/http"
	"reflect"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/terminalTrainer/models"

	authModels "soli/formations/src/auth/models"
)

type TerminalShareRegistration struct {
	entityManagementInterfaces.AbstractRegistrableInterface
}

func (tsr TerminalShareRegistration) GetSwaggerConfig() entityManagementInterfaces.EntitySwaggerConfig {
	return entityManagementInterfaces.EntitySwaggerConfig{
		Tag:        "terminal-shares",
		EntityName: "TerminalShare",
		GetAll: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Récupérer tous les partages de terminaux",
			Description: "Retourne la liste de tous les partages de terminaux",
			Tags:        []string{"terminal-shares"},
			Security:    true,
		},
		GetOne: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Récupérer un partage de terminal",
			Description: "Retourne les détails d'un partage de terminal spécifique",
			Tags:        []string{"terminal-shares"},
			Security:    true,
		},
		Create: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Créer un partage de terminal",
			Description: "Crée un nouveau partage de terminal",
			Tags:        []string{"terminal-shares"},
			Security:    true,
		},
		Update: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Modifier un partage de terminal",
			Description: "Met à jour un partage de terminal existant",
			Tags:        []string{"terminal-shares"},
			Security:    true,
		},
		Delete: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Supprimer un partage de terminal",
			Description: "Supprime un partage de terminal",
			Tags:        []string{"terminal-shares"},
			Security:    true,
		},
	}
}

func (tsr TerminalShareRegistration) GetModel() interface{} {
	return models.TerminalShare{}
}

func (tsr TerminalShareRegistration) GetAllOutputDto() interface{} {
	return dto.TerminalShareOutput{}
}

func (tsr TerminalShareRegistration) GetOutputDto() interface{} {
	return dto.TerminalShareOutput{}
}

func (tsr TerminalShareRegistration) GetInputDto() interface{} {
	return dto.CreateTerminalShareInput{}
}

func (tsr TerminalShareRegistration) GetUpdateDto() interface{} {
	return dto.UpdateTerminalShareInput{}
}

func (tsr TerminalShareRegistration) ConvertModelToAllOutputDto(model interface{}) interface{} {
	terminalShareModel := model.(models.TerminalShare)
	return dto.TerminalShareOutput{
		ID:               terminalShareModel.ID,
		TerminalID:       terminalShareModel.TerminalID,
		SharedWithUserID: terminalShareModel.SharedWithUserID,
		SharedByUserID:   terminalShareModel.SharedByUserID,
		AccessLevel:      terminalShareModel.AccessLevel,
		ExpiresAt:        terminalShareModel.ExpiresAt,
		IsActive:         terminalShareModel.IsActive,
		CreatedAt:        terminalShareModel.CreatedAt,
	}
}

func (tsr TerminalShareRegistration) ConvertModelToOutputDto(model interface{}) interface{} {
	terminalShareModel := model.(models.TerminalShare)
	return dto.TerminalShareOutput{
		ID:               terminalShareModel.ID,
		TerminalID:       terminalShareModel.TerminalID,
		SharedWithUserID: terminalShareModel.SharedWithUserID,
		SharedByUserID:   terminalShareModel.SharedByUserID,
		AccessLevel:      terminalShareModel.AccessLevel,
		ExpiresAt:        terminalShareModel.ExpiresAt,
		IsActive:         terminalShareModel.IsActive,
		CreatedAt:        terminalShareModel.CreatedAt,
	}
}

func (tsr TerminalShareRegistration) ConvertInputDtoToModel(inputDto interface{}) interface{} {
	terminalShareInputDto := inputDto.(dto.CreateTerminalShareInput)
	return models.TerminalShare{
		TerminalID:       terminalShareInputDto.TerminalID,
		SharedWithUserID: terminalShareInputDto.SharedWithUserID,
		AccessLevel:      terminalShareInputDto.AccessLevel,
		ExpiresAt:        terminalShareInputDto.ExpiresAt,
		IsActive:         true, // Always active when created
	}
}

func (tsr TerminalShareRegistration) ConvertUpdateDtoToMap(updateDto interface{}) map[string]interface{} {
	terminalShareUpdateDto := updateDto.(dto.UpdateTerminalShareInput)
	updates := make(map[string]interface{})

	if terminalShareUpdateDto.AccessLevel != nil {
		updates["access_level"] = *terminalShareUpdateDto.AccessLevel
	}
	if terminalShareUpdateDto.ExpiresAt != nil {
		updates["expires_at"] = *terminalShareUpdateDto.ExpiresAt
	}
	if terminalShareUpdateDto.IsActive != nil {
		updates["is_active"] = *terminalShareUpdateDto.IsActive
	}

	return updates
}

func (tsr TerminalShareRegistration) GetCreateFieldsValidation() map[string]interface{} {
	return map[string]interface{}{
		"terminal_id":          "required,uuid",
		"shared_with_user_id":  "required,min=1",
		"access_level":         "required,oneof=read write admin",
		"expires_at":           "omitempty",
	}
}

func (tsr TerminalShareRegistration) GetUpdateFieldsValidation() map[string]interface{} {
	return map[string]interface{}{
		"access_level": "omitempty,oneof=read write admin",
		"expires_at":   "omitempty",
		"is_active":    "omitempty,boolean",
	}
}

func (tsr TerminalShareRegistration) GetQueryableFields() []string {
	return []string{"terminal_id", "shared_with_user_id", "shared_by_user_id", "access_level", "is_active"}
}

func (tsr TerminalShareRegistration) GetFilterableFields() []string {
	return []string{"terminal_id", "shared_with_user_id", "shared_by_user_id", "access_level", "is_active"}
}

func (tsr TerminalShareRegistration) GetUserOwnedFields() []string {
	return []string{"shared_by_user_id"}
}

func (tsr TerminalShareRegistration) GetRequiredPermissions() map[string][]string {
	return map[string][]string{
		"GET":    {"terminal_share:read"},
		"POST":   {"terminal_share:create"},
		"PUT":    {"terminal_share:update"},
		"DELETE": {"terminal_share:delete"},
	}
}


func (tsr TerminalShareRegistration) GetOrderableFields() []string {
	return []string{"created_at", "access_level", "expires_at"}
}

func (tsr TerminalShareRegistration) GetSearchableFields() []string {
	return []string{"shared_with_user_id", "shared_by_user_id", "access_level"}
}

func (tsr TerminalShareRegistration) CanUserAccess(user interface{}, requestMethod string, entityID interface{}) (bool, error) {
	// Only terminal owners and shared users can access terminal shares
	return true, nil // Let the controller handle the detailed access control
}

func (tsr TerminalShareRegistration) GetSoftDeleteField() string {
	return ""
}

func (tsr TerminalShareRegistration) GetEntityType() reflect.Type {
	return reflect.TypeOf(models.TerminalShare{})
}

func (tsr TerminalShareRegistration) GetStatusCode() map[string]int {
	return map[string]int{
		"GET":    http.StatusOK,
		"POST":   http.StatusCreated,
		"PUT":    http.StatusOK,
		"DELETE": http.StatusNoContent,
	}
}

func (tsr TerminalShareRegistration) GetEntityRegistrationInput() entityManagementInterfaces.EntityRegistrationInput {
	return entityManagementInterfaces.EntityRegistrationInput{
		EntityInterface: models.TerminalShare{},
		EntityConverters: entityManagementInterfaces.EntityConverters{
			ModelToDto: tsr.EntityModelToEntityOutput,
			DtoToModel: tsr.EntityInputDtoToEntityModel,
			DtoToMap:   tsr.EntityDtoToMap,
		},
		EntityDtos: entityManagementInterfaces.EntityDtos{
			InputCreateDto: dto.CreateTerminalShareInput{},
			OutputDto:      dto.TerminalShareOutput{},
			InputEditDto:   dto.UpdateTerminalShareInput{},
		},
	}
}

func (tsr TerminalShareRegistration) EntityModelToEntityOutput(input any) (any, error) {
	if reflect.ValueOf(input).Kind() == reflect.Ptr {
		return terminalSharePtrModelToTerminalShareOutput(input.(*models.TerminalShare))
	} else {
		return terminalShareValueModelToTerminalShareOutput(input.(models.TerminalShare))
	}
}

func terminalSharePtrModelToTerminalShareOutput(terminalShareModel *models.TerminalShare) (*dto.TerminalShareOutput, error) {
	return &dto.TerminalShareOutput{
		ID:               terminalShareModel.ID,
		TerminalID:       terminalShareModel.TerminalID,
		SharedWithUserID: terminalShareModel.SharedWithUserID,
		SharedByUserID:   terminalShareModel.SharedByUserID,
		AccessLevel:      terminalShareModel.AccessLevel,
		ExpiresAt:        terminalShareModel.ExpiresAt,
		IsActive:         terminalShareModel.IsActive,
		CreatedAt:        terminalShareModel.CreatedAt,
	}, nil
}

func terminalShareValueModelToTerminalShareOutput(terminalShareModel models.TerminalShare) (dto.TerminalShareOutput, error) {
	return dto.TerminalShareOutput{
		ID:               terminalShareModel.ID,
		TerminalID:       terminalShareModel.TerminalID,
		SharedWithUserID: terminalShareModel.SharedWithUserID,
		SharedByUserID:   terminalShareModel.SharedByUserID,
		AccessLevel:      terminalShareModel.AccessLevel,
		ExpiresAt:        terminalShareModel.ExpiresAt,
		IsActive:         terminalShareModel.IsActive,
		CreatedAt:        terminalShareModel.CreatedAt,
	}, nil
}

func (tsr TerminalShareRegistration) EntityInputDtoToEntityModel(input any) any {
	terminalShareInputDto := input.(dto.CreateTerminalShareInput)
	return &models.TerminalShare{
		TerminalID:       terminalShareInputDto.TerminalID,
		SharedWithUserID: terminalShareInputDto.SharedWithUserID,
		AccessLevel:      terminalShareInputDto.AccessLevel,
		ExpiresAt:        terminalShareInputDto.ExpiresAt,
		IsActive:         true,
	}
}

func (tsr TerminalShareRegistration) EntityDtoToMap(input any) map[string]any {
	terminalShareUpdateDto := input.(dto.UpdateTerminalShareInput)
	updates := make(map[string]any)

	if terminalShareUpdateDto.AccessLevel != nil {
		updates["access_level"] = *terminalShareUpdateDto.AccessLevel
	}
	if terminalShareUpdateDto.ExpiresAt != nil {
		updates["expires_at"] = *terminalShareUpdateDto.ExpiresAt
	}
	if terminalShareUpdateDto.IsActive != nil {
		updates["is_active"] = *terminalShareUpdateDto.IsActive
	}

	return updates
}

func (tsr TerminalShareRegistration) GetEntityRoles() entityManagementInterfaces.EntityRoles {
	roleMap := make(map[string]string)
	roleMap[string(authModels.Member)] = "(" + http.MethodGet + "|" + http.MethodPost + ")"
	roleMap[string(authModels.GroupManager)] = "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodDelete + ")"
	roleMap[string(authModels.Admin)] = "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodDelete + "|" + http.MethodPatch + ")"

	return entityManagementInterfaces.EntityRoles{
		Roles: roleMap,
	}
}