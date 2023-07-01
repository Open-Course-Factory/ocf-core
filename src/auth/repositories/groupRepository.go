package repositories

import (
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type GroupRepository interface {
	CreateGroup(groupdto dto.CreateGroupInput) (*models.Group, error)
	GetGroup(id uuid.UUID) (*models.Group, error)
	GetAllGroups() (*[]models.Group, error)
	DeleteGroup(id uuid.UUID) error
	EditGroup(id uuid.UUID, groupinfos dto.GroupEditInput) (*dto.GroupEditOutput, error)
}

type groupRepository struct {
	db *gorm.DB
}

func NewGroupRepository(db *gorm.DB) GroupRepository {
	repository := &groupRepository{
		db: db,
	}
	return repository
}

func (g groupRepository) CreateGroup(groupdto dto.CreateGroupInput) (*models.Group, error) {
	var parentGroup *models.Group
	var err error
	if groupdto.ParentGroup != uuid.Nil {
		parentGroup, err = g.GetGroup(groupdto.ParentGroup)
		if err != nil {
			return nil, err
		}
	}

	group := models.Group{
		GroupName:   groupdto.GroupName,
		ParentGroup: parentGroup,
	}

	result := g.db.Create(&group)
	if result.Error != nil {
		return nil, result.Error
	}
	return &group, nil
}

func (g groupRepository) GetAllGroups() (*[]models.Group, error) {

	var group []models.Group
	result := g.db.Find(&group)
	if result.Error != nil {
		return nil, result.Error
	}
	return &group, nil
}

func (g groupRepository) GetGroup(id uuid.UUID) (*models.Group, error) {

	var group models.Group
	result := g.db.First(&group, id)

	if result.Error != nil {
		return nil, result.Error
	}

	return &group, nil
}

func (g groupRepository) DeleteGroup(id uuid.UUID) error {
	result := g.db.Delete(&models.Group{}, id)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (g groupRepository) EditGroup(id uuid.UUID, groupinfos dto.GroupEditInput) (*dto.GroupEditOutput, error) {

	parentGroup, err := g.GetGroup(groupinfos.ParentGroup)
	if err != nil {
		return nil, err
	}

	group := models.Group{
		GroupName:   groupinfos.GroupName,
		ParentGroup: parentGroup,
	}

	result := g.db.Model(&models.Group{}).Where("id = ?", id).Updates(group)

	if result.Error != nil {
		return nil, result.Error
	}

	return &dto.GroupEditOutput{
		GroupName: group.GroupName,
	}, nil
}
