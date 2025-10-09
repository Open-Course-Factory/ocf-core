package services

import (
	"fmt"
	"strings"

	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/models"
	sqldb "soli/formations/src/db"
	ttServices "soli/formations/src/terminalTrainer/services"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/docker/docker/pkg/namesgenerator"
	"gorm.io/gorm"
)

type UserService interface {
	AddUser(userCreateDTO dto.CreateUserInput) (*dto.UserOutput, error)
	GetUserById(id string) (*dto.UserOutput, error)
	GetAllUsers() (*[]dto.UserOutput, error)
	GetUsersByIds(ids []string) (*[]dto.UserOutput, error)
	SearchUsers(query string) (*[]dto.UserOutput, error)
	DeleteUser(id string) error
}

type userService struct {
}

func NewUserService() UserService {
	return &userService{}
}

func (us *userService) AddUser(userCreateDTO dto.CreateUserInput) (*dto.UserOutput, error) {
	generatedUsername := namesgenerator.GetRandomName(1)

	user1, err := createUserIntoCasdoor(generatedUsername, userCreateDTO)
	if err != nil {
		return nil, err
	}

	errRole := addDefaultRoleToUser(user1)
	if errRole != nil {
		return nil, err
	}

	createdUser, errGet := casdoorsdk.GetUserByEmail(userCreateDTO.Email)
	if errGet != nil {
		fmt.Println(errGet.Error())
		return nil, errGet
	}

	// Add both the requested role AND ensure "student" role is added for basic permissions
	// This ensures compatibility with both Casdoor role names and OCF role names
	rolesToAdd := []string{}
	if userCreateDTO.DefaultRole != "" {
		rolesToAdd = append(rolesToAdd, userCreateDTO.DefaultRole)
	}

	// Add "member" and "student" for Casdoor compatibility
	rolesToAdd = append(rolesToAdd, "student")
	rolesToAdd = append(rolesToAdd, "member")

	// Add all roles to the user
	for _, role := range rolesToAdd {
		_, errAddRole := casdoor.Enforcer.AddGroupingPolicy(createdUser.Id, role)
		if errAddRole != nil {
			fmt.Printf("Warning: Could not add role %s to user %s: %v\n", role, createdUser.Id, errAddRole)
			// Continue adding other roles even if one fails
		} else {
			fmt.Printf("✅ Successfully added role '%s' to user %s\n", role, createdUser.Id)
		}
	}

	terminalService := ttServices.NewTerminalTrainerService(sqldb.DB)
	errCreateUserKey := terminalService.CreateUserKey(createdUser.Id, createdUser.Name)
	if errCreateUserKey != nil {
		fmt.Printf("Warning: Could not create terminal trainer key for user %s: %v\n", createdUser.Id, errCreateUserKey)
		// On ne fait pas échouer l'inscription pour ça, juste un warning
	}

	// Create default user settings
	errCreateSettings := createDefaultUserSettings(createdUser.Id)
	if errCreateSettings != nil {
		fmt.Printf("Warning: Could not create default settings for user %s: %v\n", createdUser.Id, errCreateSettings)
		// Don't fail registration for this, just log warning
	}

	return dto.UserModelToUserOutput(createdUser), nil
}

func NewTerminalTrainerService(db *gorm.DB) ttServices.TerminalTrainerService {
	return ttServices.NewTerminalTrainerService(db)
}

func addDefaultRoleToUser(user1 casdoorsdk.User) error {
	role, errRole := casdoorsdk.GetRole("student")
	if errRole != nil {
		fmt.Println(errRole.Error())
		return errRole
	}

	role.Users = append(role.Users, user1.GetId())

	_, errUpdateRole := casdoorsdk.UpdateRole(role)
	if errUpdateRole != nil {
		fmt.Println(errUpdateRole.Error())
		return errUpdateRole
	}
	return nil
}

func createUserIntoCasdoor(generatedUsername string, userCreateDTO dto.CreateUserInput) (casdoorsdk.User, error) {
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
		return casdoorsdk.User{}, errCreate
	}
	return user1, nil
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

// GetUsersByIds retrieves multiple users by their IDs
func (us *userService) GetUsersByIds(ids []string) (*[]dto.UserOutput, error) {
	var results []dto.UserOutput

	for _, id := range ids {
		user, errUser := casdoorsdk.GetUserByUserId(id)
		if errUser != nil {
			// Skip users that don't exist or can't be accessed
			continue
		}
		results = append(results, *dto.UserModelToUserOutput(user))
	}

	return &results, nil
}

// SearchUsers searches for users by name or email
func (us *userService) SearchUsers(query string) (*[]dto.UserOutput, error) {
	if len(strings.TrimSpace(query)) < 2 {
		return &[]dto.UserOutput{}, fmt.Errorf("search query must be at least 2 characters")
	}

	// Get all users since Casdoor SDK doesn't have built-in search
	users, errUser := casdoorsdk.GetUsers()
	if errUser != nil {
		fmt.Println(errUser.Error())
		return nil, errUser
	}

	var results []dto.UserOutput
	queryLower := strings.ToLower(strings.TrimSpace(query))

	for _, user := range users {
		// Case-insensitive search on name and email
		if strings.Contains(strings.ToLower(user.Name), queryLower) ||
			strings.Contains(strings.ToLower(user.Email), queryLower) {
			results = append(results, *dto.UserModelToUserOutput(user))
		}

		// Limit results to 20 users max for performance
		if len(results) >= 20 {
			break
		}
	}

	return &results, nil
}

// createDefaultUserSettings creates default settings for a new user
func createDefaultUserSettings(userID string) error {
	// Check if settings already exist
	var existingSettings models.UserSettings
	result := sqldb.DB.Where("user_id = ?", userID).First(&existingSettings)
	if result.Error == nil {
		return nil // Settings already exist
	}

	// Create default settings
	defaultSettings := models.UserSettings{
		UserID:               userID,
		DefaultLandingPage:   "/dashboard",
		PreferredLanguage:    "en",
		Timezone:             "UTC",
		Theme:                "light",
		CompactMode:          false,
		EmailNotifications:   true,
		DesktopNotifications: false,
		TwoFactorEnabled:     false,
	}

	return sqldb.DB.Create(&defaultSettings).Error
}
