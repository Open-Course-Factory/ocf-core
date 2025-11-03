package services

import (
	"fmt"
	"soli/formations/src/auth/casdoor"
	authDto "soli/formations/src/auth/dto"
	authModels "soli/formations/src/auth/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
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

	// 4. NEW: Get all entity memberships generically
	entityMemberships, err := s.getAllEntityMemberships(userID)
	if err != nil {
		utils.Warn("Failed to get entity memberships for user %s: %v", userID, err)
		entityMemberships = make(map[string][]authDto.EntityMembershipContext)
	}

	// 5. BACKWARD COMPATIBILITY: Convert generic memberships to specific types
	orgMemberships := s.convertToOrganizationMemberships(entityMemberships["Organization"])
	groupMemberships := s.convertToGroupMemberships(entityMemberships["ClassGroup"])

	// 6. Aggregate features from all entity memberships
	aggregatedFeatures := s.aggregateFeaturesGeneric(entityMemberships)

	// 7. Check for any active subscription across all entities
	hasAnySubscription := s.hasAnySubscriptionGeneric(entityMemberships)

	// 8. Build quick access flags
	canCreateOrganization := isSystemAdmin || true                // For now, all authenticated users can create orgs
	canCreateGroup := isSystemAdmin || len(entityMemberships) > 0 // Can create groups if member of any entity

	result := &authDto.UserPermissionsOutput{
		UserID:        userID,
		Permissions:   permissions,
		Roles:         roles,
		IsSystemAdmin: isSystemAdmin,

		// NEW: Generic entity memberships
		EntityMemberships: entityMemberships,

		// DEPRECATED: Kept for backward compatibility
		OrganizationMemberships: orgMemberships,
		GroupMemberships:        groupMemberships,

		AggregatedFeatures:    aggregatedFeatures,
		CanCreateOrganization: canCreateOrganization,
		CanCreateGroup:        canCreateGroup,
		HasAnySubscription:    hasAnySubscription,
	}

	// Count total memberships across all entity types
	totalMemberships := 0
	for _, memberships := range entityMemberships {
		totalMemberships += len(memberships)
	}

	utils.Debug("Permissions aggregated for user %s: %d permissions, %d total memberships across %d entity types",
		userID, len(permissions), totalMemberships, len(entityMemberships))

	return result, nil
}

// getEntityMemberships is a generic method to retrieve all memberships for a user in a given entity type
// This replaces getOrganizationMemberships and getGroupMemberships with a unified approach
func (s *userPermissionsService) getEntityMemberships(userID string, entityName string, entityTableName string) ([]authDto.EntityMembershipContext, error) {
	utils.Debug("Getting %s memberships for user: %s", entityName, userID)

	// Get membership config from entity registration
	membershipConfig := ems.GlobalEntityRegistrationService.GetMembershipConfig(entityName)
	if membershipConfig == nil {
		return nil, utils.NewEntityError(entityName, utils.OpFetch, fmt.Sprintf("membership config not found for entity type %s", entityName))
	}

	// Query with explicit column selection
	type MembershipResult struct {
		MemberID       string `gorm:"column:member_id"`
		EntityID       string `gorm:"column:entity_id"`
		UserID         string `gorm:"column:user_id"`
		Role           string `gorm:"column:role"`
		EntityName     string `gorm:"column:entity_name"`
		EntityDispName string `gorm:"column:entity_display_name"`
	}

	var results []MembershipResult

	// Set default for IsActiveColumn if not specified
	isActiveColumn := membershipConfig.IsActiveColumn
	if isActiveColumn == "" {
		isActiveColumn = "is_active"
	}

	err := s.db.Table(membershipConfig.MemberTable).
		Select(fmt.Sprintf(`
			%s.id as member_id,
			%s.%s as entity_id,
			%s.%s as user_id,
			%s.%s as role,
			entities.name as entity_name,
			entities.display_name as entity_display_name
		`,
			membershipConfig.MemberTable,
			membershipConfig.MemberTable, membershipConfig.EntityIDColumn,
			membershipConfig.MemberTable, membershipConfig.UserIDColumn,
			membershipConfig.MemberTable, membershipConfig.RoleColumn,
		)).
		Joins(fmt.Sprintf("JOIN %s AS entities ON entities.id = %s.%s",
			entityTableName,
			membershipConfig.MemberTable,
			membershipConfig.EntityIDColumn,
		)).
		Where(fmt.Sprintf("%s.%s = ? AND %s.%s = ?",
			membershipConfig.MemberTable, membershipConfig.UserIDColumn,
			membershipConfig.MemberTable, isActiveColumn,
		), userID, true).
		Scan(&results).Error

	if err != nil {
		return nil, err
	}

	contexts := make([]authDto.EntityMembershipContext, 0, len(results))
	for _, result := range results {
		entityID, err := uuid.Parse(result.EntityID)
		if err != nil {
			utils.Warn("Failed to parse entity ID %s: %v", result.EntityID, err)
			continue
		}

		// Initialize context
		context := authDto.EntityMembershipContext{
			EntityID:   entityID,
			EntityType: entityName,
			EntityName: result.EntityDispName,
			Role:       result.Role,
			IsOwner:    result.Role == "owner",
		}

		// Fetch features using FeatureProvider if available
		if membershipConfig.FeatureProvider != nil {
			features, hasSubscription, err := membershipConfig.FeatureProvider.GetFeatures(result.EntityID)
			if err != nil {
				utils.Debug("Failed to get features for %s %s: %v", entityName, entityID, err)
				// Continue without features
				features = []string{}
				hasSubscription = false
			}
			context.Features = features
			context.HasSubscription = hasSubscription
		} else {
			context.Features = []string{}
			context.HasSubscription = false
		}

		contexts = append(contexts, context)
	}

	utils.Debug("Found %d %s memberships for user %s", len(contexts), entityName, userID)
	return contexts, nil
}

// getAllEntityMemberships retrieves memberships across all registered entities with membership configs
func (s *userPermissionsService) getAllEntityMemberships(userID string) (map[string][]authDto.EntityMembershipContext, error) {
	memberships := make(map[string][]authDto.EntityMembershipContext)

	// Define entities with their table names
	// TODO: Make this dynamic by querying the registration service for all entities with membership configs
	entityConfigs := map[string]string{
		"Organization": "organizations",
		"ClassGroup":   "class_groups",
	}

	for entityName, tableName := range entityConfigs {
		// Check if entity has a membership config
		config := ems.GlobalEntityRegistrationService.GetMembershipConfig(entityName)
		if config == nil {
			utils.Debug("Skipping entity %s: no membership config", entityName)
			continue
		}

		// Get memberships for this entity type
		entityMemberships, err := s.getEntityMemberships(userID, entityName, tableName)
		if err != nil {
			utils.Warn("Failed to get %s memberships for user %s: %v", entityName, userID, err)
			continue
		}

		if len(entityMemberships) > 0 {
			memberships[entityName] = entityMemberships
		}
	}

	return memberships, nil
}

// aggregateFeaturesGeneric combines features from all entity memberships
func (s *userPermissionsService) aggregateFeaturesGeneric(entityMemberships map[string][]authDto.EntityMembershipContext) []string {
	featureSet := make(map[string]bool)

	for _, memberships := range entityMemberships {
		for _, membership := range memberships {
			for _, feature := range membership.Features {
				featureSet[feature] = true
			}
		}
	}

	features := make([]string, 0, len(featureSet))
	for feature := range featureSet {
		features = append(features, feature)
	}

	return features
}

// hasAnySubscriptionGeneric checks if user has any subscription across all entities
func (s *userPermissionsService) hasAnySubscriptionGeneric(entityMemberships map[string][]authDto.EntityMembershipContext) bool {
	for _, memberships := range entityMemberships {
		for _, membership := range memberships {
			if membership.HasSubscription {
				return true
			}
		}
	}
	return false
}

// convertToOrganizationMemberships converts generic contexts to organization-specific (for backward compatibility)
func (s *userPermissionsService) convertToOrganizationMemberships(contexts []authDto.EntityMembershipContext) []authDto.OrganizationMembershipContext {
	result := make([]authDto.OrganizationMembershipContext, 0, len(contexts))
	for _, ctx := range contexts {
		result = append(result, authDto.OrganizationMembershipContext{
			OrganizationID:   ctx.EntityID,
			OrganizationName: ctx.EntityName,
			Role:             ctx.Role,
			IsOwner:          ctx.IsOwner,
			Features:         ctx.Features,
			HasSubscription:  ctx.HasSubscription,
		})
	}
	return result
}

// convertToGroupMemberships converts generic contexts to group-specific (for backward compatibility)
func (s *userPermissionsService) convertToGroupMemberships(contexts []authDto.EntityMembershipContext) []authDto.GroupMembershipContext {
	result := make([]authDto.GroupMembershipContext, 0, len(contexts))
	for _, ctx := range contexts {
		result = append(result, authDto.GroupMembershipContext{
			GroupID:   ctx.EntityID,
			GroupName: ctx.EntityName,
			Role:      ctx.Role,
			IsOwner:   ctx.IsOwner,
		})
	}
	return result
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
