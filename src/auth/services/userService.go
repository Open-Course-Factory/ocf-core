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
	casdoorClient CasdoorUserClient
	paymentHelper paymentServices.PaymentDeletionHelper
}

// NewUserService builds a UserService with explicit collaborators. Production
// callers wire NewCasdoorUserClient() and paymentServices.NewPaymentDeletionHelper(sqldb.DB);
// tests pass stubs.
func NewUserService(casdoorClient CasdoorUserClient, paymentHelper paymentServices.PaymentDeletionHelper) UserService {
	return &userService{
		casdoorClient: casdoorClient,
		paymentHelper: paymentHelper,
	}
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
		utils.Error("%s", errGet.Error())
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
		utils.Warn("Could not create terminal trainer key for user %s: %v", createdUser.Id, errCreateUserKey)
		// On ne fait pas échouer l'inscription pour ça, juste un warning
	}

	// Create default user settings
	errCreateSettings := createDefaultUserSettings(createdUser.Id)
	if errCreateSettings != nil {
		utils.Warn("Could not create default settings for user %s: %v", createdUser.Id, errCreateSettings)
		// Don't fail registration for this, just log warning
	}

	// Assign free Trial plan to new user
	errTrialSubscription := AssignFreeTrialPlan(createdUser.Id)
	if errTrialSubscription != nil {
		utils.Warn("Could not create Trial subscription for user %s: %v", createdUser.Id, errTrialSubscription)
		// Don't fail registration for this, just log warning
	}

	// Create personal organization for new user
	orgService := organizationServices.NewOrganizationService(sqldb.DB)
	_, errPersonalOrg := orgService.CreatePersonalOrganization(createdUser.Id, createdUser.DisplayName)
	if errPersonalOrg != nil {
		utils.Warn("Could not create personal organization for user %s: %v", createdUser.Id, errPersonalOrg)
		// Don't fail registration for this, just log warning
	} else {
		utils.Info("Successfully created personal organization for user %s", createdUser.Id)
	}

	// Send verification email
	verificationService := NewEmailVerificationService(sqldb.DB)
	err = verificationService.CreateVerificationToken(createdUser.Id, createdUser.Email)
	if err != nil {
		// Log but don't fail registration
		utils.Warn("Could not send verification email to %s: %v", createdUser.Email, err)
	}

	return dto.UserModelToUserOutput(createdUser), nil
}

func NewTerminalTrainerService(db *gorm.DB) ttServices.TerminalTrainerService {
	return ttServices.NewTerminalTrainerService(db)
}

func addDefaultRoleToUser(user1 casdoorsdk.User) error {
	role, errRole := casdoorsdk.GetRole("member")
	if errRole != nil {
		utils.Error("%s", errRole.Error())
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
		utils.Error("%s", errUpdateRole.Error())
		return errUpdateRole
	}
	return nil
}

func createUserIntoCasdoor(generatedUsername string, userCreateDTO dto.CreateUserInput) (casdoorsdk.User, error) {
	properties := make(map[string]string)
	properties["username"] = generatedUsername
	properties["tos_accepted_at"] = userCreateDTO.TosAcceptedAt
	properties["tos_version"] = userCreateDTO.TosVersion

	user1 := casdoorsdk.User{
		Name:              userCreateDTO.UserName,
		DisplayName:       userCreateDTO.DisplayName,
		Email:             userCreateDTO.Email,
		Password:          userCreateDTO.Password,
		LastName:          userCreateDTO.LastName,
		FirstName:         userCreateDTO.FirstName,
		SignupApplication: "ocf",
		EmailVerified:     false,
		Properties:        properties,
	}

	user1.CreatedTime = casdoorsdk.GetCurrentTime()
	_, errCreate := casdoorsdk.AddUser(&user1)
	if errCreate != nil {
		utils.Error("%s", errCreate.Error())
		return casdoorsdk.User{}, errCreate
	}
	return user1, nil
}

func (us *userService) GetUserById(id string) (*dto.UserOutput, error) {
	user, errUser := casdoorsdk.GetUserByUserId(id)
	if errUser != nil {
		utils.Error("%s", errUser.Error())
		return nil, errUser
	}
	if user == nil {
		return nil, errors.New("user not found")
	}

	return dto.UserModelToUserOutput(user), nil
}

func (us *userService) GetAllUsers() (*[]dto.UserOutput, error) {
	users, errUser := casdoorsdk.GetUsers()
	if errUser != nil {
		utils.Error("%s", errUser.Error())
		return nil, errUser
	}

	results := make([]dto.UserOutput, 0, len(users))
	for _, user := range users {
		if user == nil {
			continue
		}
		results = append(results, *dto.UserModelToUserOutput(user))
	}

	return &results, nil
}

// DeleteUser orchestrates the RGPD-compliant cascade deletion of a user.
//
// Order of operations is load-bearing:
//  1. Look up the Casdoor user (fail fast if not found).
//  2. Cancel every active Stripe subscription — on error ABORT so we never
//     leave a deleted user who is still being billed.
//  3. Pseudonymize billing PII (BillingAddress, PaymentMethod). Best-effort:
//     a failure here is logged but does NOT abort, because the security-
//     critical work (Stripe cancel) is already done and we still want the
//     Casdoor row gone.
//  4. Delete the Casdoor user and remove their grouping policies.
//
// Invoices preserved per French law (Art. L. 123-22 Code de commerce, 10y
// legitimate-interest retention).
func (us *userService) DeleteUser(id string) error {
	// Safe to retry on partial failure: Stripe cancellation filters out already-cancelled
	// subs, pseudonymization is a no-op on already-[deleted] rows, and Casdoor delete
	// returns an error that the caller can act on.
	user, errUser := us.casdoorClient.GetUserByUserId(id)
	if errUser != nil {
		utils.Error("%s", errUser.Error())
		return errUser
	}
	if user == nil {
		return errors.New("user not found")
	}

	// Step 1: cancel active Stripe subscriptions BEFORE touching Casdoor.
	// If this fails we must not proceed — otherwise the user is deleted
	// from the identity provider but Stripe keeps charging the card.
	if err := us.paymentHelper.CancelAllActiveSubscriptionsForUser(id); err != nil {
		utils.Error("Aborting user deletion for %s: Stripe cancel failed: %v", id, err)
		return fmt.Errorf("stripe cancellation failed, aborting user deletion: %w", err)
	}

	// Step 2: pseudonymize billing PII. Best-effort — log and continue on
	// failure so the user still gets deleted from the identity provider.
	if err := us.paymentHelper.PseudonymizeBillingDataForUser(id); err != nil {
		utils.Warn("Failed to pseudonymize billing data for user %s (continuing with deletion): %v", id, err)
	}

	// Step 3: delete from Casdoor.
	if _, err := us.casdoorClient.DeleteUser(user); err != nil {
		utils.Error("Failed to delete Casdoor user %s: %v", id, err)
		return err
	}

	// Step 4: remove all role associations for this user. Guarded against a
	// nil enforcer so unit tests that don't wire Casbin can still exercise
	// this path.
	if casdoor.Enforcer != nil {
		opts := utils.DefaultPermissionOptions()
		opts.WarnOnError = true
		utils.RemoveGroupingPolicy(casdoor.Enforcer, user.Id, "", opts)
	}

	return nil
}

// GetUsersByIds retrieves multiple users by their IDs
func (us *userService) GetUsersByIds(ids []string) (*[]dto.UserOutput, error) {
	results := make([]dto.UserOutput, 0, len(ids))

	for _, id := range ids {
		user, errUser := casdoorsdk.GetUserByUserId(id)
		if errUser != nil || user == nil {
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
		utils.Error("%s", errUser.Error())
		return nil, errUser
	}

	results := make([]dto.UserOutput, 0, len(users)/2)
	queryLower := strings.ToLower(strings.TrimSpace(query))

	for _, user := range users {
		if user == nil {
			continue
		}
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

// AssignFreeTrialPlan assigns the free Trial plan to a new user
func AssignFreeTrialPlan(userID string) error {
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
		utils.Info("User %s already has an active subscription, skipping Trial assignment", userID)
		return nil // User already has a subscription
	}

	// Create subscription using the subscription service
	subscriptionService := paymentServices.NewSubscriptionService(sqldb.DB)
	_, err := subscriptionService.CreateUserSubscription(userID, trialPlan.ID)
	if err != nil {
		return fmt.Errorf("failed to create Trial subscription: %w", err)
	}

	utils.Info("Successfully assigned Trial plan to user %s", userID)
	return nil
}
