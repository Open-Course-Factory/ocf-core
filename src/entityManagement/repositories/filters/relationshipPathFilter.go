package filters

import (
	"fmt"
	"strings"

	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"

	"gorm.io/gorm"
)

// RelationshipPathFilter handles filters that traverse complex relationship paths.
// This strategy is used for registered relationship filters defined in entity registrations.
//
// Example:
// If Pages can be filtered by courseId through Chapters:
//   - Path: Pages → Chapters → Courses
//   - Filter: ?courseId=123
//   - SQL: EXISTS (SELECT 1 FROM chapters WHERE chapters.id = pages.chapter_id
//     AND chapters.course_id = '123')
//
// Relationship filters are registered in entity definitions using the RelationshipFilter struct.
// This allows entities to define custom filter paths that traverse multiple relationships.
//
// This strategy has priority 40, checked last to allow simpler filters to match first.
type RelationshipPathFilter struct {
	BaseFilterStrategy
	registeredFilters map[string]entityManagementInterfaces.RelationshipFilter
}

// NewRelationshipPathFilter creates a new relationship path filter with registered filters.
func NewRelationshipPathFilter(filters []entityManagementInterfaces.RelationshipFilter) *RelationshipPathFilter {
	filterMap := make(map[string]entityManagementInterfaces.RelationshipFilter)
	for _, rf := range filters {
		filterMap[rf.FilterName] = rf
	}

	return &RelationshipPathFilter{
		registeredFilters: filterMap,
	}
}

// Matches returns true if the key matches a registered relationship filter.
func (r *RelationshipPathFilter) Matches(key string, value any) bool {
	_, exists := r.registeredFilters[key]
	return exists
}

// Apply applies the relationship path filter using nested EXISTS clauses.
// This builds a complex SQL query that follows the relationship path defined
// in the registered filter.
func (r *RelationshipPathFilter) Apply(query *gorm.DB, key string, value any, tableName string) *gorm.DB {
	relFilter, exists := r.registeredFilters[key]
	if !exists {
		return query
	}

	return applyRelationshipFilter(query, relFilter, value, tableName)
}

// Priority returns 5, giving relationship path filters the HIGHEST priority.
// This ensures registered relationship filters are checked before pattern-based filters.
// This is critical because a filter like "courseId" might match both a ForeignKeyFilter pattern
// AND a registered relationship filter - we want the specific registered filter to win.
func (r *RelationshipPathFilter) Priority() int {
	return 5
}

// applyRelationshipFilter applies filters through relationship paths using EXISTS clauses.
// This is the core logic for traversing complex relationship paths.
//
// The function builds nested EXISTS clauses that follow the relationship path
// defined in the RelationshipFilter. Each step in the path is joined to the previous step.
//
// Example for a two-step path (Pages → Chapters → Courses):
//
//	EXISTS (
//	  SELECT 1 FROM chapters
//	  WHERE chapters.id = pages.chapter_id
//	  AND EXISTS (
//	    SELECT 1 FROM courses
//	    WHERE courses.id = chapters.course_id
//	    AND courses.id IN ?
//	  )
//	)
func applyRelationshipFilter(query *gorm.DB, relFilter entityManagementInterfaces.RelationshipFilter, value any, currentTable string) *gorm.DB {
	// Convert value to string array
	var ids []string
	switch v := value.(type) {
	case string:
		if strings.Contains(v, ",") {
			ids = strings.Split(v, ",")
			// Trim whitespace
			for i, id := range ids {
				ids[i] = strings.TrimSpace(id)
			}
		} else {
			ids = []string{v}
		}
	case []string:
		ids = v
	case []any:
		for _, val := range v {
			ids = append(ids, fmt.Sprint(val))
		}
	default:
		ids = []string{fmt.Sprint(v)}
	}

	if len(ids) == 0 {
		return query
	}

	// Build the EXISTS clause with the relationship path
	var existsClause strings.Builder
	existsClause.WriteString("EXISTS (SELECT 1 FROM ")

	// Validate path exists
	if len(relFilter.Path) == 0 {
		return query
	}

	// Start with the first join table
	firstStep := relFilter.Path[0]
	existsClause.WriteString(firstStep.JoinTable)
	existsClause.WriteString(" WHERE ")
	existsClause.WriteString(firstStep.JoinTable)
	existsClause.WriteString(".")
	existsClause.WriteString(firstStep.SourceColumn)
	existsClause.WriteString(" = ")
	existsClause.WriteString(currentTable)
	existsClause.WriteString(".id")

	// Add subsequent joins as nested EXISTS clauses
	for i := 1; i < len(relFilter.Path); i++ {
		step := relFilter.Path[i]
		prevStep := relFilter.Path[i-1]

		existsClause.WriteString(" AND EXISTS (SELECT 1 FROM ")
		existsClause.WriteString(step.JoinTable)
		existsClause.WriteString(" WHERE ")
		existsClause.WriteString(step.JoinTable)
		existsClause.WriteString(".")
		existsClause.WriteString(step.SourceColumn)
		existsClause.WriteString(" = ")
		existsClause.WriteString(prevStep.JoinTable)
		existsClause.WriteString(".")
		existsClause.WriteString(prevStep.TargetColumn)
	}

	// Add final condition with the target column and IDs
	lastStep := relFilter.Path[len(relFilter.Path)-1]
	existsClause.WriteString(" AND ")
	existsClause.WriteString(lastStep.JoinTable)
	existsClause.WriteString(".")
	existsClause.WriteString(lastStep.TargetColumn)
	existsClause.WriteString(" IN ?")

	// Close all parentheses (one for each nested EXISTS + the main EXISTS)
	for i := 1; i < len(relFilter.Path); i++ {
		existsClause.WriteString(")")
	}
	existsClause.WriteString(")")

	return query.Where(existsClause.String(), ids)
}
