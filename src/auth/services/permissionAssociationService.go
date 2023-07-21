package services

import (
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/repositories"

	"gorm.io/gorm"
)

type PermissionAssociationService interface {
	CreatePermissionAssociation(permissionAssociationCreateDTO dto.PermissionAssociationInput) (*dto.PermissionAssociationOutput, error)
	//EditPermissionAssociation(editedPermissionAssociationInput *dto.PermissionAssociationInput, id uuid.UUID) (*dto.PermissionAssociationOutput, error)
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
