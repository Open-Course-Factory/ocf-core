package models

// RoleName represents a system-level role
// After Phase 3 simplification: Only 2 system roles remain
// Business roles (owner, manager, admin, etc.) are now context-based through OrganizationMember and GroupMember
type RoleName string

// System Roles - Simplified to 2 roles only
const (
	Member        RoleName = "member"        // Default authenticated user
	Administrator RoleName = "administrator" // System administrator (platform management)
	Admin         RoleName = Administrator   // Alias for Administrator (backward compatibility)
)

// CasdoorToOCFRoleMap maps Casdoor roles to OCF Core system roles
// All user-level Casdoor roles map to "member" - business roles come from org/group membership
var CasdoorToOCFRoleMap = map[string]RoleName{
	// All regular users map to member
	"user":            Member,
	"member":          Member,
	"student":         Member,
	"premium_student": Member,
	"teacher":         Member,
	"trainer":         Member,
	"supervisor":      Member, // Added supervisor mapping

	// Only system admins map to administrator
	"admin":         Administrator,
	"administrator": Administrator,
}

// GetOCFRoleFromCasdoor converts a Casdoor role to OCF system role
func GetOCFRoleFromCasdoor(casdoorRole string) RoleName {
	if ocfRole, exists := CasdoorToOCFRoleMap[casdoorRole]; exists {
		return ocfRole
	}
	// Default: authenticated users are members
	return Member
}

// GetCasdoorRolesForOCFRole returns all Casdoor roles that map to an OCF role
func GetCasdoorRolesForOCFRole(ocfRole RoleName) []string {
	var casdoorRoles []string
	for casdoorRole, mappedOCFRole := range CasdoorToOCFRoleMap {
		if mappedOCFRole == ocfRole {
			casdoorRoles = append(casdoorRoles, casdoorRole)
		}
	}
	return casdoorRoles
}

// IsSystemAdmin checks if a role is the system administrator
func IsSystemAdmin(role RoleName) bool {
	return role == Administrator
}

// ==================================================================================
// DEPRECATED: Phase 3 Migration - Use Organization/Group membership for features
// ==================================================================================
//
// The following constants and functions are deprecated and kept only for backward
// compatibility during migration. They should not be used in new code.
//
// Instead of role-based features, use:
//   - payment/utils.GetUserEffectiveFeatures(db, userID) - Get features from orgs
//   - payment/utils.CanUserAccessFeature(db, userID, feature) - Check feature access
//   - organizations/services.IsOrganizationMember() - Check org membership
//   - groups/services.IsGroupMember() - Check group membership
//
// Migration Status:
//   - Phase 1 ✅: Organizations and OrganizationMember implemented
//   - Phase 2 ✅: Organization subscriptions and feature aggregation implemented
//   - Phase 3 🔄: Role simplification in progress
// ==================================================================================

// Deprecated: Use organization/group membership instead
// These roles are kept temporarily for backward compatibility
const (
	Guest        RoleName = "guest"         // DEPRECATED: Use unauthenticated state
	MemberPro    RoleName = "member_pro"    // DEPRECATED: Use org subscription
	GroupManager RoleName = "group_manager" // DEPRECATED: Use org/group membership
	Trainer      RoleName = "trainer"       // DEPRECATED: Use org membership
	Organization RoleName = "organization"  // DEPRECATED: Use org ownership
)

// Deprecated: Role hierarchy no longer used. Use Casbin context-based permissions instead.
var RoleHierarchy = map[RoleName][]RoleName{
	Member:        {},
	Administrator: {},
}

// Deprecated: Use payment/utils.GetUserEffectiveFeatures() instead
type RoleFeatures struct {
	MaxCourses            int
	MaxLabSessions        int
	MaxConcurrentUsers    int
	CanCreateAdvancedLabs bool
	CanUseNetwork         bool
	CanExportCourses      bool
	CanUseAPI             bool
	HasPrioritySupport    bool
	CanCustomizeThemes    bool
	HasAnalytics          bool
	StorageLimit          int64
}

// Deprecated: Use payment/utils.GetUserEffectiveFeatures(db, userID) instead
// Features now come from organization subscriptions, not system roles
func GetRoleFeatures(role RoleName) RoleFeatures {
	// Return minimal features for all members
	// Real features should come from organization subscriptions
	return RoleFeatures{
		MaxCourses:            0,
		MaxLabSessions:        0,
		MaxConcurrentUsers:    1,
		CanCreateAdvancedLabs: false,
		CanUseNetwork:         false,
		CanExportCourses:      false,
		CanUseAPI:             false,
		HasPrioritySupport:    false,
		CanCustomizeThemes:    false,
		HasAnalytics:          false,
		StorageLimit:          0,
	}
}

// Deprecated: Use organization subscription status instead
func IsRolePayingUser(role RoleName) bool {
	// Check organization subscriptions instead
	return false
}

// Deprecated: Use organization subscription upgrades instead
func GetUpgradeRecommendations(currentRole RoleName) []RoleName {
	return []RoleName{}
}

// Deprecated: Use Casbin Enforce() with context-based permissions
func HasPermission(userRole RoleName, requiredRole RoleName) bool {
	return userRole == Administrator || userRole == requiredRole
}

// Deprecated: Roles no longer have hierarchy
func GetMaximumRole(roles []RoleName) RoleName {
	for _, role := range roles {
		if role == Administrator {
			return Administrator
		}
	}
	if len(roles) > 0 {
		return roles[0]
	}
	return Member
}
