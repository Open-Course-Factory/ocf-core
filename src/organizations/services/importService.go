package services

import (
	"fmt"
	"mime/multipart"
	"strconv"
	"strings"
	"time"

	"soli/formations/src/auth/casdoor"
	groupModels "soli/formations/src/groups/models"
	"soli/formations/src/organizations/dto"
	organizationModels "soli/formations/src/organizations/models"
	orgUtils "soli/formations/src/organizations/utils"
	paymentModels "soli/formations/src/payment/models"
	paymentServices "soli/formations/src/payment/services"
	ttServices "soli/formations/src/terminalTrainer/services"
	"soli/formations/src/utils"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ImportService interface {
	ImportOrganizationData(
		orgID uuid.UUID,
		ownerUserID string,
		usersFile *multipart.FileHeader,
		groupsFile *multipart.FileHeader,
		membershipsFile *multipart.FileHeader,
		dryRun bool,
		updateExisting bool,
		targetGroup string,
	) (*dto.ImportOrganizationDataResponse, error)
}

type importService struct {
	db              *gorm.DB
	terminalService ttServices.TerminalTrainerService
}

func NewImportService(db *gorm.DB) ImportService {
	return &importService{
		db:              db,
		terminalService: ttServices.NewTerminalTrainerService(db),
	}
}

// ImportOrganizationData handles the bulk import of users, groups, and memberships
func (s *importService) ImportOrganizationData(
	orgID uuid.UUID,
	ownerUserID string,
	usersFile *multipart.FileHeader,
	groupsFile *multipart.FileHeader,
	membershipsFile *multipart.FileHeader,
	dryRun bool,
	updateExisting bool,
	targetGroup string,
) (*dto.ImportOrganizationDataResponse, error) {

	startTime := time.Now()
	response := &dto.ImportOrganizationDataResponse{
		Success: false,
		DryRun:  dryRun,
		Summary: dto.ImportSummary{
			StartTime: startTime,
		},
		Errors:      []dto.ImportError{},
		Warnings:    []dto.ImportWarning{},
		Credentials: []dto.UserCredential{},
	}

	// 1. Load organization
	var org organizationModels.Organization
	if err := s.db.Preload("Members").Preload("Groups").First(&org, orgID).Error; err != nil {
		response.Errors = append(response.Errors, dto.ImportError{
			Row:     0,
			File:    "organization",
			Message: fmt.Sprintf("Organization not found: %v", err),
			Code:    dto.ErrCodeNotFound,
		})
		return response, fmt.Errorf("organization not found: %w", err)
	}

	// 2. Parse CSV files
	users, userErrors, userWarnings := orgUtils.ParseUsersCSV(usersFile)
	response.Errors = append(response.Errors, userErrors...)
	response.Warnings = append(response.Warnings, userWarnings...)

	var groups []dto.GroupImportRow
	var groupErrors []dto.ImportError
	if groupsFile != nil {
		groups, groupErrors = orgUtils.ParseGroupsCSV(groupsFile)
		response.Errors = append(response.Errors, groupErrors...)
	}

	var memberships []dto.MembershipImportRow
	var membershipErrors []dto.ImportError
	if membershipsFile != nil {
		memberships, membershipErrors = orgUtils.ParseMembershipsCSV(membershipsFile)
		response.Errors = append(response.Errors, membershipErrors...)
	}

	// Stop if parsing errors
	if len(response.Errors) > 0 {
		return response, fmt.Errorf("CSV parsing errors: %d errors found", len(response.Errors))
	}

	// 3. Validate organization limits
	limitErrors := s.validateOrganizationLimits(&org, users, groups)
	if len(limitErrors) > 0 {
		response.Errors = append(response.Errors, limitErrors...)
		return response, fmt.Errorf("organization limits exceeded")
	}

	// 4. Start transaction (or dry-run simulation)
	tx := s.db.Begin()
	if tx.Error != nil {
		response.Errors = append(response.Errors, dto.ImportError{
			Row:     0,
			File:    "transaction",
			Message: fmt.Sprintf("Could not start transaction: %v", tx.Error),
			Code:    dto.ErrCodeValidation,
		})
		return response, tx.Error
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			response.Errors = append(response.Errors, dto.ImportError{
				Row:     0,
				File:    "transaction",
				Message: fmt.Sprintf("Panic during import: %v", r),
				Code:    dto.ErrCodeValidation,
			})
		}
	}()

	// 5. Process users
	emailToUserID := make(map[string]string)
	for i := range users {
		user := &users[i]

		// Auto-generate password if not provided
		if user.Password == "" {
			user.Password = orgUtils.GenerateSecurePassword(16)
			user.ForceReset = "true"
			user.GeneratedPassword = user.Password
		}

		// Default role if not provided
		if user.Role == "" {
			user.Role = "member"
		}

		userID, err := s.processUser(*user, orgID, updateExisting, dryRun)
		if err != nil {
			response.Errors = append(response.Errors, dto.ImportError{
				Row:     i + 2, // +2 for header and 0-index
				File:    "users",
				Message: fmt.Sprintf("Failed to process user %s: %v", user.Email, err),
				Code:    dto.ErrCodeValidation,
			})
			continue
		}

		if userID != "" {
			emailToUserID[user.Email] = userID
			if strings.ToLower(user.UpdateIfExists) == "true" || updateExisting {
				response.Summary.UsersUpdated++
			} else {
				response.Summary.UsersCreated++
			}

			// Collect generated credentials (skip during dry-run — passwords would differ on real import)
			if !dryRun && user.GeneratedPassword != "" {
				response.Credentials = append(response.Credentials, dto.UserCredential{
					Email:    user.Email,
					Password: user.GeneratedPassword,
					Name:     fmt.Sprintf("%s %s", user.FirstName, user.LastName),
				})
			}
		} else {
			response.Summary.UsersSkipped++
		}
	}

	// 6. Process groups (with nested support)
	groupNameToID := make(map[string]uuid.UUID)
	if groupsFile != nil {
		// First pass: create all groups without parent references
		for i, group := range groups {
			groupID, err := s.processGroup(group, orgID, ownerUserID, nil, dryRun)
			if err != nil {
				response.Errors = append(response.Errors, dto.ImportError{
					Row:     i + 2,
					File:    "groups",
					Message: fmt.Sprintf("Failed to process group %s: %v", group.GroupName, err),
					Code:    dto.ErrCodeValidation,
				})
				continue
			}

			if groupID != uuid.Nil {
				groupNameToID[group.GroupName] = groupID
				response.Summary.GroupsCreated++
			}
		}

		// Second pass: update parent references for nested groups
		for i, group := range groups {
			if group.ParentGroup != "" {
				parentID, exists := groupNameToID[group.ParentGroup]
				if !exists {
					response.Errors = append(response.Errors, dto.ImportError{
						Row:     i + 2,
						File:    "groups",
						Field:   "parent_group",
						Message: fmt.Sprintf("Parent group '%s' not found for group '%s'", group.ParentGroup, group.GroupName),
						Code:    dto.ErrCodeNotFound,
					})
					continue
				}

				groupID := groupNameToID[group.GroupName]
				if !dryRun {
					if err := tx.Model(&groupModels.ClassGroup{}).Where("id = ?", groupID).Update("parent_group_id", parentID).Error; err != nil {
						response.Errors = append(response.Errors, dto.ImportError{
							Row:     i + 2,
							File:    "groups",
							Message: fmt.Sprintf("Failed to set parent for group %s: %v", group.GroupName, err),
							Code:    dto.ErrCodeValidation,
						})
					}
				}
			}
		}
	}

	// 6.5 Target group: assign all imported users to a specific group
	if targetGroup != "" {
		var group groupModels.ClassGroup
		result := s.db.Where("(id::text = ? OR name = ?) AND organization_id = ?", targetGroup, targetGroup, orgID).First(&group)
		if result.Error != nil {
			response.Errors = append(response.Errors, dto.ImportError{
				Row:     0,
				File:    "users",
				Field:   "target_group",
				Message: fmt.Sprintf("Target group '%s' not found in organization", targetGroup),
				Code:    dto.ErrCodeNotFound,
			})
		} else {
			for email, userID := range emailToUserID {
				if dryRun {
					utils.Debug("[DRY-RUN] Would add user %s to target group %s", email, targetGroup)
					response.Summary.MembershipsCreated++
					continue
				}

				// Check if membership already exists
				var existingMember groupModels.GroupMember
				memberResult := s.db.Where("group_id = ? AND user_id = ?", group.ID, userID).First(&existingMember)
				if memberResult.Error == nil {
					continue // Already a member
				}

				newMember := groupModels.GroupMember{
					GroupID:   group.ID,
					UserID:    userID,
					Role:      groupModels.GroupMemberRole("member"),
					InvitedBy: ownerUserID,
					JoinedAt:  time.Now(),
					IsActive:  true,
				}

				if err := s.db.Create(&newMember).Error; err != nil {
					response.Warnings = append(response.Warnings, dto.ImportWarning{
						Row:     0,
						File:    "users",
						Message: fmt.Sprintf("Failed to add user %s to target group: %v", email, err),
					})
					continue
				}

				response.Summary.MembershipsCreated++
			}
		}
	}

	// 7. Process memberships
	if membershipsFile != nil {
		for i, membership := range memberships {
			err := s.processMembership(membership, emailToUserID, groupNameToID, orgID, ownerUserID, dryRun)
			if err != nil {
				response.Errors = append(response.Errors, dto.ImportError{
					Row:     i + 2,
					File:    "memberships",
					Message: fmt.Sprintf("Failed to process membership %s -> %s: %v", membership.UserEmail, membership.GroupName, err),
					Code:    dto.ErrCodeValidation,
				})
				continue
			}

			response.Summary.MembershipsCreated++
		}
	}

	// 8. Commit or rollback
	if dryRun {
		tx.Rollback()
		utils.Info("Dry-run mode: All changes rolled back")
	} else {
		if err := tx.Commit().Error; err != nil {
			tx.Rollback()
			response.Errors = append(response.Errors, dto.ImportError{
				Row:     0,
				File:    "transaction",
				Message: fmt.Sprintf("Failed to commit transaction: %v", err),
				Code:    dto.ErrCodeValidation,
			})
			return response, err
		}
		utils.Info("Import committed successfully")
	}

	// 9. Calculate summary
	response.Summary.TotalProcessed = response.Summary.UsersCreated + response.Summary.UsersUpdated +
		response.Summary.UsersSkipped + response.Summary.GroupsCreated + response.Summary.MembershipsCreated
	response.Summary.ProcessingTime = time.Since(startTime).String()
	response.Success = len(response.Errors) == 0

	return response, nil
}

// processUser creates or updates a user in Casdoor
func (s *importService) processUser(user dto.UserImportRow, orgID uuid.UUID, updateExisting bool, dryRun bool) (string, error) {
	if dryRun {
		utils.Debug("[DRY-RUN] Would create/update user: %s", user.Email)
		return "dry-run-user-id", nil
	}

	// Check if user already exists
	existingUser, err := casdoorsdk.GetUserByEmail(user.Email)
	if err == nil && existingUser != nil {
		// User exists
		if !updateExisting {
			utils.Debug("User %s already exists, skipping", user.Email)
			return "", nil
		}

		// Update user
		utils.Debug("Updating existing user: %s", user.Email)
		existingUser.FirstName = user.FirstName
		existingUser.LastName = user.LastName
		existingUser.DisplayName = fmt.Sprintf("%s %s", user.FirstName, user.LastName)

		// Update password if provided
		if user.Password != "" {
			existingUser.Password = user.Password
		}

		// Set force reset flag
		if strings.ToLower(user.ForceReset) == "true" {
			if existingUser.Properties == nil {
				existingUser.Properties = make(map[string]string)
			}
			existingUser.Properties["force_password_reset"] = "true"
		}

		_, errUpdate := casdoorsdk.UpdateUser(existingUser)
		if errUpdate != nil {
			return "", fmt.Errorf("failed to update user in Casdoor: %v", errUpdate)
		}

		// Add user to organization
		s.addUserToOrganization(existingUser.Id, orgID)

		return existingUser.Id, nil
	}

	// Create new user
	utils.Debug("Creating new user: %s", user.Email)

	// Generate ToS acceptance timestamp (current time for bulk import)
	tosTime := time.Now().Format(time.RFC3339)
	tosVersion := time.Now().Format("2006-01-02")

	// Prepare Casdoor user properties
	properties := make(map[string]string)
	properties["tos_accepted_at"] = tosTime
	properties["tos_version"] = tosVersion
	if strings.ToLower(user.ForceReset) == "true" {
		properties["force_password_reset"] = "true"
	}

	// Create user directly in Casdoor
	newUser := casdoorsdk.User{
		Name:              fmt.Sprintf("%s_%s_%d", user.FirstName, user.LastName, time.Now().Unix()),
		DisplayName:       fmt.Sprintf("%s %s", user.FirstName, user.LastName),
		Email:             user.Email,
		Password:          user.Password,
		FirstName:         user.FirstName,
		LastName:          user.LastName,
		SignupApplication: "ocf",
		Properties:        properties,
		CreatedTime:       casdoorsdk.GetCurrentTime(),
	}

	_, err = casdoorsdk.AddUser(&newUser)
	if err != nil {
		return "", fmt.Errorf("failed to create user in Casdoor: %w", err)
	}

	// Get the created user to get the actual ID
	createdUser, err := casdoorsdk.GetUserByEmail(user.Email)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve created user: %w", err)
	}

	// Add default roles
	s.addRolesToUser(createdUser.Id, user.Role)

	// Add user to organization
	s.addUserToOrganization(createdUser.Id, orgID)

	// Assign free Trial plan (same as normal registration)
	if err := s.assignFreeTrialPlan(createdUser.Id); err != nil {
		utils.Warn("Could not assign Trial plan to imported user %s: %v", createdUser.Id, err)
	}

	// Create terminal trainer key (same as normal registration)
	if err := s.terminalService.CreateUserKey(createdUser.Id, createdUser.Name); err != nil {
		utils.Warn("Could not create terminal trainer key for imported user %s: %v", createdUser.Id, err)
	}

	return createdUser.Id, nil
}

// addRolesToUser adds roles to a user in Casbin
func (s *importService) addRolesToUser(userID string, primaryRole string) {
	rolesToAdd := []string{}
	if primaryRole != "" {
		rolesToAdd = append(rolesToAdd, primaryRole)
	}

	// Add basic roles for compatibility
	rolesToAdd = append(rolesToAdd, "student", "member")

	opts := utils.DefaultPermissionOptions()
	opts.WarnOnError = true

	for _, role := range rolesToAdd {
		err := utils.AddGroupingPolicy(casdoor.Enforcer, userID, role, opts)
		if err != nil {
			utils.Warn("Could not add role %s to user %s: %v", role, userID, err)
		} else {
			utils.Debug("Added role '%s' to user %s", role, userID)
		}
	}
}

// processGroup creates a group
func (s *importService) processGroup(group dto.GroupImportRow, orgID uuid.UUID, ownerUserID string, parentID *uuid.UUID, dryRun bool) (uuid.UUID, error) {
	if dryRun {
		utils.Debug("[DRY-RUN] Would create group: %s", group.GroupName)
		return uuid.New(), nil
	}

	// Parse max members
	maxMembers := 50
	if group.MaxMembers != "" {
		parsed, err := strconv.Atoi(group.MaxMembers)
		if err == nil && parsed > 0 {
			maxMembers = parsed
		}
	}

	// Parse expiration date
	var expiresAt *time.Time
	if group.ExpiresAt != "" {
		parsed, err := time.Parse(time.RFC3339, group.ExpiresAt)
		if err == nil {
			expiresAt = &parsed
		}
	}

	newGroup := groupModels.ClassGroup{
		Name:           group.GroupName,
		DisplayName:    group.DisplayName,
		Description:    group.Description,
		OwnerUserID:    ownerUserID,
		OrganizationID: &orgID,
		ParentGroupID:  parentID,
		MaxMembers:     maxMembers,
		ExpiresAt:      expiresAt,
		IsActive:       true,
	}

	if err := s.db.Create(&newGroup).Error; err != nil {
		return uuid.Nil, fmt.Errorf("failed to create group: %w", err)
	}

	utils.Debug("Created group: %s (ID: %s)", group.GroupName, newGroup.ID)
	return newGroup.ID, nil
}

// processMembership adds a user to a group
func (s *importService) processMembership(
	membership dto.MembershipImportRow,
	emailToUserID map[string]string,
	groupNameToID map[string]uuid.UUID,
	orgID uuid.UUID,
	inviterID string,
	dryRun bool,
) error {
	userID, userExists := emailToUserID[membership.UserEmail]
	if !userExists {
		return fmt.Errorf("user %s not found in import", membership.UserEmail)
	}

	groupID, groupExists := groupNameToID[membership.GroupName]
	if !groupExists {
		return fmt.Errorf("group %s not found in import", membership.GroupName)
	}

	if dryRun {
		utils.Debug("[DRY-RUN] Would add user %s to group %s", membership.UserEmail, membership.GroupName)
		return nil
	}

	// Check if membership already exists
	var existingMember groupModels.GroupMember
	result := s.db.Where("group_id = ? AND user_id = ?", groupID, userID).First(&existingMember)
	if result.Error == nil {
		utils.Debug("Membership already exists: %s -> %s", membership.UserEmail, membership.GroupName)
		return nil
	}

	// Create membership
	newMember := groupModels.GroupMember{
		GroupID:   groupID,
		UserID:    userID,
		Role:      groupModels.GroupMemberRole(strings.ToLower(membership.Role)),
		InvitedBy: inviterID,
		JoinedAt:  time.Now(),
		IsActive:  true,
	}

	if err := s.db.Create(&newMember).Error; err != nil {
		return fmt.Errorf("failed to create membership: %w", err)
	}

	utils.Debug("Added user %s to group %s", membership.UserEmail, membership.GroupName)
	return nil
}

// addUserToOrganization adds a user to an organization as a member
func (s *importService) addUserToOrganization(userID string, orgID uuid.UUID) error {
	// Check if already a member
	var existingMember organizationModels.OrganizationMember
	result := s.db.Where("organization_id = ? AND user_id = ?", orgID, userID).First(&existingMember)
	if result.Error == nil {
		return nil // Already a member
	}

	// Add as member
	newMember := organizationModels.OrganizationMember{
		OrganizationID: orgID,
		UserID:         userID,
		Role:           organizationModels.OrgRoleMember,
		JoinedAt:       time.Now(),
		IsActive:       true,
	}

	if err := s.db.Create(&newMember).Error; err != nil {
		return fmt.Errorf("failed to add user to organization: %w", err)
	}

	// Add Casbin permission for organization
	opts := utils.DefaultPermissionOptions()
	opts.WarnOnError = true
	utils.AddPolicy(casdoor.Enforcer, userID, fmt.Sprintf("/api/v1/organizations/%s", orgID), "GET|POST|PATCH", opts)

	return nil
}

// assignFreeTrialPlan assigns the free Trial plan to a new user.
// NOTE: Duplicated from auth/services/userService.go:AssignFreeTrialPlan
// because importing auth/services creates a circular dependency.
// Keep in sync with the original.
func (s *importService) assignFreeTrialPlan(userID string) error {
	var trialPlan paymentModels.SubscriptionPlan
	result := s.db.Where("name = ? AND price_amount = 0 AND is_active = true", "Trial").First(&trialPlan)
	if result.Error != nil {
		return fmt.Errorf("could not find active Trial plan: %v", result.Error)
	}

	var existingSub paymentModels.UserSubscription
	existingResult := s.db.Where("user_id = ? AND status = ?", userID, "active").First(&existingSub)
	if existingResult.Error == nil {
		utils.Info("User %s already has an active subscription, skipping Trial assignment", userID)
		return nil
	}

	subscriptionService := paymentServices.NewSubscriptionService(s.db)
	_, err := subscriptionService.CreateUserSubscription(userID, trialPlan.ID)
	if err != nil {
		return fmt.Errorf("failed to create Trial subscription: %w", err)
	}

	utils.Info("Successfully assigned Trial plan to user %s", userID)
	return nil
}

// validateOrganizationLimits checks if the import would exceed organization limits
func (s *importService) validateOrganizationLimits(
	org *organizationModels.Organization,
	users []dto.UserImportRow,
	groups []dto.GroupImportRow,
) []dto.ImportError {
	var errors []dto.ImportError

	// Check member limit
	if org.MaxMembers > 0 {
		currentMembers := len(org.Members)
		newMembers := len(users)
		totalMembers := currentMembers + newMembers

		if err := utils.ValidateLimitNotReached(totalMembers, org.MaxMembers, "members"); err != nil {
			errors = append(errors, dto.ImportError{
				Row:     0,
				File:    "users",
				Message: fmt.Sprintf("Organization member limit exceeded: current %d + new %d = %d (max: %d)", currentMembers, newMembers, totalMembers, org.MaxMembers),
				Code:    dto.ErrCodeLimitExceeded,
			})
		}
	}

	// Check group limit
	if org.MaxGroups > 0 {
		currentGroups := len(org.Groups)
		newGroups := len(groups)
		totalGroups := currentGroups + newGroups

		if err := utils.ValidateLimitNotReached(totalGroups, org.MaxGroups, "groups"); err != nil {
			errors = append(errors, dto.ImportError{
				Row:     0,
				File:    "groups",
				Message: fmt.Sprintf("Organization group limit exceeded: current %d + new %d = %d (max: %d)", currentGroups, newGroups, totalGroups, org.MaxGroups),
				Code:    dto.ErrCodeLimitExceeded,
			})
		}
	}

	return errors
}
