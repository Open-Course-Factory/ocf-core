package casbin

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

// rolePriority maps role names to their priority for both group and organization hierarchies.
// Higher value = more permissions.
var rolePriority = map[string]int{
	"member":  10,
	"manager": 50,
	"owner":   100,
}

// IsRoleAtLeast checks whether userRole has at least the same privilege level
// as requiredRole within the group role hierarchy (member < manager < owner).
// Returns false if either role is unknown.
func IsRoleAtLeast(userRole, requiredRole string) bool {
	userPriority, userOk := rolePriority[userRole]
	requiredPriority, reqOk := rolePriority[requiredRole]

	if !userOk {
		log.Printf("[WARN] IsRoleAtLeast: unknown user role %q (valid: member, manager, owner)", userRole)
		return false
	}
	if !reqOk {
		log.Printf("[WARN] IsRoleAtLeast: unknown required role %q (valid: member, manager, owner)", requiredRole)
		return false
	}

	return userPriority >= requiredPriority
}
