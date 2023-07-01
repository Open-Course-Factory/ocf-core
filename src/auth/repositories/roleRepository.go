package repositories

import (
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type RoleRepository interface {
	CreateRole(roledto dto.CreateRoleInput) (*models.Role, error)
	GetAllRoles() (*[]models.Role, error)
	GetRole(id uuid.UUID) (*models.Role, error)
	DeleteRole(id uuid.UUID) error
	EditRole(id uuid.UUID, roleinfos dto.RoleEditInput) (*dto.RoleEditOutput, error)
}

type roleRepository struct {
	db *gorm.DB
}

func NewRoleRepository(db *gorm.DB) RoleRepository {
	repository := &roleRepository{
		db: db,
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

func (r roleRepository) GetAllRoles() (*[]models.Role, error) {

	var role []models.Role
	result := r.db.Find(&role)
	if result.Error != nil {
		return nil, result.Error
	}
	return &role, nil
}

func (r roleRepository) GetRole(id uuid.UUID) (*models.Role, error) {

	var role models.Role
	result := r.db.First(&role, id)

	if result.Error != nil {
		return nil, result.Error
	}

	return &role, nil
}

func (r roleRepository) DeleteRole(id uuid.UUID) error {
	result := r.db.Delete(&models.Role{}, id)
	if result.Error != nil {
		return result.Error
	}
	return nil
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
