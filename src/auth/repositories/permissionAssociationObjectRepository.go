package repositories

import (
	"soli/formations/src/auth/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type PermissionAssociationObjectRepository interface {
	CreatePermissionAssociationObject(permissionAssociationObject *models.PermissionAssociationObject) (*models.PermissionAssociationObject, error)
	GetPermissionAssociationObject(subObjectId uuid.UUID, subObjectType string) (*models.PermissionAssociationObject, error)
}

type permissionAssociationObjectRepository struct {
	db *gorm.DB
}

func NewPermissionAssociationObjectRepository(db *gorm.DB) PermissionAssociationObjectRepository {
	repository := &permissionAssociationObjectRepository{
		db: db,
	}
	return repository
}

func (p permissionAssociationObjectRepository) CreatePermissionAssociationObject(permissionAssociationObject *models.PermissionAssociationObject) (*models.PermissionAssociationObject, error) {
	result := p.db.Create(&permissionAssociationObject)

	if result.Error != nil {
		return nil, result.Error
	}

	return permissionAssociationObject, nil
}

func (p permissionAssociationObjectRepository) GetPermissionAssociationObject(subObjectId uuid.UUID, subObjectType string) (*models.PermissionAssociationObject, error) {

	var permissionAssociationObject models.PermissionAssociationObject
	result := p.db.Where(&models.PermissionAssociationObject{SubObjectID: subObjectId, SubType: subObjectType}).First(&permissionAssociationObject)

	if result.Error != nil {
		return nil, result.Error
	}

	return &permissionAssociationObject, nil
}
