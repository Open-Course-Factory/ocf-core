package services

import (
	"net/mail"
	"time"

	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/models"
	"soli/formations/src/auth/repositories"

	config "soli/formations/src/configuration"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type UserService interface {
	CreateUser(userCreateDTO dto.CreateUserInput, config *config.Configuration) (*dto.UserOutput, error)
	GetUser(id uuid.UUID) (*models.User, error)
	GetUsers() ([]dto.UserOutput, error)
	EditUser(editedUserInput *dto.UserEditInput, id uuid.UUID, isSelf bool) (*dto.UserEditOutput, error)
	DeleteUser(id uuid.UUID) error
	UserLogin(userLogin *dto.UserLoginInput, config *config.Configuration) (*dto.UserTokens, error)
	AddUserSshKey(sshKeyCreateDTO dto.CreateSshKeyInput) (*dto.SshKeyOutput, error)
}

type userService struct {
	repository       repositories.UserRepository
	sshKeyRepository repositories.SshKeyRepository
}

func NewUserService(db *gorm.DB) UserService {
	return &userService{
		repository:       repositories.NewUserRepository(db),
		sshKeyRepository: repositories.NewSshKeyRepository(db),
	}
}

func (u *userService) EditUser(editedUserInput *dto.UserEditInput, id uuid.UUID, isSelf bool) (*dto.UserEditOutput, error) {

	editUser := editedUserInput

	// Edit le mot de passe si il est proposé dans le body
	//TODO: Constant pour la longueur du mort de passe
	if len(editUser.Password) > 1 {
		password, bcryptError := bcrypt.GenerateFromPassword([]byte(editUser.Password), bcrypt.DefaultCost)
		if bcryptError != nil {
			return nil, bcryptError
		}
		editUser.Password = string(password)
	}

	editedUser, userError := u.repository.EditUser(id, *editUser, isSelf)

	if userError != nil {
		return nil, userError
	}

	return editedUser, nil
}

func (u *userService) DeleteUser(id uuid.UUID) error {
	errorDelete := u.repository.DeleteUser(id)
	if errorDelete != nil {
		return errorDelete
	}
	return nil
}

func (u *userService) GetUser(id uuid.UUID) (*models.User, error) {
	user, err := u.repository.GetUser(id)

	if err != nil {
		return nil, err
	}

	return user, nil

}

func (u *userService) GetUsers() ([]dto.UserOutput, error) {

	userModel, err := u.repository.GetAllUsers()

	if err != nil {
		return nil, err
	}

	var usersDto []dto.UserOutput

	for _, s := range *userModel {
		usersDto = append(usersDto, *dto.UserModelToUserOutput(s))
	}

	return usersDto, nil
}

func (u *userService) UserLogin(userLogin *dto.UserLoginInput, config *config.Configuration) (*dto.UserTokens, error) {

	userModel, errorUser := u.repository.GetUserWithEmail(userLogin.Email)
	jwtService := JwtService{}

	if errorUser != nil {
		return nil, errorUser
	}

	// USE PASSWORD SERVICE
	errorCompare := bcrypt.CompareHashAndPassword([]byte(userModel.Password), []byte(userLogin.Password))

	if errorCompare != nil {
		return nil, errorCompare
	}

	newToken, errToken := jwtService.CreateJWT(userModel.ID, time.Now().Add(time.Minute*15), config.SecretJwt)
	newRefreshToken, errRefreshToken := jwtService.CreateJWT(userModel.ID, time.Now().Add(time.Hour*24*7), config.SecretRefreshJwt)

	if errToken != nil || errRefreshToken != nil {
		return nil, errToken
	}

	tokens := dto.UserTokens{Token: newToken, RefreshToken: newRefreshToken}

	_, errUpdate := u.repository.EditUserToken(userModel.ID, tokens)

	if errUpdate != nil {
		return nil, errUpdate
	}

	return &dto.UserTokens{Token: newToken, RefreshToken: newRefreshToken}, nil

}

func (u *userService) CreateUser(userCreateDTO dto.CreateUserInput, config *config.Configuration) (*dto.UserOutput, error) {
	jwtService := JwtService{}

	userTokens := dto.UserTokens{}

	_, parseEmailError := mail.ParseAddress(userCreateDTO.Email)
	if parseEmailError != nil {

		return nil, parseEmailError
	}

	user, createUserError := u.repository.CreateUser(userCreateDTO)

	if createUserError != nil {
		return nil, createUserError
	}

	token, tokenError := jwtService.CreateJWT(
		user.ID,
		time.Now().Add(time.Minute*15),
		config.SecretJwt,
	)
	if tokenError != nil {
		return nil, tokenError
	}

	userTokens.Token = token

	refreshToken, refreshError := jwtService.CreateJWT(
		user.ID,
		time.Now().Add(time.Hour*24*7),
		config.SecretRefreshJwt,
	)
	if refreshError != nil {
		return nil, refreshError
	}

	userTokens.RefreshToken = refreshToken

	userComplete, editTokensError := u.repository.EditUserToken(user.ID, userTokens)

	if editTokensError != nil {
		return nil, editTokensError
	}

	user.Token = userComplete.Token
	user.RefreshToken = userComplete.RefreshToken

	return &dto.UserOutput{
		ID:        user.ID,
		Email:     user.Email,
		FirstName: user.FirstName,
		LastName:  user.LastName,
	}, nil

}

func (u *userService) AddUserSshKey(sshKeyCreateDTO dto.CreateSshKeyInput) (*dto.SshKeyOutput, error) {

	sshKey, creatSshKeyError := u.sshKeyRepository.CreateSshKey(sshKeyCreateDTO)
	if creatSshKeyError != nil {
		return nil, creatSshKeyError
	}

	return &dto.SshKeyOutput{
		Id:         sshKey.ID,
		Name:       sshKey.Name,
		PrivateKey: sshKey.PrivateKey,
		CreatedAt:  sshKey.CreatedAt,
	}, nil
}
