package services

import (
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/models"
	"soli/formations/src/auth/repositories"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type GroupService interface {
	GetGroup(id uuid.UUID) (*models.Group, error)
	GetGroups() ([]dto.GroupOutput, error)
	CreateGroup(groupCreateDTO dto.CreateGroupInput) (*dto.GroupOutput, error)
	EditGroup(editedGroupInput *dto.GroupEditInput, id uuid.UUID) (*dto.GroupEditOutput, error)
	DeleteGroup(id uuid.UUID) error
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

func (g groupService) DeleteGroup(id uuid.UUID) error {
	errorDelete := g.repository.DeleteGroup(id)
	if errorDelete != nil {
		return errorDelete
	}
	return nil
}

func (g groupService) GetGroup(id uuid.UUID) (*models.Group, error) {
	user, err := g.repository.GetGroup(id)

	if err != nil {
		return nil, err
	}

	return user, nil

}

func (g groupService) GetGroups() ([]dto.GroupOutput, error) {

	userModel, err := g.repository.GetAllGroups()

	if err != nil {
		return nil, err
	}

	var usersDto []dto.GroupOutput

	for _, s := range *userModel {
		usersDto = append(usersDto, *dto.GroupModelToGroupOutput(s))
	}

	return usersDto, nil
}

func (g groupService) CreateGroup(groupCreateDTO dto.CreateGroupInput) (*dto.GroupOutput, error) {

	group, createGroupError := g.repository.CreateGroup(groupCreateDTO)

	if createGroupError != nil {
		return nil, createGroupError
	}

	return &dto.GroupOutput{
		ID:        group.ID,
		GroupName: group.GroupName,
	}, nil

}
