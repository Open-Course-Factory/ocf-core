package services

import (
	"fmt"
	"soli/formations/src/organizations/models"
	"soli/formations/src/utils"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"gorm.io/gorm"
)

type OrganizationMigrationService interface {
	MigrateExistingUsersToPersonalOrganizations() (*MigrationResult, error)
}

type organizationMigrationService struct {
	db         *gorm.DB
	orgService OrganizationService
}

type MigrationResult struct {
	TotalUsers      int      `json:"total_users"`
	AlreadyHadOrg   int      `json:"already_had_org"`
	NewOrgsCreated  int      `json:"new_orgs_created"`
	FailedCreations int      `json:"failed_creations"`
	FailedUserIDs   []string `json:"failed_user_ids,omitempty"`
}

func NewOrganizationMigrationService(db *gorm.DB) OrganizationMigrationService {
	return &organizationMigrationService{
		db:         db,
		orgService: NewOrganizationService(db),
	}
}

// MigrateExistingUsersToPersonalOrganizations creates personal organizations for all users who don't have one
func (oms *organizationMigrationService) MigrateExistingUsersToPersonalOrganizations() (*MigrationResult, error) {
	result := &MigrationResult{
		FailedUserIDs: make([]string, 0),
	}

	// Get all users from Casdoor
	users, err := casdoorsdk.GetUsers()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch users from Casdoor: %w", err)
	}

	result.TotalUsers = len(users)
	utils.Info("Starting migration: Found %d users to check", result.TotalUsers)

	// Process each user
	for _, user := range users {
		userID := user.Id

		// Check if user already has a personal organization
		var existingOrg models.Organization
		err := oms.db.Where("owner_user_id = ? AND is_personal = ?", userID, true).First(&existingOrg).Error

		if err == nil {
			// User already has a personal organization
			result.AlreadyHadOrg++
			utils.Debug("User %s (%s) already has personal organization: %s", user.Name, userID, existingOrg.ID)
			continue
		}

		if err != gorm.ErrRecordNotFound {
			// Database error (not just "not found")
			utils.Warn("Error checking personal org for user %s: %v", userID, err)
			result.FailedCreations++
			result.FailedUserIDs = append(result.FailedUserIDs, userID)
			continue
		}

		// Create personal organization for this user
		utils.Info("Creating personal organization for user %s (%s)", user.Name, userID)
		_, createErr := oms.orgService.CreatePersonalOrganization(userID)

		if createErr != nil {
			utils.Error("Failed to create personal organization for user %s: %v", userID, createErr)
			result.FailedCreations++
			result.FailedUserIDs = append(result.FailedUserIDs, userID)
		} else {
			result.NewOrgsCreated++
			utils.Info("✅ Created personal organization for user %s (%s)", user.Name, userID)
		}
	}

	utils.Info("Migration completed: %d total users, %d already had org, %d new orgs created, %d failed",
		result.TotalUsers, result.AlreadyHadOrg, result.NewOrgsCreated, result.FailedCreations)

	return result, nil
}
