package services

import (
	"fmt"
	"soli/formations/src/auth/casdoor"
	authDto "soli/formations/src/auth/dto"
	authModels "soli/formations/src/auth/models"
	paymentRepo "soli/formations/src/payment/repositories"
	"soli/formations/src/utils"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// UserPermissionsService provides methods to aggregate user permissions
type UserPermissionsService interface {
	GetUserPermissions(userID string) (*authDto.UserPermissionsOutput, error)
}

type userPermissionsService struct {
	db *gorm.DB
}

// NewUserPermissionsService creates a new user permissions service
func NewUserPermissionsService(db *gorm.DB) UserPermissionsService {
	return &userPermissionsService{
		db: db,
	}
}

// GetUserPermissions aggregates all permissions for a user
func (s *userPermissionsService) GetUserPermissions(userID string) (*authDto.UserPermissionsOutput, error) {
	utils.Debug("Getting permissions for user: %s", userID)

	// 1. Get Casbin implicit permissions (includes inherited permissions from roles)
	casbinPerms, err := casdoor.Enforcer.GetImplicitPermissionsForUser(userID)
	if err != nil {
		utils.Error("Failed to get implicit permissions for user %s: %v", userID, err)
		return nil, fmt.Errorf("failed to get implicit permissions: %w", err)
	}

	// Convert Casbin permissions to PermissionRules
	permissions := make([]authDto.PermissionRule, 0)
	for _, perm := range casbinPerms {
		rule := authDto.CasbinPermissionToRule(perm)
		if rule != nil {
			permissions = append(permissions, *rule)
		}
	}

	// 2. Get user roles
	roles, err := casdoor.Enforcer.GetRolesForUser(userID)
	if err != nil {
		utils.Warn("Failed to get roles for user %s: %v", userID, err)
		roles = []string{} // Continue with empty roles
	}

	// 3. Check if user is system admin
	isSystemAdmin := false
	for _, role := range roles {
		if role == string(authModels.Administrator) || role == "administrator" {
			isSystemAdmin = true
			break
		}
	}

	// 4. Get organization memberships
	orgMemberships, err := s.getOrganizationMemberships(userID)
	if err != nil {
		utils.Warn("Failed to get organization memberships for user %s: %v", userID, err)
		orgMemberships = []authDto.OrganizationMembershipContext{}
	}

	// 5. Get group memberships
	groupMemberships, err := s.getGroupMemberships(userID)
	if err != nil {
		utils.Warn("Failed to get group memberships for user %s: %v", userID, err)
		groupMemberships = []authDto.GroupMembershipContext{}
	}

	// 6. Aggregate features from all organizations
	aggregatedFeatures := s.aggregateFeatures(orgMemberships)

	// 7. Check for any active subscription
	hasAnySubscription := false
	for _, org := range orgMemberships {
		if org.HasSubscription {
			hasAnySubscription = true
			break
		}
	}

	// 8. Build quick access flags
	canCreateOrganization := isSystemAdmin || true // For now, all authenticated users can create orgs
	canCreateGroup := isSystemAdmin || len(orgMemberships) > 0 // Can create groups if member of any org

	result := &authDto.UserPermissionsOutput{
		UserID:                  userID,
		Permissions:             permissions,
		Roles:                   roles,
		IsSystemAdmin:           isSystemAdmin,
		OrganizationMemberships: orgMemberships,
		GroupMemberships:        groupMemberships,
		AggregatedFeatures:      aggregatedFeatures,
		CanCreateOrganization:   canCreateOrganization,
		CanCreateGroup:          canCreateGroup,
		HasAnySubscription:      hasAnySubscription,
	}

	utils.Debug("Permissions aggregated for user %s: %d permissions, %d orgs, %d groups",
		userID, len(permissions), len(orgMemberships), len(groupMemberships))

	return result, nil
}

// getOrganizationMemberships retrieves all organization memberships for a user
func (s *userPermissionsService) getOrganizationMemberships(userID string) ([]authDto.OrganizationMembershipContext, error) {
	// Query with explicit column selection
	type MembershipResult struct {
		MemberID       string `gorm:"column:member_id"`
		OrganizationID string `gorm:"column:organization_id"`
		UserID         string `gorm:"column:user_id"`
		Role           string `gorm:"column:role"`
		OrgName        string `gorm:"column:org_name"`
		OrgDisplayName string `gorm:"column:org_display_name"`
	}

	var results []MembershipResult

	err := s.db.Table("organization_members").
		Select(`
			organization_members.id as member_id,
			organization_members.organization_id,
			organization_members.user_id,
			organization_members.role,
			organizations.name as org_name,
			organizations.display_name as org_display_name
		`).
		Joins("JOIN organizations ON organizations.id = organization_members.organization_id").
		Where("organization_members.user_id = ? AND organization_members.is_active = ?", userID, true).
		Scan(&results).Error

	if err != nil {
		return nil, err
	}

	// Create subscription repository
	subscriptionRepo := paymentRepo.NewOrganizationSubscriptionRepository(s.db)

	contexts := make([]authDto.OrganizationMembershipContext, 0, len(results))
	for _, result := range results {
		orgID, err := uuid.Parse(result.OrganizationID)
		if err != nil {
			utils.Warn("Failed to parse organization ID %s: %v", result.OrganizationID, err)
			continue
		}

		// Fetch organization subscription
		features := []string{}
		hasSubscription := false

		subscription, err := subscriptionRepo.GetActiveOrganizationSubscription(orgID)
		if err != nil {
			// No active subscription - organization is on free tier or no plan
			utils.Debug("No active subscription for organization %s: %v", orgID, err)
		} else {
			// Extract features from subscription plan
			hasSubscription = true
			features = subscription.SubscriptionPlan.Features

			// Additional validation: check subscription status
			if subscription.Status != "active" && subscription.Status != "trialing" {
				hasSubscription = false
				utils.Debug("Organization %s has subscription but status is %s", orgID, subscription.Status)
			}
		}

		context := authDto.OrganizationMembershipContext{
			OrganizationID:   orgID,
			OrganizationName: result.OrgDisplayName,
			Role:             result.Role,
			IsOwner:          result.Role == "owner",
			Features:         features,
			HasSubscription:  hasSubscription,
		}
		contexts = append(contexts, context)
	}

	return contexts, nil
}

// getGroupMemberships retrieves all group memberships for a user
func (s *userPermissionsService) getGroupMemberships(userID string) ([]authDto.GroupMembershipContext, error) {
	// Query with explicit column selection
	type MembershipResult struct {
		MemberID       string `gorm:"column:member_id"`
		GroupID        string `gorm:"column:group_id"`
		UserID         string `gorm:"column:user_id"`
		Role           string `gorm:"column:role"`
		GroupName      string `gorm:"column:group_name"`
		GroupDisplayName string `gorm:"column:group_display_name"`
	}

	var results []MembershipResult

	err := s.db.Table("group_members").
		Select(`
			group_members.id as member_id,
			group_members.group_id,
			group_members.user_id,
			group_members.role,
			class_groups.name as group_name,
			class_groups.display_name as group_display_name
		`).
		Joins("JOIN class_groups ON class_groups.id = group_members.group_id").
		Where("group_members.user_id = ? AND group_members.is_active = ?", userID, true).
		Scan(&results).Error

	if err != nil {
		return nil, err
	}

	contexts := make([]authDto.GroupMembershipContext, 0, len(results))
	for _, result := range results {
		groupID, err := uuid.Parse(result.GroupID)
		if err != nil {
			utils.Warn("Failed to parse group ID %s: %v", result.GroupID, err)
			continue
		}

		context := authDto.GroupMembershipContext{
			GroupID:   groupID,
			GroupName: result.GroupDisplayName,
			Role:      result.Role,
			IsOwner:   result.Role == "owner",
		}
		contexts = append(contexts, context)
	}

	return contexts, nil
}

// aggregateFeatures combines features from all organization memberships
func (s *userPermissionsService) aggregateFeatures(orgMemberships []authDto.OrganizationMembershipContext) []string {
	featureSet := make(map[string]bool)

	for _, org := range orgMemberships {
		for _, feature := range org.Features {
			featureSet[feature] = true
		}
	}

	features := make([]string, 0, len(featureSet))
	for feature := range featureSet {
		features = append(features, feature)
	}

	return features
}
