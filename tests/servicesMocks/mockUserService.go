package services

import (
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/models"
	config "soli/formations/src/configuration"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
)

type MockUserService struct {
	mock.Mock
}

func (m *MockUserService) CreateUser(userCreateDTO dto.CreateUserInput, config *config.Configuration) (*dto.UserOutput, error) {
	args := m.Called(userCreateDTO, config)
	return args.Get(0).(*dto.UserOutput), args.Error(1)
}

func (m *MockUserService) GetUser(id uuid.UUID) (*models.User, error) {
	args := m.Called(id)
	return args.Get(0).(*models.User), args.Error(1)
}

func (m *MockUserService) GetUsers() ([]dto.UserOutput, error) {
	args := m.Called()
	return args.Get(0).([]dto.UserOutput), args.Error(1)
}

func (m *MockUserService) EditUser(editedUserInput *dto.UserEditInput, id uuid.UUID, isSelf bool) (*dto.UserEditOutput, error) {
	args := m.Called(editedUserInput, id, isSelf)
	return args.Get(0).(*dto.UserEditOutput), args.Error(1)
}

func (m *MockUserService) DeleteUser(id uuid.UUID) error {
	args := m.Called(id)
	return args.Error(0)
}

func (m *MockUserService) UserLogin(userLogin *dto.UserLoginInput, config *config.Configuration) (*dto.UserTokens, error) {
	args := m.Called(userLogin, config)
	return args.Get(0).(*dto.UserTokens), args.Error(1)
}

func (m *MockUserService) AddUserSshKey(sshKeyCreateDTO dto.CreateSshKeyInput) (*dto.SshKeyOutput, error) {
	args := m.Called(sshKeyCreateDTO)
	return args.Get(0).(*dto.SshKeyOutput), args.Error(1)
}
