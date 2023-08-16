package repositories

import (
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type UserRoleAssociationObjectRepository interface {
	CreateUserRoleAssociationObject(rolePermissionAssociationObjectDto *dto.UserRoleObjectAssociationInput) (*models.UserRole, error)
	GetUserRoleAssociationObject(subObjectId *uuid.UUID, subObjectType string) (*models.UserRole, error)
}

type permissionAssociationObjectRepository struct {
	GenericRepository
	db *gorm.DB
}

func NewRolePermissionAssociationObjectRepository(db *gorm.DB) UserRoleAssociationObjectRepository {
	repository := &permissionAssociationObjectRepository{
		GenericRepository: NewGenericRepository(db),
		db:                db,
	}
	return repository
}

func (p permissionAssociationObjectRepository) CreateUserRoleAssociationObject(permissionAssociationObject *dto.UserRoleObjectAssociationInput) (*models.UserRole, error) {

	rolePermissionAssociationObject := models.UserRole{
		SubType:     permissionAssociationObject.SubType,
		SubObjectID: &permissionAssociationObject.SubObjectID,
	}

	if permissionAssociationObject.RoleId != uuid.Nil {
		role, errRole := p.GetEntity(permissionAssociationObject.RoleId, models.Role{})
		if errRole != nil {
			return nil, errRole
		}

		rolePermissionAssociationObject.Role = role.(*models.Role)
	}

	if permissionAssociationObject.UserId != uuid.Nil {
		user, errUser := p.GetEntity(permissionAssociationObject.RoleId, models.User{})
		if errUser != nil {
			return nil, errUser
		}

		rolePermissionAssociationObject.User = user.(*models.User)
	}

	result := p.db.Create(&rolePermissionAssociationObject)

	if result.Error != nil {
		return nil, result.Error
	}

	return &rolePermissionAssociationObject, nil
}

func (p permissionAssociationObjectRepository) GetUserRoleAssociationObject(subObjectId *uuid.UUID, subObjectType string) (*models.UserRole, error) {

	var permissionAssociationObject models.UserRole
	result := p.db.Where(&models.UserRole{SubObjectID: subObjectId, SubType: subObjectType}).First(&permissionAssociationObject)

	if result.Error != nil {
		return nil, result.Error
	}

	return &permissionAssociationObject, nil
}
