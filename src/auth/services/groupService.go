package services

import (
	"fmt"
	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/dto"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
)

type GroupService interface {
	AddGroup(userCreateDTO dto.CreateGroupInput) (*dto.CreateGroupOutput, error)
	AddUserInGroup(addUserInGroupDTO dto.AddUserInGroupInput) (string, error)
	DeleteGroup(id string) error
}

type groupService struct {
}

func NewGroupService() GroupService {
	return &groupService{}
}

func (us *groupService) AddGroup(groupCreateDTO dto.CreateGroupInput) (*dto.CreateGroupOutput, error) {
	group1 := casdoorsdk.Group{
		Name:        groupCreateDTO.Name,
		DisplayName: groupCreateDTO.DisplayName,
		Owner:       "sdv",
		ParentId:    groupCreateDTO.ParentGroup,
	}

	group1.CreatedTime = casdoorsdk.GetCurrentTime()
	_, errCreate := casdoorsdk.AddGroup(&group1)
	if errCreate != nil {
		fmt.Println(errCreate.Error())
		return nil, errCreate
	}

	createdGroup, errGet := casdoorsdk.GetGroup(group1.Name)
	if errGet != nil {
		fmt.Println(errGet.Error())
		return nil, errGet
	}

	// _, errStudent := casdoor.Enforcer.AddGroupingPolicy(createdGroup.Id, groupCreateDTO.DefaultRole)
	// if errStudent != nil {
	// 	fmt.Println(errStudent.Error())
	// 	return nil, errStudent
	// }

	return dto.GroupModelToGroupOutput(createdGroup), nil
}

func (us *groupService) AddUserInGroup(addUserInGroupDTO dto.AddUserInGroupInput) (string, error) {
	group, errGroup := casdoorsdk.GetGroup(addUserInGroupDTO.GroupName)
	if errGroup != nil {
		return "", errGroup
	}

	user, errUser := casdoorsdk.GetUserByUserId(addUserInGroupDTO.UserId)
	if errUser != nil {
		return "", errUser
	}

	user.Groups = append(user.Groups, group.Name)
	group.Users = append(group.Users, user)

	_, errUpdateGroup := casdoorsdk.UpdateGroup(group)
	if errUpdateGroup != nil {
		return "", errUpdateGroup
	}

	_, errUpdateUser := casdoorsdk.UpdateUser(user)
	if errUpdateUser != nil {
		return "", errUpdateUser
	}

	return "User added in group", nil
}

func (us *groupService) DeleteGroup(id string) error {
	user, errUser := casdoorsdk.GetUserByUserId(id)
	if errUser != nil {
		fmt.Println(errUser.Error())
		return errUser
	}
	casdoorsdk.DeleteUser(user)

	casdoor.Enforcer.RemoveGroupingPolicy(user.Id)

	return nil
}
