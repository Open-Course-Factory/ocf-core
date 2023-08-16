package repositories

import (
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type RoleRepository interface {
	CreateRole(roledto dto.CreateRoleInput) (*models.Role, error)
	EditRole(id uuid.UUID, roleinfos dto.RoleEditInput) (*dto.RoleEditOutput, error)
	GetRoleByUser(user uuid.UUID) (*[]models.UserRole, error)
	CreateUserRoleObjectAssociation(userId uuid.UUID, roleId uuid.UUID, objectId uuid.UUID, objectType string) (*dto.UserRoleObjectAssociationOutput, error)
}

type roleRepository struct {
	GenericRepository
	db *gorm.DB
}

func NewRoleRepository(db *gorm.DB) RoleRepository {
	repository := &roleRepository{
		GenericRepository: NewGenericRepository(db),
		db:                db,
	}
	return repository
}

func (r roleRepository) CreateRole(roledto dto.CreateRoleInput) (*models.Role, error) {

	role := models.Role{
		RoleName: roledto.RoleName,
	}

	result := r.db.Create(&role)
	if result.Error != nil {
		return nil, result.Error
	}
	return &role, nil
}

func (r roleRepository) EditRole(id uuid.UUID, roleinfos dto.RoleEditInput) (*dto.RoleEditOutput, error) {

	role := models.Role{
		RoleName: roleinfos.RoleName,
	}

	result := r.db.Model(&models.Role{}).Where("id = ?", id).Updates(role)

	if result.Error != nil {
		return nil, result.Error
	}

	return &dto.RoleEditOutput{
		RoleName: role.RoleName,
	}, nil
}

func (r roleRepository) GetRoleByUser(user uuid.UUID) (*[]models.UserRole, error) {

	var userRoleObjectAssociation []models.UserRole
	result := r.db.Where("user_id = ?", user).Preload("Role").Preload("User").Find(&userRoleObjectAssociation)
	if result.Error != nil {
		return nil, result.Error
	}
	return &userRoleObjectAssociation, nil
}

func (r roleRepository) CreateUserRoleObjectAssociation(userId uuid.UUID, roleId uuid.UUID, objectId uuid.UUID, objectType string) (*dto.UserRoleObjectAssociationOutput, error) {

	userRoleObjectAssociation := models.UserRole{
		UserID:      &userId,
		RoleID:      &roleId,
		SubObjectID: &objectId,
		SubType:     objectType,
	}

	if userId != uuid.Nil {
		user, errUser := r.GetEntity(userId, models.User{})
		if errUser != nil {
			return nil, errUser
		}
		userRoleObjectAssociation.User = user.(*models.User)
	}

	if roleId != uuid.Nil {
		role, errRole := r.GetEntity(roleId, models.Role{})
		if errRole != nil {
			return nil, errRole
		}
		userRoleObjectAssociation.Role = role.(*models.Role)
	}

	result := r.db.Create(&userRoleObjectAssociation)
	if result.Error != nil {
		return nil, result.Error
	}

	return &dto.UserRoleObjectAssociationOutput{
		UserID:      *userRoleObjectAssociation.UserID,
		RoleID:      *userRoleObjectAssociation.RoleID,
		SubObjectID: *userRoleObjectAssociation.SubObjectID,
		SubType:     userRoleObjectAssociation.SubType,
	}, nil
}
