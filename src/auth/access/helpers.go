package access

import (
	"log"
	"sort"
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

// Contextual role names, shared by organizations and groups. Named constants
// rather than literals so a role can be found by its identifier instead of by
// grepping for a string, and so the hierarchy below cannot disagree with the
// callers that test against it.
//
// The lowest rung reuses RoleMember from the platform-role block: the literal is
// the same "member", even though the two blocks answer different questions
// (which platform gateway role you hold, versus your rank inside one
// organization or group).
const (
	RoleTeacher = "teacher"
	RoleManager = "manager"
	RoleOwner   = "owner"
)

// roleHierarchy maps role names to their priority for group and organization
// hierarchies. Higher value = more permissions. Call RegisterRole() to add or
// override roles.
//
// This is the ONLY hierarchy. Two hardcoded copies used to shadow it, on
// OrganizationMember and GroupMember, each with its own switch and a `default: 0`
// — so a role registered here ranked BELOW member in those, silently. They now
// delegate to RolePriority (#460).
var roleHierarchy = map[string]int{
	RoleMember:  10,
	RoleTeacher: 30,
	RoleManager: 50,
	RoleOwner:   100,
}

// RoleMinimumForClassrooms is the lowest role that may create and run classes
// inside an organization.
//
// Named so the threshold is one edit rather than a `manager` literal scattered
// across hooks, registrations and tests. It sits at teacher because running a
// class and administering the organization — members, billing, backends — are
// different jobs: a school hands out the first far more freely than the second.
const RoleMinimumForClassrooms = RoleTeacher

// RolePriority returns a role's priority, or 0 for an unknown role.
//
// Exported so model-level helpers can rank roles through the single hierarchy
// instead of restating it.
func RolePriority(role string) int {
	return roleHierarchy[role]
}

// IsKnownRole reports whether a role exists in the hierarchy.
//
// Validation callers use this instead of their own allowlist, so a role added
// here is accepted everywhere the day it exists — the org-membership CSV import
// carried its own {member,manager,owner} literal and would have rejected any new
// role outright (#460).
func IsKnownRole(role string) bool {
	_, ok := roleHierarchy[role]
	return ok
}

// KnownRolesForMessage renders the valid roles lowest-privilege first, for error
// messages that tell a user what they may write.
func KnownRolesForMessage() string {
	names := make([]string, 0, len(roleHierarchy))
	for name := range roleHierarchy {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool { return roleHierarchy[names[i]] < roleHierarchy[names[j]] })
	return strings.Join(names, ", ")
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
