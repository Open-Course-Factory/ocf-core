package repositories

import (
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type PermissionRepository interface {
	CreatePermission(groupdto dto.CreatePermissionInput) (*models.Permission, error)
	GetPermission(id uuid.UUID) (*models.Permission, error)
	GetAllPermissions() (*[]models.Permission, error)
	DeletePermission(id uuid.UUID) error
	EditPermission(id uuid.UUID, groupinfos dto.PermissionEditInput) (*dto.PermissionEditOutput, error)
	GetPermissionsByUser(id uuid.UUID) (*[]models.Permission, error)
}

type permissionRepository struct {
	db *gorm.DB
}

func NewPermissionRepository(db *gorm.DB) PermissionRepository {
	repository := &permissionRepository{
		db: db,
	}
	return repository
}

func (p permissionRepository) CreatePermission(permissiondto dto.CreatePermissionInput) (*models.Permission, error) {

	g := NewGroupRepository(p.db)
	o := NewOrganisationRepository(p.db)

	genericRepository := NewGenericRepository(p.db)

	user, err := genericRepository.GetEntity(permissiondto.User, models.User{})
	if err != nil {
		return nil, err
	}

	role, err := genericRepository.GetEntity(permissiondto.Role, models.Role{})
	if err != nil {
		return nil, err
	}

	group, err := g.GetGroup(permissiondto.Group)
	if err != nil {
		return nil, err
	}

	organisation, err := o.GetOrganisation(permissiondto.Organisation)
	if err != nil {
		return nil, err
	}

	permission := models.Permission{
		User:            user.(*models.User),
		Role:            role.(*models.Role),
		Group:           group,
		Organisation:    organisation,
		PermissionTypes: permissiondto.PermissionTypes,
	}

	result := p.db.Create(&permission)
	if result.Error != nil {
		return nil, result.Error
	}
	return &permission, nil
}

func (p permissionRepository) GetAllPermissions() (*[]models.Permission, error) {

	var permission []models.Permission
	result := p.db.Find(&permission)
	if result.Error != nil {
		return nil, result.Error
	}
	return &permission, nil
}

func (p permissionRepository) GetPermission(id uuid.UUID) (*models.Permission, error) {

	var permission models.Permission
	result := p.db.First(&permission, id)

	if result.Error != nil {
		return nil, result.Error
	}

	return &permission, nil
}

func (p permissionRepository) DeletePermission(id uuid.UUID) error {
	result := p.db.Delete(&models.Permission{}, id)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (p permissionRepository) EditPermission(id uuid.UUID, permissioninfos dto.PermissionEditInput) (*dto.PermissionEditOutput, error) {

	g := NewGroupRepository(p.db)
	o := NewOrganisationRepository(p.db)

	genericRepository := NewGenericRepository(p.db)

	user, err := genericRepository.GetEntity(permissioninfos.User, models.User{})
	if err != nil {
		return nil, err
	}

	role, err := genericRepository.GetEntity(permissioninfos.Role, models.Role{})
	if err != nil {
		return nil, err
	}

	group, err := g.GetGroup(permissioninfos.Group)
	if err != nil {
		return nil, err
	}

	organisation, err := o.GetOrganisation(permissioninfos.Organisation)
	if err != nil {
		return nil, err
	}

	permission, err := p.GetPermission(id)
	if err != nil {
		return nil, err
	}

	permission.User = user.(*models.User)
	permission.Role = role.(*models.Role)
	permission.Group = group
	permission.Organisation = organisation
	permission.PermissionTypes = permissioninfos.PermissionTypes

	result := p.db.Model(&models.Permission{}).Where("id = ?", id).Updates(permission)

	if result.Error != nil {
		return nil, result.Error
	}

	return &dto.PermissionEditOutput{
		User:            user.(*models.User).ID,
		Role:            role.(*models.Role).ID,
		Group:           group.ID,
		Organisation:    organisation.ID,
		PermissionTypes: permission.PermissionTypes,
	}, nil
}

func (p permissionRepository) GetPermissionsByUser(user uuid.UUID) (*[]models.Permission, error) {

	var permission []models.Permission
	result := p.db.Where("user_id = ?", user).Find(&permission)
	if result.Error != nil {
		return nil, result.Error
	}
	return &permission, nil
}
