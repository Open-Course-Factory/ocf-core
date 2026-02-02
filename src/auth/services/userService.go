package services

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/models"
	sqldb "soli/formations/src/db"
	organizationServices "soli/formations/src/organizations/services"
	paymentModels "soli/formations/src/payment/models"
	paymentServices "soli/formations/src/payment/services"
	ttServices "soli/formations/src/terminalTrainer/services"
	"soli/formations/src/utils"

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

// validateTosAcceptance validates that Terms of Service have been properly accepted
func validateTosAcceptance(userCreateDTO dto.CreateUserInput) error {
	// Check if ToS fields are present
	if strings.TrimSpace(userCreateDTO.TosAcceptedAt) == "" {
		return errors.New("TOS_ACCEPTANCE_REQUIRED: Terms of Service acceptance timestamp is required for registration")
	}

	if strings.TrimSpace(userCreateDTO.TosVersion) == "" {
		return errors.New("TOS_ACCEPTANCE_REQUIRED: Terms of Service version is required for registration")
	}

	// Validate timestamp format (ISO 8601)
	tosTime, err := time.Parse(time.RFC3339, userCreateDTO.TosAcceptedAt)
	if err != nil {
		return errors.New("INVALID_TOS_TIMESTAMP: Terms of Service acceptance timestamp must be in ISO 8601 format (e.g., 2025-10-11T14:23:45.123Z)")
	}

	// Check that timestamp is not in the future (with 5-minute tolerance for clock skew)
	fiveMinutesFromNow := time.Now().Add(5 * time.Minute)
	if tosTime.After(fiveMinutesFromNow) {
		return errors.New("INVALID_TOS_TIMESTAMP: Terms of Service acceptance timestamp cannot be in the future")
	}

	// Optional: Check that timestamp is recent (within last 24 hours)
	// This prevents replay attacks
	twentyFourHoursAgo := time.Now().Add(-24 * time.Hour)
	if tosTime.Before(twentyFourHoursAgo) {
		return errors.New("INVALID_TOS_TIMESTAMP: Terms of Service acceptance timestamp must be within the last 24 hours")
	}

	// Validate version format (YYYY-MM-DD)
	_, err = time.Parse("2006-01-02", userCreateDTO.TosVersion)
	if err != nil {
		return errors.New("INVALID_TOS_VERSION: Terms of Service version must be in YYYY-MM-DD format")
	}

	return nil
}

func (us *userService) AddUser(userCreateDTO dto.CreateUserInput) (*dto.UserOutput, error) {
	// Validate ToS acceptance
	if err := validateTosAcceptance(userCreateDTO); err != nil {
		return nil, err
	}

	generatedUsername := namesgenerator.GetRandomName(1)

	user1, err := createUserIntoCasdoor(generatedUsername, userCreateDTO)
	if err != nil {
		return nil, err
	}

	errRole := addDefaultRoleToUser(user1)
	if errRole != nil {
		return nil, errRole
	}

	createdUser, errGet := casdoorsdk.GetUserByEmail(userCreateDTO.Email)
	if errGet != nil {
		fmt.Println(errGet.Error())
		return nil, errGet
	}

	// This ensures compatibility with both Casdoor role names and OCF role names
	rolesToAdd := []string{}
	if userCreateDTO.DefaultRole != "" {
		rolesToAdd = append(rolesToAdd, userCreateDTO.DefaultRole)
	}

	rolesToAdd = append(rolesToAdd, "member")

	// Add all roles to the user
	opts := utils.DefaultPermissionOptions()
	opts.WarnOnError = true

	for _, role := range rolesToAdd {
		errAddRole := utils.AddGroupingPolicy(casdoor.Enforcer, createdUser.Id, role, opts)
		if errAddRole != nil {
			utils.Warn("Could not add role %s to user %s: %v", role, createdUser.Id, errAddRole)
			// Continue adding other roles even if one fails
		} else {
			utils.Info("Successfully added role '%s' to user %s", role, createdUser.Id)
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

	// Assign free Trial plan to new user
	errTrialSubscription := assignFreeTrialPlan(createdUser.Id)
	if errTrialSubscription != nil {
		fmt.Printf("Warning: Could not create Trial subscription for user %s: %v\n", createdUser.Id, errTrialSubscription)
		// Don't fail registration for this, just log warning
	}

	// Create personal organization for new user
	orgService := organizationServices.NewOrganizationService(sqldb.DB)
	_, errPersonalOrg := orgService.CreatePersonalOrganization(createdUser.Id)
	if errPersonalOrg != nil {
		fmt.Printf("Warning: Could not create personal organization for user %s: %v\n", createdUser.Id, errPersonalOrg)
		// Don't fail registration for this, just log warning
	} else {
		fmt.Printf("✅ Successfully created personal organization for user %s\n", createdUser.Id)
	}

	// Send verification email
	verificationService := NewEmailVerificationService(sqldb.DB)
	err = verificationService.CreateVerificationToken(createdUser.Id, createdUser.Email)
	if err != nil {
		// Log but don't fail registration
		fmt.Printf("Warning: Could not send verification email to %s: %v\n", createdUser.Email, err)
	}

	return dto.UserModelToUserOutput(createdUser), nil
}

func NewTerminalTrainerService(db *gorm.DB) ttServices.TerminalTrainerService {
	return ttServices.NewTerminalTrainerService(db)
}

func addDefaultRoleToUser(user1 casdoorsdk.User) error {
	role, errRole := casdoorsdk.GetRole("member")
	if errRole != nil {
		fmt.Println(errRole.Error())
		return errRole
	}

	// Check if role exists (SDK may return nil without error)
	if role == nil {
		return fmt.Errorf("role 'member' not found in Casdoor - please ensure the role exists")
	}

	// Initialize Users slice if nil
	if role.Users == nil {
		role.Users = []string{}
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
	properties["tos_accepted_at"] = userCreateDTO.TosAcceptedAt
	properties["tos_version"] = userCreateDTO.TosVersion
	properties["email_verified"] = "false"

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

	results := make([]dto.UserOutput, 0, len(users))
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

	// Remove all role associations for this user
	opts := utils.DefaultPermissionOptions()
	opts.WarnOnError = true
	utils.RemoveGroupingPolicy(casdoor.Enforcer, user.Id, "", opts)

	return nil
}

// GetUsersByIds retrieves multiple users by their IDs
func (us *userService) GetUsersByIds(ids []string) (*[]dto.UserOutput, error) {
	results := make([]dto.UserOutput, 0, len(ids))

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

	results := make([]dto.UserOutput, 0, len(users)/2)
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
		DefaultLandingPage:   "/terminal-creation",
		PreferredLanguage:    "fr",
		Timezone:             "UTC",
		Theme:                "light",
		CompactMode:          false,
		EmailNotifications:   true,
		DesktopNotifications: false,
		TwoFactorEnabled:     false,
	}

	return sqldb.DB.Create(&defaultSettings).Error
}

// assignFreeTrialPlan assigns the free Trial plan to a new user
func assignFreeTrialPlan(userID string) error {
	// Find the free Trial plan (price_amount = 0, name = "Trial")
	var trialPlan paymentModels.SubscriptionPlan
	result := sqldb.DB.Where("name = ? AND price_amount = 0 AND is_active = true", "Trial").First(&trialPlan)
	if result.Error != nil {
		return fmt.Errorf("could not find active Trial plan: %v", result.Error)
	}

	// Check if user already has a subscription
	var existingSub paymentModels.UserSubscription
	existingResult := sqldb.DB.Where("user_id = ? AND status = ?", userID, "active").First(&existingSub)
	if existingResult.Error == nil {
		fmt.Printf("User %s already has an active subscription, skipping Trial assignment\n", userID)
		return nil // User already has a subscription
	}

	// Create subscription using the subscription service
	subscriptionService := paymentServices.NewSubscriptionService(sqldb.DB)
	_, err := subscriptionService.CreateUserSubscription(userID, trialPlan.ID)
	if err != nil {
		return fmt.Errorf("failed to create Trial subscription: %w", err)
	}

	fmt.Printf("✅ Successfully assigned Trial plan to user %s\n", userID)
	return nil
}
