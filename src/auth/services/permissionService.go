package services

import (
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/models"
	"soli/formations/src/auth/repositories"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type PermissionService interface {
	GetPermission(id uuid.UUID) (*models.Permission, error)
	GetPermissions() ([]dto.PermissionOutput, error)
	CreatePermission(permissionCreateDTO dto.CreatePermissionInput) (*dto.PermissionOutput, error)
	EditPermission(editedPermissionInput *dto.PermissionEditInput, id uuid.UUID) (*dto.PermissionEditOutput, error)
	DeletePermission(id uuid.UUID) error
	GetPermissionsByUser(id uuid.UUID) (*[]models.Permission, error)
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

func (p permissionService) DeletePermission(id uuid.UUID) error {
	errorDelete := p.repository.DeletePermission(id)
	if errorDelete != nil {
		return errorDelete
	}
	return nil
}

func (p permissionService) GetPermission(id uuid.UUID) (*models.Permission, error) {
	user, err := p.repository.GetPermission(id)

	if err != nil {
		return nil, err
	}

	return user, nil

}

func (p permissionService) GetPermissions() ([]dto.PermissionOutput, error) {

	permissions, err := p.repository.GetAllPermissions()

	if err != nil {
		return nil, err
	}

	var permissionsDto []dto.PermissionOutput

	for _, s := range *permissions {
		permissionsDto = append(permissionsDto, *dto.PermissionModelToPermissionOutput(s))
	}

	return permissionsDto, nil
}

func (p permissionService) CreatePermission(permissionCreateDTO dto.CreatePermissionInput) (*dto.PermissionOutput, error) {

	permission, createPermissionError := p.repository.CreatePermission(permissionCreateDTO)

	if createPermissionError != nil {
		return nil, createPermissionError
	}

	return &dto.PermissionOutput{
		ID:           permission.ID,
		User:         permission.User.ID,
		Role:         permission.Role.ID,
		Group:        permission.Group.ID,
		Organisation: permission.Organisation.ID,
		PermissionTypes: permission.PermissionTypes
	}, nil

}

func (p permissionService) GetPermissionsByUser(userId uuid.UUID) (*[]models.Permission, error) {

	permissions, createPermissionError := p.repository.GetPermissionsByUser(userId)

	if createPermissionError != nil {
		return nil, createPermissionError
	}

	return permissions, nil
}
