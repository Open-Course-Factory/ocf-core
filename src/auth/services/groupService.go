package services

import (
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/repositories"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type GroupService interface {
	CreateGroup(groupCreateDTO dto.CreateGroupInput) (*dto.GroupOutput, error)
	EditGroup(editedGroupInput *dto.GroupEditInput, id uuid.UUID) (*dto.GroupEditOutput, error)
}

type groupService struct {
	repository repositories.GroupRepository
}

func NewGroupService(db *gorm.DB) GroupService {
	return &groupService{
		repository: repositories.NewGroupRepository(db),
	}
}

func (g groupService) EditGroup(editedGroupInput *dto.GroupEditInput, id uuid.UUID) (*dto.GroupEditOutput, error) {

	editGroup := editedGroupInput

	editedGroup, userError := g.repository.EditGroup(id, *editGroup)

	if userError != nil {
		return nil, userError
	}

	return editedGroup, nil
}

func (g groupService) CreateGroup(groupCreateDTO dto.CreateGroupInput) (*dto.GroupOutput, error) {

	group, createGroupError := g.repository.CreateGroup(groupCreateDTO)

	if createGroupError != nil {
		return nil, createGroupError
	}

	groupOutput := dto.GroupOutput{
		ID:        group.ID,
		GroupName: group.GroupName,
	}

	if group.OrganisationID != nil {
		groupOutput.Organisation = *group.OrganisationID
	}

	if group.ParentGroupID != nil {
		groupOutput.ParentGroup = *group.ParentGroupID
	}

	return &groupOutput, nil

}
