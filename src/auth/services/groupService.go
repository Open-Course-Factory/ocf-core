package services

import (
	"fmt"
	"slices"
	"soli/formations/src/auth/dto"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
)

type GroupService interface {
	AddGroup(userCreateDTO dto.CreateGroupInput) (*dto.CreateGroupOutput, error)
	ModifyUsersInGroup(groupName string, addUserInGroupDTO dto.ModifyUsersInGroupInput) (string, error)
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

func (us *groupService) ModifyUsersInGroup(groupName string, modifyUsersInGroupDTO dto.ModifyUsersInGroupInput) (string, error) {
	group, errGroup := casdoorsdk.GetGroup(groupName)
	if errGroup != nil {
		return "", errGroup
	}

	for _, userId := range modifyUsersInGroupDTO.UserIds {
		user, errUser := casdoorsdk.GetUserByUserId(userId)
		if errUser != nil {
			return "", errUser
		}

		switch dto.Action(*modifyUsersInGroupDTO.Action) {
		case dto.ADD:
			if !slices.Contains(user.Groups, group.Name) {
				user.Groups = append(user.Groups, group.Name)
			}

			var userPosition int = -1
			for positionInGroup, userFromGroup := range group.Users {
				if userFromGroup.Id == user.Id {
					userPosition = positionInGroup
					break
				}
			}
			if userPosition < 0 {
				group.Users = append(group.Users, user)
			}
		case dto.REMOVE:

			var userPosition int = -1
			for positionInGroup, userFromGroup := range group.Users {
				if userFromGroup.Id == user.Id {
					userPosition = positionInGroup
					break
				}
			}
			if userPosition >= 0 {
				group.Users = slices.Delete(group.Users, userPosition, userPosition+1)
			}
			groupPosition := slices.Index(user.Groups, group.Name)
			if groupPosition >= 0 {
				user.Groups = slices.Delete(user.Groups, groupPosition, groupPosition+1)
			}
		}

		_, errUpdateGroup := casdoorsdk.UpdateGroup(group)
		if errUpdateGroup != nil {
			return "", errUpdateGroup
		}

		_, errUpdateUser := casdoorsdk.UpdateUser(user)
		if errUpdateUser != nil {
			return "", errUpdateUser
		}
	}

	return "Users modified in group", nil
}

func (us *groupService) DeleteGroup(name string) error {
	group, errGroup := casdoorsdk.GetGroup(name)
	if errGroup != nil {
		fmt.Println(errGroup.Error())
		return errGroup
	}
	casdoorsdk.DeleteGroup(group)

	//casdoor.Enforcer.RemoveGroupingPolicy(user.Id)

	return nil
}
