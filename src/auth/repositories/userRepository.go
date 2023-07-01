package repositories

import (
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/models"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type UserRepository interface {
	CreateUser(userdto dto.CreateUserInput) (*models.User, error)
	GetUser(id uuid.UUID) (*models.User, error)
	GetUserWithEmail(email string) (*models.User, error)
	GetAllUsers() (*[]models.User, error)
	DeleteUser(id uuid.UUID) error
	EditUser(id uuid.UUID, userinfos dto.UserEditInput, isSelf bool) (*dto.UserEditOutput, error)
	EditUserToken(id uuid.UUID, usertoken dto.UserTokens) (*models.User, error)
}

type userRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) UserRepository {
	repository := &userRepository{
		db: db,
	}
	return repository
}

func (u userRepository) CreateUser(userdto dto.CreateUserInput) (*models.User, error) {

	tmp := []byte(userdto.Password)
	passByte, err := bcrypt.GenerateFromPassword(tmp, bcrypt.DefaultCost)
	pwd := string(passByte)
	if err != nil {
		println("Une erreur s'est produite", err.Error())
		return nil, err
	}

	user := models.User{
		Email:     userdto.Email,
		Password:  pwd,
		FirstName: userdto.FirstName,
		LastName:  userdto.LastName,
	}

	result := u.db.Create(&user)
	if result.Error != nil {
		return nil, result.Error
	}
	return &user, err
}

func (u userRepository) GetUser(id uuid.UUID) (*models.User, error) {

	var user models.User
	result := u.db.Preload("SshKeys").First(&user, id)

	if result.Error != nil {
		return nil, result.Error
	}

	return &user, nil
}
func (u userRepository) DeleteUser(id uuid.UUID) error {
	result := u.db.Delete(&models.User{}, id)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (u userRepository) EditUser(id uuid.UUID, userinfos dto.UserEditInput, isSelf bool) (*dto.UserEditOutput, error) {

	var user models.User

	if isSelf {
		user = models.User{
			Password:  userinfos.Password,
			FirstName: userinfos.FirstName,
			LastName:  userinfos.LastName,
		}

	} else {
		user = models.User{
			Password:  userinfos.Password,
			FirstName: userinfos.FirstName,
			LastName:  userinfos.LastName,
		}
	}

	result := u.db.Model(&models.User{}).Where("id = ?", id).Updates(user)

	if result.Error != nil {
		return nil, result.Error
	}

	return &dto.UserEditOutput{
		LastName:  user.LastName,
		FirstName: user.FirstName,
	}, nil
}

func (u userRepository) EditUserToken(id uuid.UUID, usertoken dto.UserTokens) (*models.User, error) {

	user := models.User{
		Token:        usertoken.Token,
		RefreshToken: usertoken.RefreshToken,
	}

	result := u.db.Model(&models.User{}).Where("id = ?", id).Updates(user)

	if result.Error != nil {
		return nil, result.Error
	}

	return &user, nil

}

func (u userRepository) GetAllUsers() (*[]models.User, error) {
	var user []models.User
	result := u.db.Find(&user)
	if result.Error != nil {
		return nil, result.Error
	}
	return &user, nil
}

func (u userRepository) GetUserWithEmail(email string) (*models.User, error) {
	var user models.User
	result := u.db.First(&user, "email=?", email)
	if result.Error != nil {
		return nil, result.Error
	}
	return &user, nil
}
