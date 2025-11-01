package services

import (
	"fmt"
	"strings"

	"soli/formations/src/auth/casdoor"
	"soli/formations/src/utils"

	"github.com/google/uuid"
)

// PermissionService provides centralized permission management
// All Casbin permission operations should go through this service
type PermissionService interface {
	// Entity permissions
	GrantEntityPermissions(userID string, entityType string, entityID uuid.UUID, methods []string) error
	RevokeEntityPermissions(userID string, entityType string, entityID uuid.UUID) error
	GrantEntityPermissionsWithPath(userID string, path string, methods []string) error
	RevokeEntityPermissionsWithPath(userID string, path string) error

	// Bulk operations
	GrantBulkPermissions(userID string, permissions []EntityPermission) error
	RevokeBulkPermissions(userID string, permissions []EntityPermission) error

	// Check permissions
	HasPermission(userID string, path string, method string) (bool, error)
	HasEntityPermission(userID string, entityType string, entityID uuid.UUID, method string) (bool, error)

	// Role-based permissions
	GrantRolePermissions(userID string, role string) error
	RevokeRolePermissions(userID string, role string) error
}

// EntityPermission represents a permission grant for a specific entity
type EntityPermission struct {
	EntityType string
	EntityID   uuid.UUID
	Methods    []string
}

type permissionService struct{}

// NewPermissionService creates a new permission service
func NewPermissionService() PermissionService {
	return &permissionService{}
}

// GrantEntityPermissions grants permissions for a specific entity
// entityType: "groups", "organizations", "terminals", etc.
// methods: ["GET", "POST", "PATCH", "DELETE"]
func (ps *permissionService) GrantEntityPermissions(
	userID string,
	entityType string,
	entityID uuid.UUID,
	methods []string,
) error {
	path := fmt.Sprintf("/api/v1/%s/%s", entityType, entityID)
	return ps.GrantEntityPermissionsWithPath(userID, path, methods)
}

// RevokeEntityPermissions revokes all permissions for a specific entity
func (ps *permissionService) RevokeEntityPermissions(
	userID string,
	entityType string,
	entityID uuid.UUID,
) error {
	path := fmt.Sprintf("/api/v1/%s/%s", entityType, entityID)
	return ps.RevokeEntityPermissionsWithPath(userID, path)
}

// GrantEntityPermissionsWithPath grants permissions using a custom path
func (ps *permissionService) GrantEntityPermissionsWithPath(
	userID string,
	path string,
	methods []string,
) error {
	if len(methods) == 0 {
		return utils.NewValidationError("methods", "no methods specified")
	}

	// Join methods with |
	methodStr := "(" + joinMethods(methods) + ")"

	// Add policy to Casbin
	opts := utils.DefaultPermissionOptions()
	err := utils.AddPolicy(casdoor.Enforcer, userID, path, methodStr, opts)
	if err != nil {
		utils.Error("Failed to grant permissions to user %s for path %s: %v", userID, path, err)
		return err
	}

	utils.Debug("Granted permissions to user %s for path %s: %s", userID, path, methodStr)
	return nil
}

// RevokeEntityPermissionsWithPath revokes permissions using a custom path
func (ps *permissionService) RevokeEntityPermissionsWithPath(
	userID string,
	path string,
) error {
	// Remove all policies for this user and path
	opts := utils.DefaultPermissionOptions()
	opts.WarnOnError = true

	err := utils.RemoveFilteredPolicy(casdoor.Enforcer, 0, opts, userID, path)
	if err != nil {
		return err
	}

	utils.Debug("Revoked permissions from user %s for path %s", userID, path)
	return nil
}

// GrantBulkPermissions grants multiple permissions at once
// Useful for granting permissions when adding a user to an organization
func (ps *permissionService) GrantBulkPermissions(
	userID string,
	permissions []EntityPermission,
) error {
	var errs utils.MultiError

	for _, perm := range permissions {
		err := ps.GrantEntityPermissions(userID, perm.EntityType, perm.EntityID, perm.Methods)
		if err != nil {
			errs.AddError(err)
		}
	}

	return errs.ToError()
}

// RevokeBulkPermissions revokes multiple permissions at once
func (ps *permissionService) RevokeBulkPermissions(
	userID string,
	permissions []EntityPermission,
) error {
	var errs utils.MultiError

	for _, perm := range permissions {
		err := ps.RevokeEntityPermissions(userID, perm.EntityType, perm.EntityID)
		if err != nil {
			errs.AddError(err)
		}
	}

	return errs.ToError()
}

// HasPermission checks if a user has a specific permission
func (ps *permissionService) HasPermission(
	userID string,
	path string,
	method string,
) (bool, error) {
	allowed, err := casdoor.Enforcer.Enforce(userID, path, method)
	if err != nil {
		return false, err
	}
	return allowed, nil
}

// HasEntityPermission checks if a user has permission for a specific entity
func (ps *permissionService) HasEntityPermission(
	userID string,
	entityType string,
	entityID uuid.UUID,
	method string,
) (bool, error) {
	path := fmt.Sprintf("/api/v1/%s/%s", entityType, entityID)
	return ps.HasPermission(userID, path, method)
}

// GrantRolePermissions grants all permissions associated with a role
// This is a placeholder for future role-based permission management
func (ps *permissionService) GrantRolePermissions(
	userID string,
	role string,
) error {
	// TODO: Implement role-based permission granting
	// This would look up all permissions associated with a role
	// and grant them to the user
	utils.Debug("GrantRolePermissions not yet implemented for role: %s", role)
	return nil
}

// RevokeRolePermissions revokes all permissions associated with a role
func (ps *permissionService) RevokeRolePermissions(
	userID string,
	role string,
) error {
	// TODO: Implement role-based permission revoking
	utils.Debug("RevokeRolePermissions not yet implemented for role: %s", role)
	return nil
}

// Helper functions

// joinMethods joins HTTP methods with |
func joinMethods(methods []string) string {
	return strings.Join(methods, "|")
}

// SplitMethods splits a method string like "GET|POST" into []string{"GET", "POST"}
func SplitMethods(methodStr string) []string {
	// Remove parentheses if present
	methodStr = strings.Trim(methodStr, "()")
	return strings.Split(methodStr, "|")
}

// Standard method sets for common permission patterns
var (
	MethodsRead      = []string{"GET"}
	MethodsWrite     = []string{"GET", "POST", "PATCH"}
	MethodsFull      = []string{"GET", "POST", "PATCH", "DELETE"}
	MethodsAdmin     = []string{"GET", "POST", "PATCH", "DELETE"}
	MethodsReadWrite = []string{"GET", "POST", "PATCH"}
	MethodsOwner     = []string{"GET", "POST", "PATCH", "DELETE"}
	MethodsMember    = []string{"GET", "POST"}
)

// GetStandardMethods returns standard method set by name
func GetStandardMethods(level string) []string {
	switch level {
	case "read":
		return MethodsRead
	case "write":
		return MethodsWrite
	case "admin":
		return MethodsAdmin
	case "full":
		return MethodsFull
	case "owner":
		return MethodsOwner
	case "member":
		return MethodsMember
	default:
		return MethodsRead
	}
}
