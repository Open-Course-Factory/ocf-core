package repositories

import (
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type GroupRepository interface {
	CreateGroup(groupdto dto.CreateGroupInput) (*models.Group, error)
	EditGroup(id uuid.UUID, groupinfos dto.GroupEditInput) (*dto.GroupEditOutput, error)
}

type groupRepository struct {
	GenericRepository
	db *gorm.DB
}

func NewGroupRepository(db *gorm.DB) GroupRepository {
	repository := &groupRepository{
		GenericRepository: NewGenericRepository(db),
		db:                db,
	}
	return repository
}

func (g groupRepository) CreateGroup(groupdto dto.CreateGroupInput) (*models.Group, error) {
	var parentGroup interface{}
	var organisation interface{}
	var errGrp, errOrg error

	if groupdto.ParentGroup != uuid.Nil {
		parentGroup, errGrp = g.GetEntity(groupdto.ParentGroup, models.Group{})
		if errGrp != nil {
			return nil, errGrp
		}
	}

	if groupdto.Organisation != uuid.Nil {
		organisation, errOrg = g.GetEntity(groupdto.Organisation, models.Organisation{})
		if errOrg != nil {
			return nil, errOrg
		}
	}

	group := models.Group{
		GroupName: groupdto.GroupName,
	}

	if parentGroup != nil {
		group.ParentGroupID = &parentGroup.(*models.Group).ID
	}

	if organisation != nil {
		group.OrganisationID = &organisation.(*models.Organisation).ID
	}

	result := g.db.Create(&group)
	if result.Error != nil {
		return nil, result.Error
	}
	return &group, nil
}

func (g groupRepository) EditGroup(id uuid.UUID, groupinfos dto.GroupEditInput) (*dto.GroupEditOutput, error) {

	parentGroup, err := g.GetEntity(id, models.Group{})
	if err != nil {
		return nil, err
	}

	organisation, errOrg := g.GetEntity(groupinfos.Organisation, models.Organisation{})
	if errOrg != nil {
		if errOrg.Error() != "record not found" {
			return nil, errOrg
		}

	}

	group := models.Group{
		GroupName:    groupinfos.GroupName,
		ParentGroup:  parentGroup.(*models.Group),
		Organisation: organisation.(*models.Organisation),
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
