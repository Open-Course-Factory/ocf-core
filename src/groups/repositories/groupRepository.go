package repositories

import (
	"fmt"
	"soli/formations/src/groups/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type GroupRepository interface {
	// Group operations
	CreateGroup(group *models.ClassGroup) (*models.ClassGroup, error)
	GetGroupByID(groupID uuid.UUID, includeMembers bool) (*models.ClassGroup, error)
	GetGroupByNameAndOwner(name, ownerUserID string) (*models.ClassGroup, error)
	GetGroupsByUserID(userID string) (*[]models.ClassGroup, error)
	GetGroupsByOwner(ownerUserID string) (*[]models.ClassGroup, error)
	GetGroupsByOrganization(organizationID uuid.UUID, includes []string) (*[]models.ClassGroup, error)
	UpdateGroup(groupID uuid.UUID, updates map[string]any) (*models.ClassGroup, error)
	DeleteGroup(groupID uuid.UUID) error

	// Group member operations
	AddGroupMember(member *models.GroupMember) error
	GetGroupMember(groupID uuid.UUID, userID string) (*models.GroupMember, error)
	GetGroupMembers(groupID uuid.UUID) (*[]models.GroupMember, error)
	RemoveGroupMember(groupID uuid.UUID, userID string) error
	UpdateGroupMemberRole(groupID uuid.UUID, userID string, role models.GroupMemberRole) error
}

type groupRepository struct {
	db *gorm.DB
}

func NewGroupRepository(db *gorm.DB) GroupRepository {
	return &groupRepository{db: db}
}

// CreateGroup creates a new group
func (gr *groupRepository) CreateGroup(group *models.ClassGroup) (*models.ClassGroup, error) {
	err := gr.db.Create(group).Error
	if err != nil {
		return nil, err
	}
	return group, nil
}

// GetGroupByID retrieves a group by ID
func (gr *groupRepository) GetGroupByID(groupID uuid.UUID, includeMembers bool) (*models.ClassGroup, error) {
	var group models.ClassGroup
	query := gr.db.Where("id = ?", groupID)

	if includeMembers {
		query = query.Preload("Members")
	}

	err := query.First(&group).Error
	if err != nil {
		return nil, err
	}
	return &group, nil
}

// GetGroupByNameAndOwner retrieves a group by name and owner
func (gr *groupRepository) GetGroupByNameAndOwner(name, ownerUserID string) (*models.ClassGroup, error) {
	var group models.ClassGroup
	err := gr.db.Where("name = ? AND owner_user_id = ?", name, ownerUserID).First(&group).Error
	if err != nil {
		return nil, err
	}
	return &group, nil
}

// GetGroupsByUserID returns all groups a user is a member of
func (gr *groupRepository) GetGroupsByUserID(userID string) (*[]models.ClassGroup, error) {
	var groups []models.ClassGroup

	// Join with group_members to get all groups the user is in
	err := gr.db.
		Joins("JOIN group_members ON group_members.group_id = class_groups.id").
		Where("group_members.user_id = ? AND group_members.is_active = ?", userID, true).
		Preload("Members").
		Find(&groups).Error

	if err != nil {
		return nil, err
	}
	return &groups, nil
}

// GetGroupsByOwner returns all groups owned by a user
func (gr *groupRepository) GetGroupsByOwner(ownerUserID string) (*[]models.ClassGroup, error) {
	var groups []models.ClassGroup
	err := gr.db.Where("owner_user_id = ?", ownerUserID).Preload("Members").Find(&groups).Error
	if err != nil {
		return nil, err
	}
	return &groups, nil
}

// GetGroupsByOrganization returns all groups belonging to an organization
func (gr *groupRepository) GetGroupsByOrganization(organizationID uuid.UUID, includes []string) (*[]models.ClassGroup, error) {
	var groups []models.ClassGroup
	query := gr.db.Where("organization_id = ?", organizationID)

	// Always preload Members for accurate member_count (but only active members)
	query = query.Preload("Members", "is_active = ?", true)

	// Handle additional selective preloading
	for _, include := range includes {
		// Skip Members since we already preloaded it above
		if include != "Members" {
			query = query.Preload(include)
		}
	}

	err := query.Find(&groups).Error
	if err != nil {
		return nil, err
	}
	return &groups, nil
}

// UpdateGroup updates a group
func (gr *groupRepository) UpdateGroup(groupID uuid.UUID, updates map[string]any) (*models.ClassGroup, error) {
	var group models.ClassGroup
	err := gr.db.Model(&group).Where("id = ?", groupID).Updates(updates).Error
	if err != nil {
		return nil, err
	}

	// Fetch the updated group
	return gr.GetGroupByID(groupID, false)
}

// DeleteGroup deletes a group (cascade will delete members)
func (gr *groupRepository) DeleteGroup(groupID uuid.UUID) error {
	return gr.db.Where("id = ?", groupID).Delete(&models.ClassGroup{}).Error
}

// AddGroupMember adds a member to a group
func (gr *groupRepository) AddGroupMember(member *models.GroupMember) error {
	return gr.db.Create(member).Error
}

// GetGroupMember retrieves a specific group member
func (gr *groupRepository) GetGroupMember(groupID uuid.UUID, userID string) (*models.GroupMember, error) {
	var member models.GroupMember
	err := gr.db.Where("group_id = ? AND user_id = ?", groupID, userID).First(&member).Error
	if err != nil {
		return nil, err
	}
	return &member, nil
}

// GetGroupMembers returns all members of a group
func (gr *groupRepository) GetGroupMembers(groupID uuid.UUID) (*[]models.GroupMember, error) {
	var members []models.GroupMember
	err := gr.db.Where("group_id = ? AND is_active = ?", groupID, true).Find(&members).Error
	if err != nil {
		return nil, err
	}
	return &members, nil
}

// RemoveGroupMember removes a member from a group (soft delete by setting is_active = false)
func (gr *groupRepository) RemoveGroupMember(groupID uuid.UUID, userID string) error {
	return gr.db.Model(&models.GroupMember{}).
		Where("group_id = ? AND user_id = ?", groupID, userID).
		Update("is_active", false).Error
}

// UpdateGroupMemberRole updates a member's role in a group
func (gr *groupRepository) UpdateGroupMemberRole(groupID uuid.UUID, userID string, role models.GroupMemberRole) error {
	result := gr.db.Model(&models.GroupMember{}).
		Where("group_id = ? AND user_id = ?", groupID, userID).
		Update("role", role)

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("group member not found")
	}

	return nil
}
