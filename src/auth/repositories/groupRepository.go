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
	var organisation *models.Organisation
	var errGrp, errOrg error

	o := NewOrganisationRepository(g.db)

	if groupdto.ParentGroup != uuid.Nil {
		parentGroup, errGrp = g.GetGroup(groupdto.ParentGroup)
		if errGrp != nil {
			return nil, errGrp
		}
	}

	if groupdto.Organisation != uuid.Nil {
		organisation, errOrg = o.GetOrganisation(groupdto.Organisation)
		if errOrg != nil {
			return nil, errOrg
		}
	}

	group := models.Group{
		GroupName:    groupdto.GroupName,
		ParentGroup:  parentGroup,
		Organisation: organisation,
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

	o := NewOrganisationRepository(g.db)

	parentGroup, err := g.GetGroup(groupinfos.ParentGroup)
	if err != nil {
		return nil, err
	}

	organisation, errOrg := o.GetOrganisation(groupinfos.Organisation)
	if errOrg != nil {
		return nil, errOrg
	}

	group := models.Group{
		GroupName:    groupinfos.GroupName,
		ParentGroup:  parentGroup,
		Organisation: organisation,
	}

	result := g.db.Model(&models.Group{}).Where("id = ?", id).Updates(group)

	if result.Error != nil {
		return nil, result.Error
	}

	groupEditOutput := dto.GroupEditOutput{
		GroupName: group.GroupName,
	}

	if group.ParentGroupID != nil {
		groupEditOutput.ParentGroup = *group.ParentGroupID
	}

	if group.OrganisationID != nil {
		groupEditOutput.Organisation = *group.OrganisationID
	}

	return &groupEditOutput, nil
}
