package filters

import (
	"fmt"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"strings"

	"gorm.io/gorm"
)

// GenericMembershipFilter filters entities based on user membership using configurable settings
// This replaces entity-specific filters (OrganizationMemberFilter, GroupMemberFilter) with a generic approach
type GenericMembershipFilter struct {
	config *entityManagementInterfaces.MembershipConfig
}

// NewGenericMembershipFilter creates a new filter with the given membership configuration
func NewGenericMembershipFilter(config *entityManagementInterfaces.MembershipConfig) *GenericMembershipFilter {
	return &GenericMembershipFilter{config: config}
}

// Priority returns the priority of this filter strategy
func (f *GenericMembershipFilter) Priority() int {
	return 5 // Higher priority than standard filters
}

// Matches checks if this strategy should handle the given filter
func (f *GenericMembershipFilter) Matches(key string, value any) bool {
	return key == "user_member_id"
}

// Apply applies the membership filter to the query
// This filters entities to show only those the user has access to through direct membership
// or through organization management (if OrgAccessEnabled is true)
func (f *GenericMembershipFilter) Apply(
	query *gorm.DB,
	key string,
	value any,
	tableName string,
) *gorm.DB {
	userID, ok := value.(string)
	if !ok {
		return query
	}

	if f.config == nil {
		return query
	}

	// Default to "is_active" if not specified
	isActiveColumn := f.config.IsActiveColumn
	if isActiveColumn == "" {
		isActiveColumn = "is_active"
	}

	var directMembershipCondition string

	// Special case: When the entity table IS the membership table
	// (e.g., listing GroupMember or OrganizationMember entities)
	if tableName == f.config.MemberTable {
		// For membership entities, show records where the user is a member of the same group/org
		// Example: Show GroupMember records where group_id matches groups the user belongs to
		directMembershipCondition = fmt.Sprintf(`
			EXISTS (
				SELECT 1 FROM %s membership_check
				WHERE membership_check.%s = %s.%s
				AND membership_check.%s = ?
				AND membership_check.%s = true
			)`,
			f.config.MemberTable,
			f.config.EntityIDColumn, tableName, f.config.EntityIDColumn,
			f.config.UserIDColumn,
			isActiveColumn,
		)
	} else {
		// Normal case: Entity table is different from membership table
		// Example: Show ClassGroup records where the user is a member
		directMembershipCondition = fmt.Sprintf(`
			EXISTS (
				SELECT 1 FROM %s
				WHERE %s.%s = %s.id
				AND %s.%s = ?
				AND %s.%s = true
			)`,
			f.config.MemberTable,
			f.config.MemberTable, f.config.EntityIDColumn, tableName,
			f.config.MemberTable, f.config.UserIDColumn,
			f.config.MemberTable, isActiveColumn,
		)
	}

	args := []interface{}{userID}

	// If organization access is enabled and manager roles are specified,
	// add organization-based access condition
	if f.config.OrgAccessEnabled && len(f.config.ManagerRoles) > 0 {
		// Build placeholders for IN clause
		placeholders := make([]string, len(f.config.ManagerRoles))
		for i := range f.config.ManagerRoles {
			placeholders[i] = "?"
		}
		rolesPlaceholder := strings.Join(placeholders, ", ")

		orgAccessCondition := fmt.Sprintf(`
			EXISTS (
				SELECT 1 FROM organization_members om
				WHERE om.organization_id = %s.organization_id
				AND om.user_id = ?
				AND om.is_active = true
				AND om.role IN (%s)
			)`,
			tableName,
			rolesPlaceholder,
		)

		// Add the user ID for organization check
		args = append(args, userID)

		// Add all manager roles
		for _, role := range f.config.ManagerRoles {
			args = append(args, role)
		}

		// Combine both conditions with OR
		finalCondition := fmt.Sprintf("(%s OR %s)", directMembershipCondition, orgAccessCondition)
		return query.Where(finalCondition, args...)
	}

	// Only direct membership check
	return query.Where(directMembershipCondition, args...)
}
