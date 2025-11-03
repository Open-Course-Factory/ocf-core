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
