package services

import (
	"fmt"
	"time"

	"soli/formations/src/utils"

	"github.com/google/uuid"
)

// PermissionManager interface to avoid import cycles
// Implement this in your permission service
type PermissionManager interface {
	GrantEntityPermissions(userID string, entityType string, entityID uuid.UUID, methods []string) error
	RevokeEntityPermissions(userID string, entityType string, entityID uuid.UUID) error
}

// MemberManagementService provides generic member management operations
// This reduces code duplication between Groups and Organizations
type MemberManagementService interface {
	AddMembers(entityID uuid.UUID, requestingUserID string, userIDs []string, role string) error
	RemoveMember(entityID uuid.UUID, requestingUserID string, userID string) error
	UpdateMemberRole(entityID uuid.UUID, requestingUserID string, userID string, newRole string) error
	GetMembers(entityID uuid.UUID) ([]any, error)
	IsUserMember(entityID uuid.UUID, userID string) (bool, error)
	GetUserRole(entityID uuid.UUID, userID string) (string, error)
}

// MemberEntity represents an entity that can have members (Group, Organization, etc.)
type MemberEntity interface {
	GetID() uuid.UUID
	GetOwnerUserID() string
	GetMaxMembers() int
	GetCurrentMemberCount() int
	IsExpired() bool
	IsActive() bool
}

// Member represents a member of an entity
type Member interface {
	GetUserID() string
	GetRole() string
	GetJoinedAt() time.Time
	IsActive() bool
}

// MemberRepository defines operations for member management
type MemberRepository interface {
	// Entity operations
	GetEntityByID(entityID uuid.UUID) (MemberEntity, error)

	// Member operations
	AddMember(member Member) error
	RemoveMember(entityID uuid.UUID, userID string) error
	UpdateMemberRole(entityID uuid.UUID, userID string, newRole string) error
	GetMember(entityID uuid.UUID, userID string) (Member, error)
	GetMembers(entityID uuid.UUID) ([]Member, error)
}

// MemberConfig contains configuration for member management
type MemberConfig struct {
	EntityType      string   // "group", "organization", etc.
	RoleOwner       string   // "owner"
	RoleManager     string   // "manager", "admin"
	AllowedRoles    []string // List of allowed roles
	PermissionPaths []string // Custom permission paths to grant
}

type memberManagementService struct {
	repository        MemberRepository
	permissionService PermissionManager
	config            MemberConfig
}

// NewMemberManagementService creates a new member management service
func NewMemberManagementService(
	repository MemberRepository,
	permissionService PermissionManager,
	config MemberConfig,
) MemberManagementService {
	return &memberManagementService{
		repository:        repository,
		permissionService: permissionService,
		config:            config,
	}
}

// AddMembers adds multiple users as members to an entity
func (mms *memberManagementService) AddMembers(
	entityID uuid.UUID,
	requestingUserID string,
	userIDs []string,
	role string,
) error {
	// Get entity
	entity, err := mms.repository.GetEntityByID(entityID)
	if err != nil {
		return utils.ErrEntityNotFound(mms.config.EntityType, entityID)
	}

	// Check if requesting user can manage this entity
	canManage, err := mms.canUserManage(entityID, requestingUserID)
	if err != nil {
		return err
	}
	if !canManage {
		return utils.ErrPermissionDenied(mms.config.EntityType, "add members to")
	}

	// Validate entity is active and not expired
	if !entity.IsActive() {
		return utils.ErrEntityInactive(mms.config.EntityType, entityID)
	}
	if entity.IsExpired() {
		return utils.ErrEntityExpired(mms.config.EntityType, entityID)
	}

	// Validate role
	if !mms.isValidRole(role) {
		return utils.ErrInvalidRole(mms.config.EntityType, role)
	}

	// Check member limit
	maxMembers := entity.GetMaxMembers()
	currentMembers := entity.GetCurrentMemberCount()
	if maxMembers != -1 && currentMembers+len(userIDs) > maxMembers {
		return utils.CapacityWillExceedError(mms.config.EntityType, currentMembers, len(userIDs), maxMembers)
	}

	// Add each user
	var multiErr utils.MultiError
	for _, userID := range userIDs {
		// Check if already a member
		isMember, _ := mms.IsUserMember(entityID, userID)
		if isMember {
			multiErr.AddError(fmt.Errorf("user %s is already a member", userID))
			continue
		}

		// Create member (implementation depends on specific member type)
		// This would need to be handled by a factory or builder pattern
		// For now, we'll use the repository directly

		// Grant permissions
		err = mms.grantMemberPermissions(userID, entityID, role)
		if err != nil {
			utils.Warn("Failed to grant permissions to user %s: %v", userID, err)
			multiErr.AddError(err)
		}

		utils.Info("User %s added to %s %s with role %s", userID, mms.config.EntityType, entityID, role)
	}

	return multiErr.ToError()
}

// RemoveMember removes a user from an entity
func (mms *memberManagementService) RemoveMember(
	entityID uuid.UUID,
	requestingUserID string,
	userID string,
) error {
	// Get entity
	entity, err := mms.repository.GetEntityByID(entityID)
	if err != nil {
		return utils.ErrEntityNotFound(mms.config.EntityType, entityID)
	}

	// Check permissions
	canManage, err := mms.canUserManage(entityID, requestingUserID)
	if err != nil {
		return err
	}
	if !canManage {
		return utils.ErrPermissionDenied(mms.config.EntityType, "remove members from")
	}

	// Cannot remove the owner
	if entity.GetOwnerUserID() == userID {
		return utils.ErrCannotRemoveOwner(mms.config.EntityType)
	}

	// Remove member
	err = mms.repository.RemoveMember(entityID, userID)
	if err != nil {
		return fmt.Errorf("failed to remove member: %w", err)
	}

	// Revoke permissions
	err = mms.revokeMemberPermissions(userID, entityID)
	if err != nil {
		utils.Warn("Failed to revoke permissions from user %s: %v", userID, err)
	}

	utils.Info("User %s removed from %s %s", userID, mms.config.EntityType, entityID)
	return nil
}

// UpdateMemberRole updates a member's role
func (mms *memberManagementService) UpdateMemberRole(
	entityID uuid.UUID,
	requestingUserID string,
	userID string,
	newRole string,
) error {
	// Get entity
	entity, err := mms.repository.GetEntityByID(entityID)
	if err != nil {
		return utils.ErrEntityNotFound(mms.config.EntityType, entityID)
	}

	// Check permissions
	canManage, err := mms.canUserManage(entityID, requestingUserID)
	if err != nil {
		return err
	}
	if !canManage {
		return utils.ErrPermissionDenied(mms.config.EntityType, "update roles in")
	}

	// Validate role
	if !mms.isValidRole(newRole) {
		return utils.ErrInvalidRole(mms.config.EntityType, newRole)
	}

	// Cannot change owner role
	if entity.GetOwnerUserID() == userID && newRole != mms.config.RoleOwner {
		return utils.ErrCannotModifyOwner(mms.config.EntityType)
	}

	// Update role
	err = mms.repository.UpdateMemberRole(entityID, userID, newRole)
	if err != nil {
		return fmt.Errorf("failed to update member role: %w", err)
	}

	// Update permissions based on new role
	err = mms.updateMemberPermissions(userID, entityID, newRole)
	if err != nil {
		utils.Warn("Failed to update permissions for user %s: %v", userID, err)
	}

	utils.Info("User %s role updated to %s in %s %s", userID, newRole, mms.config.EntityType, entityID)
	return nil
}

// GetMembers returns all members of an entity
func (mms *memberManagementService) GetMembers(entityID uuid.UUID) ([]any, error) {
	members, err := mms.repository.GetMembers(entityID)
	if err != nil {
		return nil, err
	}

	// Convert to []any
	result := make([]any, len(members))
	for i, m := range members {
		result[i] = m
	}

	return result, nil
}

// IsUserMember checks if a user is a member of an entity
func (mms *memberManagementService) IsUserMember(entityID uuid.UUID, userID string) (bool, error) {
	member, err := mms.repository.GetMember(entityID, userID)
	if err != nil || member == nil {
		return false, nil
	}
	return member.IsActive(), nil
}

// GetUserRole returns a user's role in an entity
func (mms *memberManagementService) GetUserRole(entityID uuid.UUID, userID string) (string, error) {
	member, err := mms.repository.GetMember(entityID, userID)
	if err != nil || member == nil {
		return "", utils.ErrMemberNotFound(mms.config.EntityType, userID)
	}
	return member.GetRole(), nil
}

// Helper methods

func (mms *memberManagementService) canUserManage(entityID uuid.UUID, userID string) (bool, error) {
	entity, err := mms.repository.GetEntityByID(entityID)
	if err != nil {
		return false, err
	}

	// Owner can always manage
	if entity.GetOwnerUserID() == userID {
		return true, nil
	}

	// Check if user is a manager/admin
	member, err := mms.repository.GetMember(entityID, userID)
	if err != nil || member == nil {
		return false, nil
	}

	role := member.GetRole()
	return role == mms.config.RoleManager || role == mms.config.RoleOwner, nil
}

func (mms *memberManagementService) isValidRole(role string) bool {
	for _, allowed := range mms.config.AllowedRoles {
		if role == allowed {
			return true
		}
	}
	return false
}

func (mms *memberManagementService) grantMemberPermissions(userID string, entityID uuid.UUID, role string) error {
	// Determine methods based on role
	var methods []string
	if role == mms.config.RoleManager || role == mms.config.RoleOwner {
		methods = []string{"GET", "POST", "PATCH", "DELETE"} // Full access
	} else {
		methods = []string{"GET", "POST"} // Member access
	}

	// Grant entity permissions
	return mms.permissionService.GrantEntityPermissions(
		userID,
		mms.config.EntityType+"s", // pluralize
		entityID,
		methods,
	)
}

func (mms *memberManagementService) revokeMemberPermissions(userID string, entityID uuid.UUID) error {
	return mms.permissionService.RevokeEntityPermissions(
		userID,
		mms.config.EntityType+"s", // pluralize
		entityID,
	)
}

func (mms *memberManagementService) updateMemberPermissions(userID string, entityID uuid.UUID, newRole string) error {
	// Revoke old permissions
	err := mms.revokeMemberPermissions(userID, entityID)
	if err != nil {
		return err
	}

	// Grant new permissions
	return mms.grantMemberPermissions(userID, entityID, newRole)
}
