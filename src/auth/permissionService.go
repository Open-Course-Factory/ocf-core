package authController

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

	// Check permissions
	HasPermission(userID string, path string, method string) (bool, error)
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
	if len(methods) == 0 {
		return utils.NewValidationError("methods", "no methods specified")
	}

	path := fmt.Sprintf("/api/v1/%s/%s", entityType, entityID)
	// Join methods with |
	methodStr := "(" + strings.Join(methods, "|") + ")"

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

// RevokeEntityPermissions revokes all permissions for a specific entity
func (ps *permissionService) RevokeEntityPermissions(
	userID string,
	entityType string,
	entityID uuid.UUID,
) error {
	path := fmt.Sprintf("/api/v1/%s/%s", entityType, entityID)
	opts := utils.DefaultPermissionOptions()
	opts.WarnOnError = true

	// Remove all policies for this user and path
	err := utils.RemoveFilteredPolicy(casdoor.Enforcer, 0, opts, userID, path)
	if err != nil {
		return err
	}

	utils.Debug("Revoked permissions from user %s for path %s", userID, path)
	return nil
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
