package services

import (
	"fmt"
	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/dto"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
)

type UserService interface {
	AddUser(userCreateDTO dto.CreateUserInput) (*dto.UserOutput, error)
	GetUserById(id string) (*dto.UserOutput, error)
	GetAllUsers() (*[]dto.UserOutput, error)
	DeleteUser(id string) error
}

type userService struct {
}

func NewUserService() UserService {
	return &userService{}
}

func (us *userService) AddUser(userCreateDTO dto.CreateUserInput) (*dto.UserOutput, error) {
	user1 := casdoorsdk.User{
		Name:              userCreateDTO.UserName,
		DisplayName:       userCreateDTO.DisplayName,
		Email:             userCreateDTO.Email,
		Password:          userCreateDTO.Password,
		LastName:          userCreateDTO.LastName,
		FirstName:         userCreateDTO.FirstName,
		SignupApplication: "ocf",
	}

	user1.CreatedTime = casdoorsdk.GetCurrentTime()
	_, errCreate := casdoorsdk.AddUser(&user1)
	if errCreate != nil {
		fmt.Println(errCreate.Error())
		return nil, errCreate
	}

	createdUser, errGet := casdoorsdk.GetUserByEmail(userCreateDTO.Email)
	if errGet != nil {
		fmt.Println(errGet.Error())
		return nil, errGet
	}

	_, errStudent := casdoor.Enforcer.AddGroupingPolicy(createdUser.Id, userCreateDTO.DefaultRole)
	if errStudent != nil {
		fmt.Println(errStudent.Error())
		return nil, errStudent
	}

	return dto.UserModelToUserOutput(createdUser), nil
}

func (us *userService) GetUserById(id string) (*dto.UserOutput, error) {
	user, errUser := casdoorsdk.GetUserByUserId(id)
	if errUser != nil {
		fmt.Println(errUser.Error())
		return nil, errUser
	}

	return dto.UserModelToUserOutput(user), nil
}

func (us *userService) GetAllUsers() (*[]dto.UserOutput, error) {
	users, errUser := casdoorsdk.GetUsers()
	if errUser != nil {
		fmt.Println(errUser.Error())
		return nil, errUser
	}

	var results *[]dto.UserOutput
	for _, user := range users {
		*results = append(*results, *dto.UserModelToUserOutput(user))
	}

	return results, nil
}

func (us *userService) DeleteUser(id string) error {
	user, errUser := casdoorsdk.GetUserByUserId(id)
	if errUser != nil {
		fmt.Println(errUser.Error())
		return errUser
	}
	casdoorsdk.DeleteUser(user)

	casdoor.Enforcer.RemoveGroupingPolicy(user.Id)

	return nil
}
