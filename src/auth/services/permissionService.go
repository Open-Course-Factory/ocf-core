package services

import (
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/models"
	"soli/formations/src/auth/repositories"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type PermissionService interface {
	CreatePermission(permissionCreateDTO dto.CreatePermissionInput) (*dto.PermissionOutput, error)
	EditPermission(editedPermissionInput *dto.PermissionEditInput, id uuid.UUID) (*dto.PermissionEditOutput, error)
	GetPermissionsByUser(id uuid.UUID) (*[]models.Permission, error)
	IsUserInstanceAdmin(permissions *[]models.Permission) bool
}

type permissionService struct {
	repository repositories.PermissionRepository
}

func NewPermissionService(db *gorm.DB) PermissionService {
	return &permissionService{
		repository: repositories.NewPermissionRepository(db),
	}
}

func (p permissionService) EditPermission(editedPermissionInput *dto.PermissionEditInput, id uuid.UUID) (*dto.PermissionEditOutput, error) {

	editPermission := editedPermissionInput

	editedPermission, userError := p.repository.EditPermission(id, *editPermission)

	if userError != nil {
		return nil, userError
	}

	return editedPermission, nil
}

func (p permissionService) CreatePermission(permissionCreateDTO dto.CreatePermissionInput) (*dto.PermissionOutput, error) {

	permission, createPermissionError := p.repository.CreatePermission(permissionCreateDTO)

	if createPermissionError != nil {
		return nil, createPermissionError
	}

	permissionOutput := &dto.PermissionOutput{
		ID:              permission.ID,
		User:            permission.User.ID,
		PermissionTypes: permission.PermissionTypes,
	}

	if permission.RoleID != nil {
		permissionOutput.Role = *permission.RoleID
	}

	if permission.GroupID != nil {
		permissionOutput.Group = *permission.GroupID
	}

	if permission.OrganisationID != nil {
		permissionOutput.Organisation = *permission.OrganisationID
	}

	return permissionOutput, nil

}

func (p permissionService) GetPermissionsByUser(userId uuid.UUID) (*[]models.Permission, error) {

	permissions, createPermissionError := p.repository.GetPermissionsByUser(userId)

	if createPermissionError != nil {
		return nil, createPermissionError
	}

	return permissions, nil
}

func (p permissionService) IsUserInstanceAdmin(permissions *[]models.Permission) bool {
	for _, permission := range *permissions {
		if permission.Role.RoleName == models.RoleTypeInstanceAdmin {
			return true
		}
	}
	return false
}
