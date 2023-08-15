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

	user, err := p.GetEntity(permissiondto.User, models.User{})
	if err != nil {
		return nil, err
	}

	role, err := p.GetEntity(permissiondto.Role, models.Role{})
	if err != nil {
		return nil, err
	}

	permission := models.Permission{
		User:            user.(*models.User),
		Role:            role.(*models.Role),
		PermissionTypes: permissiondto.PermissionTypes,
	}

	if permissiondto.Group != uuid.Nil {
		group, err := p.GetEntity(permissiondto.Group, models.Group{})
		if err != nil {
			return nil, err
		}
		permission.Group = group.(*models.Group)
	}

	if permissiondto.Organisation != uuid.Nil {
		organisation, err := p.GetEntity(permissiondto.Organisation, models.Organisation{})
		if err != nil {
			return nil, err
		}
		permission.Organisation = organisation.(*models.Organisation)
	}

	result := p.db.Create(&permission)
	if result.Error != nil {
		return nil, result.Error
	}
	return &permission, nil
}

func (p permissionRepository) EditPermission(id uuid.UUID, permissioninfos dto.PermissionEditInput) (*dto.PermissionEditOutput, error) {

	user, err := p.GetEntity(permissioninfos.User, models.User{})
	if err != nil {
		return nil, err
	}

	role, err := p.GetEntity(permissioninfos.Role, models.Role{})
	if err != nil {
		return nil, err
	}

	group, err := p.GetEntity(permissioninfos.Group, models.Group{})
	if err != nil {
		return nil, err
	}

	organisation, err := p.GetEntity(permissioninfos.Organisation, models.Organisation{})
	if err != nil {
		return nil, err
	}

	permission, err := p.GetEntity(id, models.Permission{})
	if err != nil {
		return nil, err
	}

	permissionEntity := permission.(*models.Permission)

	permissionEntity.User = user.(*models.User)
	permissionEntity.Role = role.(*models.Role)
	permissionEntity.Group = group.(*models.Group)
	permissionEntity.Organisation = organisation.(*models.Organisation)
	permissionEntity.PermissionTypes = permissioninfos.PermissionTypes

	result := p.db.Model(&models.Permission{}).Where("id = ?", id).Updates(permission)

	if result.Error != nil {
		return nil, result.Error
	}

	return &dto.PermissionEditOutput{
		User:            user.(*models.User).ID,
		Role:            role.(*models.Role).ID,
		Group:           group.(*models.Group).ID,
		Organisation:    organisation.(*models.Organisation).ID,
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
