package repositories

import (
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type PermissionRepository interface {
	CreatePermission(groupdto dto.CreatePermissionInput) (*models.Permission, error)
	EditPermission(id uuid.UUID, groupinfos dto.PermissionEditInput) (*dto.PermissionEditOutput, error)
	GetPermissionsByUser(id uuid.UUID) (*[]models.Permission, error)
}

type permissionRepository struct {
	GenericRepository
	db *gorm.DB
}

func NewPermissionRepository(db *gorm.DB) PermissionRepository {
	repository := &permissionRepository{
		GenericRepository: NewGenericRepository(db),
		db:                db,
	}
	return repository
}

func (p permissionRepository) CreatePermission(permissiondto dto.CreatePermissionInput) (*models.Permission, error) {

	permission := models.Permission{
		PermissionTypes: permissiondto.PermissionTypes,
	}

	result := p.db.Create(&permission)
	if result.Error != nil {
		return nil, result.Error
	}
	return &permission, nil
}

func (p permissionRepository) EditPermission(id uuid.UUID, permissioninfos dto.PermissionEditInput) (*dto.PermissionEditOutput, error) {

	permission, err := p.GetEntity(id, models.Permission{})
	if err != nil {
		return nil, err
	}

	permissionEntity := permission.(*models.Permission)

	permissionEntity.PermissionTypes = permissioninfos.PermissionTypes

	result := p.db.Model(&models.Permission{}).Where("id = ?", id).Updates(permission)

	if result.Error != nil {
		return nil, result.Error
	}

	return &dto.PermissionEditOutput{
		PermissionTypes: permissionEntity.PermissionTypes,
	}, nil
}

func (p permissionRepository) GetPermissionsByUser(user uuid.UUID) (*[]models.Permission, error) {

	var permission []models.Permission
	result := p.db.Where("user_id = ?", user).Preload("Role").Find(&permission)
	if result.Error != nil {
		return nil, result.Error
	}
	return &permission, nil
}
