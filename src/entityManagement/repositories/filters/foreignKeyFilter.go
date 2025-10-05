package filters

import (
	"strings"

	"gorm.io/gorm"
)

// ForeignKeyFilter handles filters for foreign key relationships.
// This strategy matches filters that end with "Id" or "ID" but not "IDs" or "Ids".
//
// Examples:
//   - courseId=123 → WHERE course_id = '123'
//   - userId=456 → WHERE user_id = '456'
//   - courseId=123,456 → WHERE course_id IN ('123', '456')
//
// This strategy has priority 20, lower than DirectFieldFilter (10) but higher
// than ManyToManyFilter (30), ensuring single foreign keys are matched before
// many-to-many relationships.
type ForeignKeyFilter struct {
	BaseFilterStrategy
}

// Matches returns true if the key represents a foreign key filter.
// Foreign keys end with "Id" or "ID" but not "IDs" or "Ids".
func (f *ForeignKeyFilter) Matches(key string, value interface{}) bool {
	// Match keys ending with Id or ID, but exclude IDs/Ids (many-to-many)
	return (strings.HasSuffix(key, "Id") || strings.HasSuffix(key, "ID")) &&
		!strings.HasSuffix(key, "IDs") &&
		!strings.HasSuffix(key, "Ids")
}

// Apply applies the foreign key filter to the query.
// Supports both single IDs and comma-separated or array IDs for IN clauses.
func (f *ForeignKeyFilter) Apply(query *gorm.DB, key string, value interface{}, tableName string) *gorm.DB {
	dbColumn := camelToSnake(key)

	switch v := value.(type) {
	case string:
		// Check if comma-separated for IN clause
		if strings.Contains(v, ",") {
			ids := strings.Split(v, ",")
			// Trim whitespace from each ID
			for i, id := range ids {
				ids[i] = strings.TrimSpace(id)
			}
			return query.Where(dbColumn+" IN ?", ids)
		}
		// Single ID equality
		return query.Where(dbColumn+" = ?", v)

	case []string:
		// Array of IDs - use IN clause
		return query.Where(dbColumn+" IN ?", v)

	case []interface{}:
		// Array of interfaces - use IN clause
		return query.Where(dbColumn+" IN ?", v)

	default:
		// Other types (UUID, int, etc.) - direct equality
		return query.Where(dbColumn+" = ?", v)
	}
}

// Priority returns 20, giving foreign key filters medium-high priority.
// This is checked after direct fields (10) but before many-to-many (30).
func (f *ForeignKeyFilter) Priority() int {
	return 20
}
