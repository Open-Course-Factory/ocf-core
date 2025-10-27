package services

import (
	"fmt"
	"time"

	"soli/formations/src/auth/casdoor"
	groupModels "soli/formations/src/groups/models"
	"soli/formations/src/organizations/dto"
	"soli/formations/src/organizations/models"
	"soli/formations/src/organizations/repositories"
	"soli/formations/src/utils"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type OrganizationService interface {
	// Organization management
	CreateOrganization(userID string, input dto.CreateOrganizationInput) (*models.Organization, error)
	CreatePersonalOrganization(userID string) (*models.Organization, error)
	GetOrganization(orgID uuid.UUID, includeRelations bool) (*models.Organization, error)
	GetUserOrganizations(userID string) (*[]models.Organization, error)
	GetUserPersonalOrganization(userID string) (*models.Organization, error)
	GetOrganizationsByOwner(ownerUserID string) (*[]models.Organization, error)
	UpdateOrganization(orgID uuid.UUID, requestingUserID string, updates map[string]interface{}) (*models.Organization, error)
	DeleteOrganization(orgID uuid.UUID, requestingUserID string) error

	// Member management
	AddMemberToOrganization(orgID uuid.UUID, requestingUserID string, userID string, role models.OrganizationMemberRole) error
	AddMembersToOrganization(orgID uuid.UUID, requestingUserID string, userIDs []string, role models.OrganizationMemberRole) error
	RemoveMemberFromOrganization(orgID uuid.UUID, requestingUserID string, userID string) error
	UpdateMemberRole(orgID uuid.UUID, requestingUserID string, userID string, newRole models.OrganizationMemberRole) error
	GetOrganizationMembers(orgID uuid.UUID, includes []string) (*[]models.OrganizationMember, error)
	IsUserInOrganization(orgID uuid.UUID, userID string) (bool, error)
	GetUserOrganizationRole(orgID uuid.UUID, userID string) (models.OrganizationMemberRole, error)

	// Group access
	GetOrganizationGroups(orgID uuid.UUID) (*[]groupModels.ClassGroup, error)
	CanUserAccessGroupViaOrg(groupID uuid.UUID, userID string) (bool, error)

	// Permissions
	CanUserManageOrganization(orgID uuid.UUID, userID string) (bool, error)
	GrantOrganizationPermissions(userID string, orgID uuid.UUID) error
	RevokeOrganizationPermissions(userID string, orgID uuid.UUID) error
	GrantOrganizationManagerPermissions(userID string, orgID uuid.UUID) error
	RevokeOrganizationManagerPermissions(userID string, orgID uuid.UUID) error
}

type organizationService struct {
	repository repositories.OrganizationRepository
	db         *gorm.DB
}

func NewOrganizationService(db *gorm.DB) OrganizationService {
	return &organizationService{
		repository: repositories.NewOrganizationRepository(db),
		db:         db,
	}
}

// CreateOrganization creates a new organization and adds the creator as owner
func (os *organizationService) CreateOrganization(userID string, input dto.CreateOrganizationInput) (*models.Organization, error) {
	// Check if organization name is unique for this user
	existingOrg, _ := os.repository.GetOrganizationByNameAndOwner(input.Name, userID)
	if existingOrg != nil {
		return nil, fmt.Errorf("you already have an organization with this name")
	}

	// Create organization
	org := &models.Organization{
		Name:               input.Name,
		DisplayName:        input.DisplayName,
		Description:        input.Description,
		OwnerUserID:        userID,
		SubscriptionPlanID: input.SubscriptionPlanID,
		IsPersonal:         false,
		MaxGroups:          input.MaxGroups,
		MaxMembers:         input.MaxMembers,
		Metadata:           input.Metadata,
		IsActive:           true,
	}

	if org.MaxGroups == 0 {
		org.MaxGroups = 10 // Default limit
	}
	if org.MaxMembers == 0 {
		org.MaxMembers = 50 // Default limit
	}

	createdOrg, err := os.repository.CreateOrganization(org)
	if err != nil {
		return nil, fmt.Errorf("failed to create organization: %v", err)
	}

	// Automatically add creator as owner-member
	ownerMember := &models.OrganizationMember{
		OrganizationID: createdOrg.ID,
		UserID:         userID,
		Role:           models.OrgRoleOwner,
		InvitedBy:      userID,
		JoinedAt:       time.Now(),
		IsActive:       true,
	}

	err = os.repository.AddOrganizationMember(ownerMember)
	if err != nil {
		// Rollback organization creation if adding owner fails
		os.repository.DeleteOrganization(createdOrg.ID)
		return nil, fmt.Errorf("failed to add owner to organization: %v", err)
	}

	// Grant permissions to the owner (both member and manager permissions)
	err = os.GrantOrganizationPermissions(userID, createdOrg.ID)
	if err != nil {
		utils.Warn("Failed to grant member permissions to organization owner: %v", err)
	}

	err = os.GrantOrganizationManagerPermissions(userID, createdOrg.ID)
	if err != nil {
		utils.Warn("Failed to grant manager permissions to organization owner: %v", err)
	}

	utils.Info("Organization created: %s (ID: %s) by user %s", createdOrg.Name, createdOrg.ID, userID)
	return createdOrg, nil
}

// CreatePersonalOrganization creates a personal organization for a user
func (os *organizationService) CreatePersonalOrganization(userID string) (*models.Organization, error) {
	// Check if personal org already exists
	existingOrg, err := os.repository.GetPersonalOrganization(userID)
	if err == nil && existingOrg != nil {
		return existingOrg, nil
	}

	// Create personal organization
	org := &models.Organization{
		Name:        fmt.Sprintf("personal_%s", userID),
		DisplayName: "Personal Organization",
		Description: "Your personal workspace",
		OwnerUserID: userID,
		IsPersonal:  true,
		MaxGroups:   -1, // Unlimited for personal orgs
		MaxMembers:  1,  // Only owner
		IsActive:    true,
	}

	createdOrg, err := os.repository.CreateOrganization(org)
	if err != nil {
		return nil, fmt.Errorf("failed to create personal organization: %v", err)
	}

	// Add user as owner
	ownerMember := &models.OrganizationMember{
		OrganizationID: createdOrg.ID,
		UserID:         userID,
		Role:           models.OrgRoleOwner,
		InvitedBy:      userID,
		JoinedAt:       time.Now(),
		IsActive:       true,
	}

	err = os.repository.AddOrganizationMember(ownerMember)
	if err != nil {
		os.repository.DeleteOrganization(createdOrg.ID)
		return nil, fmt.Errorf("failed to add owner to personal organization: %v", err)
	}

	// Grant permissions
	os.GrantOrganizationPermissions(userID, createdOrg.ID)
	os.GrantOrganizationManagerPermissions(userID, createdOrg.ID)

	utils.Info("Personal organization created for user %s", userID)
	return createdOrg, nil
}

// GetOrganization retrieves an organization by ID
func (os *organizationService) GetOrganization(orgID uuid.UUID, includeRelations bool) (*models.Organization, error) {
	return os.repository.GetOrganizationByID(orgID, includeRelations)
}

// GetUserOrganizations returns all organizations a user is a member of
func (os *organizationService) GetUserOrganizations(userID string) (*[]models.Organization, error) {
	return os.repository.GetOrganizationsByUserID(userID)
}

// GetUserPersonalOrganization returns a user's personal organization
func (os *organizationService) GetUserPersonalOrganization(userID string) (*models.Organization, error) {
	return os.repository.GetPersonalOrganization(userID)
}

// GetOrganizationsByOwner returns all organizations owned by a user
func (os *organizationService) GetOrganizationsByOwner(ownerUserID string) (*[]models.Organization, error) {
	return os.repository.GetOrganizationsByOwner(ownerUserID)
}

// UpdateOrganization updates an organization (only owner or manager can update)
func (os *organizationService) UpdateOrganization(orgID uuid.UUID, requestingUserID string, updates map[string]interface{}) (*models.Organization, error) {
	// Check if user can manage this organization
	canManage, err := os.CanUserManageOrganization(orgID, requestingUserID)
	if err != nil {
		return nil, err
	}
	if !canManage {
		return nil, fmt.Errorf("you don't have permission to manage this organization")
	}

	updatedOrg, err := os.repository.UpdateOrganization(orgID, updates)
	if err != nil {
		return nil, fmt.Errorf("failed to update organization: %v", err)
	}

	return updatedOrg, nil
}

// DeleteOrganization deletes an organization (only owner can delete)
func (os *organizationService) DeleteOrganization(orgID uuid.UUID, requestingUserID string) error {
	org, err := os.repository.GetOrganizationByID(orgID, false)
	if err != nil {
		return err
	}

	// Only owner can delete
	if org.OwnerUserID != requestingUserID {
		return fmt.Errorf("only the organization owner can delete the organization")
	}

	// Cannot delete personal organization
	if org.IsPersonal {
		return fmt.Errorf("cannot delete personal organization")
	}

	// Revoke all member permissions
	members, err := os.repository.GetOrganizationMembers(orgID, []string{})
	if err == nil && members != nil {
		for _, member := range *members {
			os.RevokeOrganizationPermissions(member.UserID, orgID)
			os.RevokeOrganizationManagerPermissions(member.UserID, orgID)
		}
	}

	return os.repository.DeleteOrganization(orgID)
}

// AddMemberToOrganization adds a single member to an organization
func (os *organizationService) AddMemberToOrganization(orgID uuid.UUID, requestingUserID string, userID string, role models.OrganizationMemberRole) error {
	return os.AddMembersToOrganization(orgID, requestingUserID, []string{userID}, role)
}

// AddMembersToOrganization adds multiple members to an organization
func (os *organizationService) AddMembersToOrganization(orgID uuid.UUID, requestingUserID string, userIDs []string, role models.OrganizationMemberRole) error {
	// Check if user can manage this organization
	canManage, err := os.CanUserManageOrganization(orgID, requestingUserID)
	if err != nil {
		return err
	}
	if !canManage {
		return fmt.Errorf("you don't have permission to add members to this organization")
	}

	// Get organization to check limits
	org, err := os.repository.GetOrganizationByID(orgID, true)
	if err != nil {
		return err
	}

	// Check if organization is full
	if org.MaxMembers > 0 && len(org.Members)+len(userIDs) > org.MaxMembers {
		return fmt.Errorf("organization is full (max %d members)", org.MaxMembers)
	}

	// Add each member
	for _, userID := range userIDs {
		// Check if already a member
		isMember, _ := os.IsUserInOrganization(orgID, userID)
		if isMember {
			utils.Warn("User %s is already a member of organization %s", userID, orgID)
			continue
		}

		member := &models.OrganizationMember{
			OrganizationID: orgID,
			UserID:         userID,
			Role:           role,
			InvitedBy:      requestingUserID,
			JoinedAt:       time.Now(),
			IsActive:       true,
		}

		err = os.repository.AddOrganizationMember(member)
		if err != nil {
			utils.Error("Failed to add user %s to organization %s: %v", userID, orgID, err)
			continue
		}

		// Grant appropriate permissions based on role
		err = os.GrantOrganizationPermissions(userID, orgID)
		if err != nil {
			utils.Warn("Failed to grant member permissions to user %s: %v", userID, err)
		}

		if role == models.OrgRoleOwner || role == models.OrgRoleManager {
			err = os.GrantOrganizationManagerPermissions(userID, orgID)
			if err != nil {
				utils.Warn("Failed to grant manager permissions to user %s: %v", userID, err)
			}
		}

		utils.Info("User %s added to organization %s with role %s", userID, orgID, role)
	}

	return nil
}

// RemoveMemberFromOrganization removes a member from an organization
func (os *organizationService) RemoveMemberFromOrganization(orgID uuid.UUID, requestingUserID string, userID string) error {
	// Check if user can manage this organization
	canManage, err := os.CanUserManageOrganization(orgID, requestingUserID)
	if err != nil {
		return err
	}
	if !canManage {
		return fmt.Errorf("you don't have permission to remove members from this organization")
	}

	// Get organization
	org, err := os.repository.GetOrganizationByID(orgID, false)
	if err != nil {
		return err
	}

	// Cannot remove the owner
	if org.OwnerUserID == userID {
		return fmt.Errorf("cannot remove the organization owner")
	}

	// Remove member
	err = os.repository.RemoveOrganizationMember(orgID, userID)
	if err != nil {
		return fmt.Errorf("failed to remove member: %v", err)
	}

	// Revoke permissions
	os.RevokeOrganizationPermissions(userID, orgID)
	os.RevokeOrganizationManagerPermissions(userID, orgID)

	utils.Info("User %s removed from organization %s", userID, orgID)
	return nil
}

// UpdateMemberRole updates a member's role in an organization
func (os *organizationService) UpdateMemberRole(orgID uuid.UUID, requestingUserID string, userID string, newRole models.OrganizationMemberRole) error {
	// Check if user can manage this organization
	canManage, err := os.CanUserManageOrganization(orgID, requestingUserID)
	if err != nil {
		return err
	}
	if !canManage {
		return fmt.Errorf("you don't have permission to update roles in this organization")
	}

	// Get organization
	org, err := os.repository.GetOrganizationByID(orgID, false)
	if err != nil {
		return err
	}

	// Cannot change owner role
	if org.OwnerUserID == userID && newRole != models.OrgRoleOwner {
		return fmt.Errorf("cannot change the owner's role")
	}

	// Update role
	err = os.repository.UpdateOrganizationMemberRole(orgID, userID, newRole)
	if err != nil {
		return fmt.Errorf("failed to update member role: %v", err)
	}

	// Update permissions based on new role
	if newRole == models.OrgRoleOwner || newRole == models.OrgRoleManager {
		// Grant manager permissions
		os.GrantOrganizationManagerPermissions(userID, orgID)
	} else {
		// Revoke manager permissions (keep member permissions)
		os.RevokeOrganizationManagerPermissions(userID, orgID)
	}

	utils.Info("User %s role updated to %s in organization %s", userID, newRole, orgID)
	return nil
}

// GetOrganizationMembers returns all members of an organization
func (os *organizationService) GetOrganizationMembers(orgID uuid.UUID, includes []string) (*[]models.OrganizationMember, error) {
	return os.repository.GetOrganizationMembers(orgID, includes)
}

// IsUserInOrganization checks if a user is a member of an organization
func (os *organizationService) IsUserInOrganization(orgID uuid.UUID, userID string) (bool, error) {
	member, err := os.repository.GetOrganizationMember(orgID, userID)
	if err != nil || member == nil {
		return false, nil
	}
	return member.IsActive, nil
}

// GetUserOrganizationRole returns the user's role in an organization
func (os *organizationService) GetUserOrganizationRole(orgID uuid.UUID, userID string) (models.OrganizationMemberRole, error) {
	member, err := os.repository.GetOrganizationMember(orgID, userID)
	if err != nil || member == nil {
		return "", fmt.Errorf("user is not a member of this organization")
	}
	return member.Role, nil
}

// GetOrganizationGroups returns all groups belonging to an organization
func (os *organizationService) GetOrganizationGroups(orgID uuid.UUID) (*[]groupModels.ClassGroup, error) {
	return os.repository.GetOrganizationGroups(orgID)
}

// CanUserAccessGroupViaOrg checks if a user can access a group through organization membership
func (os *organizationService) CanUserAccessGroupViaOrg(groupID uuid.UUID, userID string) (bool, error) {
	// Get the group's organization
	var group groupModels.ClassGroup
	result := os.db.Where("id = ?", groupID).First(&group)
	if result.Error != nil {
		return false, result.Error
	}

	// If group doesn't belong to an organization, no org-based access
	if group.OrganizationID == nil {
		return false, nil
	}

	// Check if user is a manager in the organization
	member, err := os.repository.GetOrganizationMember(*group.OrganizationID, userID)
	if err != nil || member == nil {
		return false, nil
	}

	// Only managers and owners have cascading access to all org groups
	return member.IsManager(), nil
}

// CanUserManageOrganization checks if a user can manage an organization (owner or manager)
func (os *organizationService) CanUserManageOrganization(orgID uuid.UUID, userID string) (bool, error) {
	org, err := os.repository.GetOrganizationByID(orgID, false)
	if err != nil {
		return false, err
	}

	// Owner can always manage
	if org.OwnerUserID == userID {
		return true, nil
	}

	// Check if user is a manager member
	member, err := os.repository.GetOrganizationMember(orgID, userID)
	if err != nil || member == nil {
		return false, nil
	}

	return member.IsManager(), nil
}

// GrantOrganizationPermissions grants basic organization-related permissions to a user via Casbin
func (os *organizationService) GrantOrganizationPermissions(userID string, orgID uuid.UUID) error {
	opts := utils.DefaultPermissionOptions()
	opts.WarnOnError = true // Non-critical permissions log warnings instead of failing

	// Grant basic organization access (GET permission) with sub-resources (members, groups)
	err := utils.GrantCompleteEntityAccess(casdoor.Enforcer, userID, "organization", orgID.String(),
		[]string{"members", "groups"}, opts)
	if err != nil {
		return err
	}

	utils.Debug("Granted organization permissions to user %s for organization %s", userID, orgID)
	return nil
}

// RevokeOrganizationPermissions revokes organization permissions from a user
func (os *organizationService) RevokeOrganizationPermissions(userID string, orgID uuid.UUID) error {
	opts := utils.DefaultPermissionOptions()

	// Revoke entity access (removes user from organization role)
	err := utils.RevokeEntityAccess(casdoor.Enforcer, userID, "organization", orgID.String(), opts)
	if err != nil {
		return err
	}

	utils.Debug("Revoked organization permissions from user %s for organization %s", userID, orgID)
	return nil
}

// GrantOrganizationManagerPermissions grants manager-level permissions (manage org, members, groups)
func (os *organizationService) GrantOrganizationManagerPermissions(userID string, orgID uuid.UUID) error {
	opts := utils.DefaultPermissionOptions()
	opts.WarnOnError = true // Non-critical permissions log warnings instead of failing

	// Grant manager permissions with access to sub-resources (members, groups)
	err := utils.GrantManagerPermissions(casdoor.Enforcer, userID, "organization", orgID.String(),
		[]string{"members", "groups"}, opts)
	if err != nil {
		return err
	}

	utils.Debug("Granted organization manager permissions to user %s for organization %s", userID, orgID)
	return nil
}

// RevokeOrganizationManagerPermissions revokes manager permissions from a user
func (os *organizationService) RevokeOrganizationManagerPermissions(userID string, orgID uuid.UUID) error {
	opts := utils.DefaultPermissionOptions()

	// Revoke manager permissions (removes user from organization_manager role)
	err := utils.RevokeManagerPermissions(casdoor.Enforcer, userID, "organization", orgID.String(), opts)
	if err != nil {
		return err
	}

	utils.Debug("Revoked organization manager permissions from user %s for organization %s", userID, orgID)
	return nil
}
