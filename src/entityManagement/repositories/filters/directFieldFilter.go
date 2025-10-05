package filters

import (
	"strings"

	"gorm.io/gorm"
)

// DirectFieldFilter handles filters for regular entity fields without special naming patterns.
// This strategy matches filters that don't end with "Id", "ID", "IDs", or "Ids".
//
// Examples:
//   - title=Golang → WHERE title = 'Golang'
//   - status=published → WHERE status = 'published'
//   - title=Golang,Python → WHERE title IN ('Golang', 'Python')
//
// This strategy has the highest priority (10) to ensure regular fields are matched
// before checking for foreign key or relationship patterns.
type DirectFieldFilter struct {
	BaseFilterStrategy
}

// Matches returns true if the key represents a direct field filter.
// Direct fields don't have special suffixes like Id, ID, IDs, or Ids.
func (d *DirectFieldFilter) Matches(key string, value interface{}) bool {
	// Exclude foreign key patterns (Id, ID) and many-to-many patterns (IDs, Ids)
	return !strings.HasSuffix(key, "Id") &&
		!strings.HasSuffix(key, "ID") &&
		!strings.HasSuffix(key, "IDs") &&
		!strings.HasSuffix(key, "Ids")
}

// Apply applies the direct field filter to the query.
// Supports both single values and comma-separated or array values for IN clauses.
func (d *DirectFieldFilter) Apply(query *gorm.DB, key string, value interface{}, tableName string) *gorm.DB {
	dbColumn := camelToSnake(key)

	switch v := value.(type) {
	case string:
		// Check if comma-separated for IN clause
		if strings.Contains(v, ",") {
			values := strings.Split(v, ",")
			// Trim whitespace from each value
			for i, val := range values {
				values[i] = strings.TrimSpace(val)
			}
			return query.Where(dbColumn+" IN ?", values)
		}
		// Single value equality
		return query.Where(dbColumn+" = ?", v)

	case []string:
		// Array of strings - use IN clause
		return query.Where(dbColumn+" IN ?", v)

	case []interface{}:
		// Array of interfaces - use IN clause
		return query.Where(dbColumn+" IN ?", v)

	default:
		// Other types (int, bool, etc.) - direct equality
		return query.Where(dbColumn+" = ?", v)
	}
}

// Priority returns 10, giving direct field filters the highest priority.
// This ensures they're checked before foreign key or relationship patterns.
func (d *DirectFieldFilter) Priority() int {
	return 10
}

// camelToSnake converts camelCase to snake_case for database column names.
// Examples: "firstName" → "first_name", "userID" → "user_id"
func camelToSnake(s string) string {
	var result strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result.WriteRune('_')
		}
		result.WriteRune(r)
	}
	return strings.ToLower(result.String())
}
