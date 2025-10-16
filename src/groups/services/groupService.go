package services

import (
	"fmt"
	"time"

	"soli/formations/src/auth/casdoor"
	"soli/formations/src/groups/dto"
	"soli/formations/src/groups/models"
	"soli/formations/src/groups/repositories"
	"soli/formations/src/utils"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type GroupService interface {
	// Group management
	CreateGroup(userID string, input dto.CreateGroupInput) (*models.ClassGroup, error)
	GetGroup(groupID uuid.UUID, includeMembers bool) (*models.ClassGroup, error)
	GetUserGroups(userID string) (*[]models.ClassGroup, error)
	GetGroupsByOwner(ownerUserID string) (*[]models.ClassGroup, error)
	UpdateGroup(groupID uuid.UUID, ownerUserID string, updates map[string]interface{}) (*models.ClassGroup, error)
	DeleteGroup(groupID uuid.UUID, ownerUserID string) error

	// Member management
	AddMembersToGroup(groupID uuid.UUID, requestingUserID string, userIDs []string, role models.GroupMemberRole) error
	RemoveMemberFromGroup(groupID uuid.UUID, requestingUserID string, userID string) error
	UpdateMemberRole(groupID uuid.UUID, requestingUserID string, userID string, newRole models.GroupMemberRole) error
	GetGroupMembers(groupID uuid.UUID) (*[]models.GroupMember, error)
	IsUserInGroup(groupID uuid.UUID, userID string) (bool, error)
	GetUserGroupRole(groupID uuid.UUID, userID string) (models.GroupMemberRole, error)

	// Permissions
	CanUserManageGroup(groupID uuid.UUID, userID string) (bool, error)
	GrantGroupPermissionsToUser(userID string, groupID uuid.UUID) error
	RevokeGroupPermissionsFromUser(userID string, groupID uuid.UUID) error

	// Casdoor sync (optional)
	SyncGroupToCasdoor(groupID uuid.UUID) error
}

type groupService struct {
	repository repositories.GroupRepository
	db         *gorm.DB
}

func NewGroupService(db *gorm.DB) GroupService {
	return &groupService{
		repository: repositories.NewGroupRepository(db),
		db:         db,
	}
}

// CreateGroup creates a new group and automatically adds the creator as owner
func (gs *groupService) CreateGroup(userID string, input dto.CreateGroupInput) (*models.ClassGroup, error) {
	// Check if group name is unique for this user
	existingGroup, _ := gs.repository.GetGroupByNameAndOwner(input.Name, userID)
	if existingGroup != nil {
		return nil, fmt.Errorf("you already have a group with this name")
	}

	// Create group
	group := &models.ClassGroup{
		Name:               input.Name,
		DisplayName:        input.DisplayName,
		Description:        input.Description,
		OwnerUserID:        userID,
		SubscriptionPlanID: input.SubscriptionPlanID,
		MaxMembers:         input.MaxMembers,
		ExpiresAt:          input.ExpiresAt,
		Metadata:           input.Metadata,
		IsActive:           true,
	}

	if group.MaxMembers == 0 {
		group.MaxMembers = 50 // Default limit
	}

	createdGroup, err := gs.repository.CreateGroup(group)
	if err != nil {
		return nil, fmt.Errorf("failed to create group: %v", err)
	}

	// Automatically add creator as owner-member
	ownerMember := &models.GroupMember{
		GroupID:   createdGroup.ID,
		UserID:    userID,
		Role:      models.GroupMemberRoleOwner,
		InvitedBy: userID,
		JoinedAt:  time.Now(),
		IsActive:  true,
	}

	err = gs.repository.AddGroupMember(ownerMember)
	if err != nil {
		// Rollback group creation if adding owner fails
		gs.repository.DeleteGroup(createdGroup.ID)
		return nil, fmt.Errorf("failed to add owner to group: %v", err)
	}

	// Grant permissions to the owner
	err = gs.GrantGroupPermissionsToUser(userID, createdGroup.ID)
	if err != nil {
		utils.Warn("Failed to grant permissions to group owner: %v", err)
	}

	utils.Info("Group created: %s (ID: %s) by user %s", createdGroup.Name, createdGroup.ID, userID)
	return createdGroup, nil
}

// GetGroup retrieves a group by ID
func (gs *groupService) GetGroup(groupID uuid.UUID, includeMembers bool) (*models.ClassGroup, error) {
	return gs.repository.GetGroupByID(groupID, includeMembers)
}

// GetUserGroups returns all groups a user is a member of
func (gs *groupService) GetUserGroups(userID string) (*[]models.ClassGroup, error) {
	return gs.repository.GetGroupsByUserID(userID)
}

// GetGroupsByOwner returns all groups owned by a user
func (gs *groupService) GetGroupsByOwner(ownerUserID string) (*[]models.ClassGroup, error) {
	return gs.repository.GetGroupsByOwner(ownerUserID)
}

// UpdateGroup updates a group (only owner or admin can update)
func (gs *groupService) UpdateGroup(groupID uuid.UUID, requestingUserID string, updates map[string]interface{}) (*models.ClassGroup, error) {
	// Check if user can manage this group
	canManage, err := gs.CanUserManageGroup(groupID, requestingUserID)
	if err != nil {
		return nil, err
	}
	if !canManage {
		return nil, fmt.Errorf("you don't have permission to manage this group")
	}

	updatedGroup, err := gs.repository.UpdateGroup(groupID, updates)
	if err != nil {
		return nil, fmt.Errorf("failed to update group: %v", err)
	}

	return updatedGroup, nil
}

// DeleteGroup deletes a group (only owner can delete)
func (gs *groupService) DeleteGroup(groupID uuid.UUID, requestingUserID string) error {
	group, err := gs.repository.GetGroupByID(groupID, false)
	if err != nil {
		return err
	}

	// Only owner can delete
	if group.OwnerUserID != requestingUserID {
		return fmt.Errorf("only the group owner can delete the group")
	}

	// Revoke all member permissions
	members, err := gs.repository.GetGroupMembers(groupID)
	if err == nil && members != nil {
		for _, member := range *members {
			gs.RevokeGroupPermissionsFromUser(member.UserID, groupID)
		}
	}

	return gs.repository.DeleteGroup(groupID)
}

// AddMembersToGroup adds multiple members to a group
func (gs *groupService) AddMembersToGroup(groupID uuid.UUID, requestingUserID string, userIDs []string, role models.GroupMemberRole) error {
	// Check if user can manage this group
	canManage, err := gs.CanUserManageGroup(groupID, requestingUserID)
	if err != nil {
		return err
	}
	if !canManage {
		return fmt.Errorf("you don't have permission to add members to this group")
	}

	// Get group to check limits
	group, err := gs.repository.GetGroupByID(groupID, true)
	if err != nil {
		return err
	}

	// Check if group is full
	if group.MaxMembers > 0 && len(group.Members)+len(userIDs) > group.MaxMembers {
		return fmt.Errorf("group is full (max %d members)", group.MaxMembers)
	}

	// Check if group is expired
	if group.IsExpired() {
		return fmt.Errorf("group has expired")
	}

	// Add each member
	for _, userID := range userIDs {
		// Check if already a member
		isMember, _ := gs.IsUserInGroup(groupID, userID)
		if isMember {
			utils.Warn("User %s is already a member of group %s", userID, groupID)
			continue
		}

		member := &models.GroupMember{
			GroupID:   groupID,
			UserID:    userID,
			Role:      role,
			InvitedBy: requestingUserID,
			JoinedAt:  time.Now(),
			IsActive:  true,
		}

		err = gs.repository.AddGroupMember(member)
		if err != nil {
			utils.Error("Failed to add user %s to group %s: %v", userID, groupID, err)
			continue
		}

		// Grant group permissions to new member
		err = gs.GrantGroupPermissionsToUser(userID, groupID)
		if err != nil {
			utils.Warn("Failed to grant permissions to user %s: %v", userID, err)
		}

		utils.Info("User %s added to group %s with role %s", userID, groupID, role)
	}

	return nil
}

// RemoveMemberFromGroup removes a member from a group
func (gs *groupService) RemoveMemberFromGroup(groupID uuid.UUID, requestingUserID string, userID string) error {
	// Check if user can manage this group
	canManage, err := gs.CanUserManageGroup(groupID, requestingUserID)
	if err != nil {
		return err
	}
	if !canManage {
		return fmt.Errorf("you don't have permission to remove members from this group")
	}

	// Get group
	group, err := gs.repository.GetGroupByID(groupID, false)
	if err != nil {
		return err
	}

	// Cannot remove the owner
	if group.OwnerUserID == userID {
		return fmt.Errorf("cannot remove the group owner")
	}

	// Remove member
	err = gs.repository.RemoveGroupMember(groupID, userID)
	if err != nil {
		return fmt.Errorf("failed to remove member: %v", err)
	}

	// Revoke permissions
	err = gs.RevokeGroupPermissionsFromUser(userID, groupID)
	if err != nil {
		utils.Warn("Failed to revoke permissions from user %s: %v", userID, err)
	}

	utils.Info("User %s removed from group %s", userID, groupID)
	return nil
}

// UpdateMemberRole updates a member's role in a group
func (gs *groupService) UpdateMemberRole(groupID uuid.UUID, requestingUserID string, userID string, newRole models.GroupMemberRole) error {
	// Check if user can manage this group
	canManage, err := gs.CanUserManageGroup(groupID, requestingUserID)
	if err != nil {
		return err
	}
	if !canManage {
		return fmt.Errorf("you don't have permission to update roles in this group")
	}

	// Get group
	group, err := gs.repository.GetGroupByID(groupID, false)
	if err != nil {
		return err
	}

	// Cannot change owner role
	if group.OwnerUserID == userID && newRole != models.GroupMemberRoleOwner {
		return fmt.Errorf("cannot change the owner's role")
	}

	// Update role
	err = gs.repository.UpdateGroupMemberRole(groupID, userID, newRole)
	if err != nil {
		return fmt.Errorf("failed to update member role: %v", err)
	}

	utils.Info("User %s role updated to %s in group %s", userID, newRole, groupID)
	return nil
}

// GetGroupMembers returns all members of a group
func (gs *groupService) GetGroupMembers(groupID uuid.UUID) (*[]models.GroupMember, error) {
	return gs.repository.GetGroupMembers(groupID)
}

// IsUserInGroup checks if a user is a member of a group
func (gs *groupService) IsUserInGroup(groupID uuid.UUID, userID string) (bool, error) {
	member, err := gs.repository.GetGroupMember(groupID, userID)
	if err != nil || member == nil {
		return false, nil
	}
	return member.IsActive, nil
}

// GetUserGroupRole returns the user's role in a group
func (gs *groupService) GetUserGroupRole(groupID uuid.UUID, userID string) (models.GroupMemberRole, error) {
	member, err := gs.repository.GetGroupMember(groupID, userID)
	if err != nil || member == nil {
		return "", fmt.Errorf("user is not a member of this group")
	}
	return member.Role, nil
}

// CanUserManageGroup checks if a user can manage a group (owner or admin)
func (gs *groupService) CanUserManageGroup(groupID uuid.UUID, userID string) (bool, error) {
	group, err := gs.repository.GetGroupByID(groupID, false)
	if err != nil {
		return false, err
	}

	// Owner can always manage
	if group.OwnerUserID == userID {
		return true, nil
	}

	// Check if user is an admin member
	member, err := gs.repository.GetGroupMember(groupID, userID)
	if err != nil || member == nil {
		return false, nil
	}

	return member.IsAdmin(), nil
}

// GrantGroupPermissionsToUser grants group-related permissions to a user via Casbin
func (gs *groupService) GrantGroupPermissionsToUser(userID string, groupID uuid.UUID) error {
	// Add user to group in Casbin (for permission checks)
	// Format: group:{groupID} as a role
	groupRoleID := fmt.Sprintf("group:%s", groupID.String())

	_, err := casdoor.Enforcer.AddGroupingPolicy(userID, groupRoleID)
	if err != nil {
		return fmt.Errorf("failed to add user to group role: %v", err)
	}

	// Grant permissions to access group resources
	// Group members can view the group
	_, err = casdoor.Enforcer.AddPolicy(groupRoleID, fmt.Sprintf("/api/v1/groups/%s", groupID), "GET")
	if err != nil {
		utils.Warn("Failed to add GET permission for group %s: %v", groupID, err)
	}

	// Group members can view group members
	_, err = casdoor.Enforcer.AddPolicy(groupRoleID, fmt.Sprintf("/api/v1/groups/%s/members", groupID), "GET")
	if err != nil {
		utils.Warn("Failed to add GET members permission for group %s: %v", groupID, err)
	}

	utils.Debug("Granted group permissions to user %s for group %s", userID, groupID)
	return nil
}

// RevokeGroupPermissionsFromUser revokes group permissions from a user
func (gs *groupService) RevokeGroupPermissionsFromUser(userID string, groupID uuid.UUID) error {
	groupRoleID := fmt.Sprintf("group:%s", groupID.String())

	_, err := casdoor.Enforcer.RemoveGroupingPolicy(userID, groupRoleID)
	if err != nil {
		return fmt.Errorf("failed to remove user from group role: %v", err)
	}

	utils.Debug("Revoked group permissions from user %s for group %s", userID, groupID)
	return nil
}

// SyncGroupToCasdoor syncs a group to Casdoor (optional feature)
func (gs *groupService) SyncGroupToCasdoor(groupID uuid.UUID) error {
	// TODO: Implement Casdoor group synchronization if needed
	// This is optional and can be implemented later
	return fmt.Errorf("casdoor sync not yet implemented")
}
