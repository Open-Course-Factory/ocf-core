package access

import (
	"log"
	"strings"
)

// IsAdmin checks whether any of the given roles indicates administrator status.
// It is case-insensitive and also accepts "admin" as an alias (Casdoor uses both forms).
// This is the canonical helper — all admin checks should delegate here.
func IsAdmin(roles []string) bool {
	for _, role := range roles {
		if strings.EqualFold(role, "administrator") || strings.EqualFold(role, "admin") {
			return true
		}
	}
	return false
}

// roleHierarchy maps role names to their priority for group and organization hierarchies.
// Higher value = more permissions. Call RegisterRole() to add or override roles.
var roleHierarchy = map[string]int{
	"member":  10,
	"manager": 50,
	"owner":   100,
}

// RegisterRole adds or overrides a role in the hierarchy.
// Call at startup to extend the built-in roles (member < manager < owner)
// with custom roles for your project.
//
// Example: access.RegisterRole("supervisor", 75) // between manager and owner
func RegisterRole(name string, priority int) {
	roleHierarchy[name] = priority
}

// GetRoleHierarchy returns a copy of the current role hierarchy (for the reference page).
func GetRoleHierarchy() map[string]int {
	copy := make(map[string]int, len(roleHierarchy))
	for k, v := range roleHierarchy {
		copy[k] = v
	}
	return copy
}

// IsRoleAtLeast checks whether userRole has at least the same privilege level
// as requiredRole within the role hierarchy.
// Returns false if either role is unknown.
func IsRoleAtLeast(userRole, requiredRole string) bool {
	userPriority, userOk := roleHierarchy[userRole]
	requiredPriority, reqOk := roleHierarchy[requiredRole]

	if !userOk {
		log.Printf("[WARN] IsRoleAtLeast: unknown user role %q", userRole)
		return false
	}
	if !reqOk {
		log.Printf("[WARN] IsRoleAtLeast: unknown required role %q", requiredRole)
		return false
	}

	return userPriority >= requiredPriority
}
