package utils

import (
	"fmt"

	"soli/formations/src/auth/interfaces"
)

// PermissionOptions configures permission operations
type PermissionOptions struct {
	// LoadPolicyFirst reloads policies from storage before operation
	LoadPolicyFirst bool
	// WarnOnError logs warning instead of returning error (useful for non-critical permissions)
	WarnOnError bool
}

// DefaultPermissionOptions returns sensible defaults
func DefaultPermissionOptions() PermissionOptions {
	return PermissionOptions{
		LoadPolicyFirst: false, // Most operations don't need reload
		WarnOnError:     false, // Default to returning errors
	}
}

// ==========================================
// Low-Level Permission Functions
// ==========================================

// AddPolicy adds a permission policy (subject can perform methods on route)
//
// Example:
//
//	AddPolicy(enforcer, "userId123", "/api/v1/groups/abc", "GET|POST", opts)
func AddPolicy(enforcer interfaces.EnforcerInterface, subject, route, methods string, opts PermissionOptions) error {
	if opts.LoadPolicyFirst {
		if err := enforcer.LoadPolicy(); err != nil {
			return fmt.Errorf("failed to load policy: %w", err)
		}
	}

	_, err := enforcer.AddPolicy(subject, route, methods)
	if err != nil {
		if opts.WarnOnError {
			Warn("Failed to add policy [%s, %s, %s]: %v", subject, route, methods, err)
			return nil
		}
		return fmt.Errorf("failed to add policy: %w", err)
	}

	return nil
}

// RemovePolicy removes a permission policy
//
// Example:
//
//	RemovePolicy(enforcer, "userId123", "/api/v1/groups/abc", "GET|POST", opts)
func RemovePolicy(enforcer interfaces.EnforcerInterface, subject, route, methods string, opts PermissionOptions) error {
	if opts.LoadPolicyFirst {
		if err := enforcer.LoadPolicy(); err != nil {
			return fmt.Errorf("failed to load policy: %w", err)
		}
	}

	_, err := enforcer.RemovePolicy(subject, route, methods)
	if err != nil {
		if opts.WarnOnError {
			Warn("Failed to remove policy [%s, %s, %s]: %v", subject, route, methods, err)
			return nil
		}
		return fmt.Errorf("failed to remove policy: %w", err)
	}

	return nil
}

// RemoveFilteredPolicy removes policies matching a filter pattern
//
// fieldIndex specifies which field to filter on:
//
//	0 = subject (user/role)
//	1 = route/object
//	2 = action/methods
//
// Example (remove all policies for a user):
//
//	RemoveFilteredPolicy(enforcer, 0, opts, "userId123")
//
// Example (remove all policies for a specific route):
//
//	RemoveFilteredPolicy(enforcer, 1, opts, "/api/v1/groups/abc")
//
// Example (remove all policies for a user on a specific route):
//
//	RemoveFilteredPolicy(enforcer, 0, opts, "userId123", "/api/v1/groups/abc")
func RemoveFilteredPolicy(enforcer interfaces.EnforcerInterface, fieldIndex int, opts PermissionOptions, fieldValues ...string) error {
	if opts.LoadPolicyFirst {
		if err := enforcer.LoadPolicy(); err != nil {
			return fmt.Errorf("failed to load policy: %w", err)
		}
	}

	_, err := enforcer.RemoveFilteredPolicy(fieldIndex, fieldValues...)
	if err != nil {
		if opts.WarnOnError {
			Warn("Failed to remove filtered policy (index=%d, values=%v): %v", fieldIndex, fieldValues, err)
			return nil
		}
		return fmt.Errorf("failed to remove filtered policy: %w", err)
	}

	return nil
}

// AddGroupingPolicy adds a user to a role group
//
// Example:
//
//	AddGroupingPolicy(enforcer, "userId123", "group:abc", opts)
func AddGroupingPolicy(enforcer interfaces.EnforcerInterface, userID, roleID string, opts PermissionOptions) error {
	if opts.LoadPolicyFirst {
		if err := enforcer.LoadPolicy(); err != nil {
			return fmt.Errorf("failed to load policy: %w", err)
		}
	}

	_, err := enforcer.AddGroupingPolicy(userID, roleID)
	if err != nil {
		if opts.WarnOnError {
			Warn("Failed to add user %s to role %s: %v", userID, roleID, err)
			return nil
		}
		return fmt.Errorf("failed to add user to role group: %w", err)
	}

	return nil
}

// RemoveGroupingPolicy removes a user from a role group
//
// Example:
//
//	RemoveGroupingPolicy(enforcer, "userId123", "group:abc", opts)
func RemoveGroupingPolicy(enforcer interfaces.EnforcerInterface, userID, roleID string, opts PermissionOptions) error {
	if opts.LoadPolicyFirst {
		if err := enforcer.LoadPolicy(); err != nil {
			return fmt.Errorf("failed to load policy: %w", err)
		}
	}

	_, err := enforcer.RemoveGroupingPolicy(userID, roleID)
	if err != nil {
		if opts.WarnOnError {
			Warn("Failed to remove user %s from role %s: %v", userID, roleID, err)
			return nil
		}
		return fmt.Errorf("failed to remove user from role group: %w", err)
	}

	return nil
}

// ==========================================
// High-Level Entity Permission Functions
// ==========================================

// GrantEntityAccess grants a user access to an entity (adds to role group and grants route permissions)
//
// Example:
//
//	GrantEntityAccess(enforcer, "userId123", "group", "abc-def-ghi", "GET|POST", opts)
//
// This will:
// 1. Add user to role group "group:abc-def-ghi"
// 2. Grant GET|POST permissions on "/api/v1/groups/abc-def-ghi" to the role
func GrantEntityAccess(enforcer interfaces.EnforcerInterface, userID, entityType, entityID, methods string, opts PermissionOptions) error {
	if opts.LoadPolicyFirst {
		if err := enforcer.LoadPolicy(); err != nil {
			return fmt.Errorf("failed to load policy: %w", err)
		}
	}

	// Create role ID (e.g., "group:abc-def-ghi")
	roleID := fmt.Sprintf("%s:%s", entityType, entityID)

	// Add user to role group
	if err := AddGroupingPolicy(enforcer, userID, roleID, PermissionOptions{LoadPolicyFirst: false, WarnOnError: opts.WarnOnError}); err != nil {
		return err
	}

	// Grant route access to role
	route := fmt.Sprintf("/api/v1/%ss/%s", entityType, entityID)
	if err := AddPolicy(enforcer, roleID, route, methods, PermissionOptions{LoadPolicyFirst: false, WarnOnError: opts.WarnOnError}); err != nil {
		return err
	}

	return nil
}

// RevokeEntityAccess revokes a user's access to an entity
//
// Example:
//
//	RevokeEntityAccess(enforcer, "userId123", "group", "abc-def-ghi", opts)
func RevokeEntityAccess(enforcer interfaces.EnforcerInterface, userID, entityType, entityID string, opts PermissionOptions) error {
	if opts.LoadPolicyFirst {
		if err := enforcer.LoadPolicy(); err != nil {
			return fmt.Errorf("failed to load policy: %w", err)
		}
	}

	roleID := fmt.Sprintf("%s:%s", entityType, entityID)

	// Remove user from role group
	if err := RemoveGroupingPolicy(enforcer, userID, roleID, PermissionOptions{LoadPolicyFirst: false, WarnOnError: opts.WarnOnError}); err != nil {
		return err
	}

	return nil
}

// GrantEntitySubResourceAccess grants access to a sub-resource of an entity
//
// Example:
//
//	GrantEntitySubResourceAccess(enforcer, "group:abc", "group", "abc", "members", "GET|POST", opts)
//
// This grants GET|POST on "/api/v1/groups/abc/members"
func GrantEntitySubResourceAccess(enforcer interfaces.EnforcerInterface, roleID, entityType, entityID, subResource, methods string, opts PermissionOptions) error {
	if opts.LoadPolicyFirst {
		if err := enforcer.LoadPolicy(); err != nil {
			return fmt.Errorf("failed to load policy: %w", err)
		}
	}

	route := fmt.Sprintf("/api/v1/%ss/%s/%s", entityType, entityID, subResource)
	if err := AddPolicy(enforcer, roleID, route, methods, PermissionOptions{LoadPolicyFirst: false, WarnOnError: opts.WarnOnError}); err != nil {
		return err
	}

	return nil
}

// GrantCompleteEntityAccess grants full access to an entity and common sub-resources
//
// Example:
//
//	GrantCompleteEntityAccess(enforcer, "userId123", "group", "abc-def-ghi", opts)
//
// This grants:
// - GET on /api/v1/groups/abc-def-ghi
// - GET on /api/v1/groups/abc-def-ghi/members
func GrantCompleteEntityAccess(enforcer interfaces.EnforcerInterface, userID, entityType, entityID string, subResources []string, opts PermissionOptions) error {
	if opts.LoadPolicyFirst {
		if err := enforcer.LoadPolicy(); err != nil {
			return fmt.Errorf("failed to load policy: %w", err)
		}
		// Don't reload again in sub-calls
		opts.LoadPolicyFirst = false
	}

	roleID := fmt.Sprintf("%s:%s", entityType, entityID)

	// Add user to role group
	if err := AddGroupingPolicy(enforcer, userID, roleID, opts); err != nil {
		return err
	}

	// Grant access to main entity route
	entityRoute := fmt.Sprintf("/api/v1/%ss/%s", entityType, entityID)
	if err := AddPolicy(enforcer, roleID, entityRoute, "GET", opts); err != nil {
		return err
	}

	// Grant access to sub-resources
	for _, subResource := range subResources {
		if err := GrantEntitySubResourceAccess(enforcer, roleID, entityType, entityID, subResource, "GET", opts); err != nil {
			return err
		}
	}

	return nil
}

// ==========================================
// Specialized Permission Functions
// ==========================================

// GrantManagerPermissions grants management permissions (GET|PATCH|DELETE on entity, full access to sub-resources)
//
// Example:
//
//	GrantManagerPermissions(enforcer, "userId123", "organization", "abc-def", []string{"members", "groups"}, opts)
func GrantManagerPermissions(enforcer interfaces.EnforcerInterface, userID, entityType, entityID string, manageableSubResources []string, opts PermissionOptions) error {
	if opts.LoadPolicyFirst {
		if err := enforcer.LoadPolicy(); err != nil {
			return fmt.Errorf("failed to load policy: %w", err)
		}
		opts.LoadPolicyFirst = false
	}

	managerRoleID := fmt.Sprintf("%s_manager:%s", entityType, entityID)

	// Add user to manager role
	if err := AddGroupingPolicy(enforcer, userID, managerRoleID, opts); err != nil {
		return err
	}

	// Grant management permissions on main entity
	entityRoute := fmt.Sprintf("/api/v1/%ss/%s", entityType, entityID)
	if err := AddPolicy(enforcer, managerRoleID, entityRoute, "GET|PATCH|DELETE", opts); err != nil {
		return err
	}

	// Grant full permissions on manageable sub-resources
	for _, subResource := range manageableSubResources {
		if err := GrantEntitySubResourceAccess(enforcer, managerRoleID, entityType, entityID, subResource, "GET|POST|PATCH|DELETE", opts); err != nil {
			return err
		}
	}

	return nil
}

// RevokeManagerPermissions revokes management permissions
//
// Example:
//
//	RevokeManagerPermissions(enforcer, "userId123", "organization", "abc-def", opts)
func RevokeManagerPermissions(enforcer interfaces.EnforcerInterface, userID, entityType, entityID string, opts PermissionOptions) error {
	if opts.LoadPolicyFirst {
		if err := enforcer.LoadPolicy(); err != nil {
			return fmt.Errorf("failed to load policy: %w", err)
		}
	}

	managerRoleID := fmt.Sprintf("%s_manager:%s", entityType, entityID)

	// Remove user from manager role
	if err := RemoveGroupingPolicy(enforcer, userID, managerRoleID, PermissionOptions{LoadPolicyFirst: false, WarnOnError: opts.WarnOnError}); err != nil {
		return err
	}

	return nil
}
