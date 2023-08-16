package services

import (
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/repositories"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type PermissionService interface {
	CreatePermission(permissionCreateDTO dto.CreatePermissionInput) (*dto.PermissionOutput, error)
	EditPermission(editedPermissionInput *dto.PermissionEditInput, id uuid.UUID) (*dto.PermissionEditOutput, error)
	//IsUserInstanceAdmin(permissions *[]models.Permission) bool
}

type permissionService struct {
	permissionRepository repositories.PermissionRepository
}

func NewPermissionService(db *gorm.DB) PermissionService {
	return &permissionService{
		permissionRepository: repositories.NewPermissionRepository(db),
	}
}

func (p permissionService) EditPermission(editedPermissionInput *dto.PermissionEditInput, id uuid.UUID) (*dto.PermissionEditOutput, error) {

	editPermission := editedPermissionInput

	editedPermission, userError := p.permissionRepository.EditPermission(id, *editPermission)

	if userError != nil {
		return nil, userError
	}

	return editedPermission, nil
}

func (p permissionService) CreatePermission(permissionCreateDTO dto.CreatePermissionInput) (*dto.PermissionOutput, error) {

	permission, createPermissionError := p.permissionRepository.CreatePermission(permissionCreateDTO)

	if createPermissionError != nil {
		return nil, createPermissionError
	}

	permissionOutput := &dto.PermissionOutput{
		ID:              permission.ID,
		PermissionTypes: permission.PermissionTypes,
	}

	return permissionOutput, nil

}

// func (p permissionService) IsUserInstanceAdmin(permissions *[]models.Permission) bool {
// 	for _, permission := range *permissions {
// 		if permission.Role.RoleName == models.RoleTypeInstanceAdmin {
// 			return true
// 		}
// 	}
// 	return false
// }
