package services

import (
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/models"
	"soli/formations/src/auth/repositories"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type PermissionAssociationService interface {
	GetPermissionAssociation(id uuid.UUID) (*models.PermissionAssociation, error)
	GetPermissionAssociations() ([]dto.PermissionAssociationOutput, error)
	CreatePermissionAssociation(permissionAssociationCreateDTO dto.PermissionAssociationInput) (*dto.PermissionAssociationOutput, error)
	//EditPermissionAssociation(editedPermissionAssociationInput *dto.PermissionAssociationInput, id uuid.UUID) (*dto.PermissionAssociationOutput, error)
	DeletePermissionAssociation(id uuid.UUID) error
}

type permissionAssociationService struct {
	repository repositories.PermissionAssociationRepository
}

func NewPermissionAssociationService(db *gorm.DB) PermissionAssociationService {
	return &permissionAssociationService{
		repository: repositories.NewPermissionAssociationRepository(db),
	}
}

// func (p permissionAssociationService) EditPermissionAssociation(editedPermissionAssociationInput *dto.PermissionAssociationEditInput, id uuid.UUID) (*dto.PermissionAssociationEditOutput, error) {

// 	editPermissionAssociation := editedPermissionAssociationInput

// 	editedPermissionAssociation, userError := p.repository.EditPermissionAssociation(id, *editPermissionAssociation)

// 	if userError != nil {
// 		return nil, userError
// 	}

// 	return editedPermissionAssociation, nil
// }

func (p permissionAssociationService) DeletePermissionAssociation(id uuid.UUID) error {
	errorDelete := p.repository.DeletePermissionAssociation(id)
	if errorDelete != nil {
		return errorDelete
	}
	return nil
}

func (p permissionAssociationService) GetPermissionAssociation(id uuid.UUID) (*models.PermissionAssociation, error) {
	user, err := p.repository.GetPermissionAssociation(id)

	if err != nil {
		return nil, err
	}

	return user, nil

}

func (p permissionAssociationService) GetPermissionAssociations() ([]dto.PermissionAssociationOutput, error) {

	permissionAssociations, err := p.repository.GetAllPermissionAssociations()

	if err != nil {
		return nil, err
	}

	var permissionAssociationsDto []dto.PermissionAssociationOutput

	for _, s := range *permissionAssociations {
		permissionAssociationsDto = append(permissionAssociationsDto, *dto.PermissionAssociationModelToPermissionAssociationOutput(s))
	}

	return permissionAssociationsDto, nil
}

func (p permissionAssociationService) CreatePermissionAssociation(permissionAssociationCreateDTO dto.PermissionAssociationInput) (*dto.PermissionAssociationOutput, error) {

	permissionAssociation, createPermissionAssociationError := p.repository.CreatePermissionAssociation(permissionAssociationCreateDTO)

	if createPermissionAssociationError != nil {
		return nil, createPermissionAssociationError
	}

	var permissionAssociationObjects []dto.PermissionAssociationObjectOutput

	for _, permissionAssociationObject := range permissionAssociation.PermissionAssociationObjects {
		permissionAssociationObjects = append(permissionAssociationObjects, *dto.PermissionAssociationObjectModelToPermissionAssociationObjectOutput(permissionAssociationObject))
	}

	return &dto.PermissionAssociationOutput{
		ID:                           permissionAssociation.ID,
		Permission:                   *dto.PermissionModelToPermissionOutput(permissionAssociation.Permission),
		PermissionAssociationObjects: permissionAssociationObjects,
	}, nil

}
