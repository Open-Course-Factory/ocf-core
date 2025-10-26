package filters

import (
	"gorm.io/gorm"
)

// OrganizationMemberFilter handles filtering organizations by user membership
// This filter ensures users only see organizations they are members of
type OrganizationMemberFilter struct{}

// Priority returns the priority of this filter strategy
func (f *OrganizationMemberFilter) Priority() int {
	return 5 // Higher priority than standard filters
}

// Matches checks if this strategy should handle the given filter
func (f *OrganizationMemberFilter) Matches(key string, value interface{}) bool {
	return key == "user_member_id"
}

// Apply applies the user membership filter to the query
// This joins with organization_members and filters by user_id
func (f *OrganizationMemberFilter) Apply(
	query *gorm.DB,
	key string,
	value interface{},
	tableName string,
) *gorm.DB {
	userID, ok := value.(string)
	if !ok {
		return query
	}

	// Join with organization_members and filter by user_id
	return query.
		Joins("INNER JOIN organization_members ON organization_members.organization_id = organizations.id").
		Where("organization_members.user_id = ?", userID).
		Where("organization_members.is_active = ?", true).
		Distinct() // Ensure no duplicate organizations
}
