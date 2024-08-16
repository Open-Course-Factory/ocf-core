package services

import (
	"fmt"
	"reflect"
	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/models"
	sqldb "soli/formations/src/db"
	"strings"

	"soli/formations/src/entityManagement/services"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"

	"github.com/docker/docker/pkg/namesgenerator"
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
begin:
	genericService := services.NewGenericService(
		sqldb.DB,
	)

	generatedUsername := namesgenerator.GetRandomName(1)

	usernameInput := dto.UsernameInput{
		Username: generatedUsername,
	}

	userNameEntity, createError := genericService.CreateEntity(usernameInput, reflect.TypeOf(models.Username{}).Name())
	if createError != nil {
		if strings.Contains(createError.Error(), "UNIQUE") {
			goto begin
		}
		return nil, createError
	}

	properties := make(map[string]string)
	properties["username"] = generatedUsername
	user1 := casdoorsdk.User{
		Name:              userCreateDTO.UserName,
		DisplayName:       userCreateDTO.DisplayName,
		Email:             userCreateDTO.Email,
		Password:          userCreateDTO.Password,
		LastName:          userCreateDTO.LastName,
		FirstName:         userCreateDTO.FirstName,
		SignupApplication: "ocf",
		Properties:        properties,
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

	// Once the user is really created we can set the username ownerId !
	_, errsavingUsername := genericService.AddOwnerIDs(userNameEntity, createdUser.Id)
	if errsavingUsername != nil {
		return nil, errsavingUsername
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

	var results []dto.UserOutput
	for _, user := range users {
		results = append(results, *dto.UserModelToUserOutput(user))
	}

	return &results, nil
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
