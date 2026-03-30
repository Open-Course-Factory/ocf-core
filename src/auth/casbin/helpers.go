package casbin

// groupRolePriority maps group-level role names to their priority.
// Higher value = more permissions.
var groupRolePriority = map[string]int{
	"member":  10,
	"manager": 50,
	"owner":   100,
}

// IsRoleAtLeast checks whether userRole has at least the same privilege level
// as requiredRole within the group role hierarchy (member < manager < owner).
// Returns false if either role is unknown.
func IsRoleAtLeast(userRole, requiredRole string) bool {
	userPriority, userOk := groupRolePriority[userRole]
	requiredPriority, reqOk := groupRolePriority[requiredRole]

	if !userOk || !reqOk {
		return false
	}

	return userPriority >= requiredPriority
}
