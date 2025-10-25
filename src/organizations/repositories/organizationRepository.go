package repositories

import (
	"fmt"
	groupModels "soli/formations/src/groups/models"
	"soli/formations/src/organizations/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type OrganizationRepository interface {
	// Organization CRUD
	CreateOrganization(org *models.Organization) (*models.Organization, error)
	GetOrganizationByID(orgID uuid.UUID, includeRelations bool) (*models.Organization, error)
	GetOrganizationByName(name string) (*models.Organization, error)
	GetOrganizationByNameAndOwner(name, ownerUserID string) (*models.Organization, error)
	GetOrganizationsByOwner(ownerUserID string) (*[]models.Organization, error)
	GetOrganizationsByUserID(userID string) (*[]models.Organization, error)
	GetPersonalOrganization(userID string) (*models.Organization, error)
	UpdateOrganization(orgID uuid.UUID, updates map[string]interface{}) (*models.Organization, error)
	DeleteOrganization(orgID uuid.UUID) error

	// Organization Member CRUD
	AddOrganizationMember(member *models.OrganizationMember) error
	GetOrganizationMember(orgID uuid.UUID, userID string) (*models.OrganizationMember, error)
	GetOrganizationMembers(orgID uuid.UUID) (*[]models.OrganizationMember, error)
	UpdateOrganizationMemberRole(orgID uuid.UUID, userID string, role models.OrganizationMemberRole) error
	RemoveOrganizationMember(orgID uuid.UUID, userID string) error

	// Organization Groups
	GetOrganizationGroups(orgID uuid.UUID) (*[]groupModels.ClassGroup, error)
}

type organizationRepository struct {
	db *gorm.DB
}

func NewOrganizationRepository(db *gorm.DB) OrganizationRepository {
	return &organizationRepository{db: db}
}

// CreateOrganization creates a new organization
func (r *organizationRepository) CreateOrganization(org *models.Organization) (*models.Organization, error) {
	result := r.db.Create(org)
	if result.Error != nil {
		return nil, result.Error
	}
	return org, nil
}

// GetOrganizationByID retrieves an organization by ID
func (r *organizationRepository) GetOrganizationByID(orgID uuid.UUID, includeRelations bool) (*models.Organization, error) {
	var org models.Organization
	query := r.db.Where("id = ?", orgID)

	if includeRelations {
		query = query.Preload("Members").Preload("Groups")
	}

	result := query.First(&org)
	if result.Error != nil {
		return nil, result.Error
	}

	return &org, nil
}

// GetOrganizationByName retrieves an organization by name
func (r *organizationRepository) GetOrganizationByName(name string) (*models.Organization, error) {
	var org models.Organization
	result := r.db.Where("name = ?", name).First(&org)
	if result.Error != nil {
		return nil, result.Error
	}
	return &org, nil
}

// GetOrganizationByNameAndOwner retrieves an organization by name and owner
func (r *organizationRepository) GetOrganizationByNameAndOwner(name, ownerUserID string) (*models.Organization, error) {
	var org models.Organization
	result := r.db.Where("name = ? AND owner_user_id = ?", name, ownerUserID).First(&org)
	if result.Error != nil {
		return nil, result.Error
	}
	return &org, nil
}

// GetOrganizationsByOwner retrieves all organizations owned by a user
func (r *organizationRepository) GetOrganizationsByOwner(ownerUserID string) (*[]models.Organization, error) {
	var orgs []models.Organization
	result := r.db.Where("owner_user_id = ?", ownerUserID).Find(&orgs)
	if result.Error != nil {
		return nil, result.Error
	}
	return &orgs, nil
}

// GetOrganizationsByUserID retrieves all organizations a user is a member of
func (r *organizationRepository) GetOrganizationsByUserID(userID string) (*[]models.Organization, error) {
	var orgs []models.Organization

	// Get all organization IDs where user is a member
	result := r.db.Table("organizations").
		Joins("INNER JOIN organization_members ON organizations.id = organization_members.organization_id").
		Where("organization_members.user_id = ? AND organization_members.is_active = ?", userID, true).
		Find(&orgs)

	if result.Error != nil {
		return nil, result.Error
	}

	return &orgs, nil
}

// GetPersonalOrganization retrieves a user's personal organization
func (r *organizationRepository) GetPersonalOrganization(userID string) (*models.Organization, error) {
	var org models.Organization
	result := r.db.Where("owner_user_id = ? AND is_personal = ?", userID, true).First(&org)
	if result.Error != nil {
		return nil, result.Error
	}
	return &org, nil
}

// UpdateOrganization updates an organization
func (r *organizationRepository) UpdateOrganization(orgID uuid.UUID, updates map[string]interface{}) (*models.Organization, error) {
	var org models.Organization

	// Find the organization first
	if err := r.db.Where("id = ?", orgID).First(&org).Error; err != nil {
		return nil, err
	}

	// Apply updates
	if err := r.db.Model(&org).Updates(updates).Error; err != nil {
		return nil, err
	}

	// Reload with updates
	if err := r.db.Where("id = ?", orgID).First(&org).Error; err != nil {
		return nil, err
	}

	return &org, nil
}

// DeleteOrganization soft deletes an organization
func (r *organizationRepository) DeleteOrganization(orgID uuid.UUID) error {
	result := r.db.Delete(&models.Organization{}, "id = ?", orgID)
	return result.Error
}

// AddOrganizationMember adds a member to an organization
func (r *organizationRepository) AddOrganizationMember(member *models.OrganizationMember) error {
	// Check if member already exists
	var existing models.OrganizationMember
	result := r.db.Where("organization_id = ? AND user_id = ?", member.OrganizationID, member.UserID).First(&existing)

	if result.Error == nil {
		// Member exists, update it
		return r.db.Model(&existing).Updates(map[string]interface{}{
			"role":       member.Role,
			"is_active":  member.IsActive,
			"invited_by": member.InvitedBy,
			"joined_at":  member.JoinedAt,
		}).Error
	}

	// Member doesn't exist, create new
	return r.db.Create(member).Error
}

// GetOrganizationMember retrieves a specific member from an organization
func (r *organizationRepository) GetOrganizationMember(orgID uuid.UUID, userID string) (*models.OrganizationMember, error) {
	var member models.OrganizationMember
	result := r.db.Where("organization_id = ? AND user_id = ?", orgID, userID).First(&member)
	if result.Error != nil {
		return nil, result.Error
	}
	return &member, nil
}

// GetOrganizationMembers retrieves all members of an organization
func (r *organizationRepository) GetOrganizationMembers(orgID uuid.UUID) (*[]models.OrganizationMember, error) {
	var members []models.OrganizationMember
	result := r.db.Where("organization_id = ? AND is_active = ?", orgID, true).Find(&members)
	if result.Error != nil {
		return nil, result.Error
	}
	return &members, nil
}

// UpdateOrganizationMemberRole updates a member's role in an organization
func (r *organizationRepository) UpdateOrganizationMemberRole(orgID uuid.UUID, userID string, role models.OrganizationMemberRole) error {
	result := r.db.Model(&models.OrganizationMember{}).
		Where("organization_id = ? AND user_id = ?", orgID, userID).
		Update("role", role)
	return result.Error
}

// RemoveOrganizationMember soft deletes a member from an organization
func (r *organizationRepository) RemoveOrganizationMember(orgID uuid.UUID, userID string) error {
	result := r.db.Where("organization_id = ? AND user_id = ?", orgID, userID).
		Delete(&models.OrganizationMember{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("member not found")
	}
	return nil
}

// GetOrganizationGroups retrieves all groups belonging to an organization
func (r *organizationRepository) GetOrganizationGroups(orgID uuid.UUID) (*[]groupModels.ClassGroup, error) {
	var groups []groupModels.ClassGroup
	result := r.db.Where("organization_id = ?", orgID).Find(&groups)
	if result.Error != nil {
		return nil, result.Error
	}
	return &groups, nil
}
