package filters

import (
	"strings"

	"gorm.io/gorm"
)

// ManyToManyFilter handles filters for many-to-many relationships.
// This strategy matches filters that end with "IDs" or "Ids" (plural).
//
// Examples:
//   - tagIDs=1,2,3 → EXISTS (SELECT 1 FROM tag_courses WHERE tag_courses.course_id = courses.id AND tag_courses.tag_id IN ('1','2','3'))
//   - authorIDs=abc,def → EXISTS (SELECT 1 FROM author_courses WHERE author_courses.course_id = courses.id AND author_courses.author_id IN ('abc','def'))
//
// The filter uses an EXISTS clause with the join table to handle the many-to-many relationship.
// Join table naming follows the pattern: singular_relation + "_" + current_table
//
// This strategy has priority 30, checked after direct fields (10) and foreign keys (20).
type ManyToManyFilter struct {
	BaseFilterStrategy
}

// Matches returns true if the key represents a many-to-many filter.
// Many-to-many filters end with "IDs" or "Ids" (plural form).
func (m *ManyToManyFilter) Matches(key string, value interface{}) bool {
	return strings.HasSuffix(key, "IDs") || strings.HasSuffix(key, "Ids")
}

// Apply applies the many-to-many filter to the query using an EXISTS clause.
// This method builds a subquery that checks for the existence of related records
// in the join table.
func (m *ManyToManyFilter) Apply(query *gorm.DB, key string, value interface{}, tableName string) *gorm.DB {
	// Extract relation name: "tagIDs" → "tag", "authorIds" → "author"
	relationName := strings.TrimSuffix(strings.TrimSuffix(key, "IDs"), "Ids")

	// Build table and column names
	relationTable := pluralize(relationName)
	singularRelation := strings.TrimSuffix(relationTable, "s")
	singularCurrent := strings.TrimSuffix(tableName, "s")

	// Build join table name (e.g., "tag_courses")
	joinTable := singularRelation + "_" + tableName

	// Build foreign key column names
	relationFK := singularRelation + "_id"
	currentFK := singularCurrent + "_id"

	// Parse the IDs from different value types
	var ids []string
	switch v := value.(type) {
	case string:
		// Handle comma-separated string
		if strings.Contains(v, ",") {
			ids = strings.Split(v, ",")
			// Trim whitespace from each ID
			for i, id := range ids {
				ids[i] = strings.TrimSpace(id)
			}
		} else {
			ids = []string{v}
		}

	case []string:
		ids = v

	case []interface{}:
		// Convert interface slice to string slice
		for _, val := range v {
			ids = append(ids, strings.TrimSpace(val.(string)))
		}

	default:
		// Unknown type - skip this filter
		return query
	}

	// Don't apply filter if no IDs provided
	if len(ids) == 0 {
		return query
	}

	// Build EXISTS clause for the many-to-many relationship
	// Example: EXISTS (SELECT 1 FROM tag_courses WHERE tag_courses.course_id = courses.id AND tag_courses.tag_id IN ?)
	var clauseBuilder strings.Builder
	clauseBuilder.WriteString("EXISTS (SELECT 1 FROM ")
	clauseBuilder.WriteString(joinTable)
	clauseBuilder.WriteString(" WHERE ")
	clauseBuilder.WriteString(joinTable)
	clauseBuilder.WriteString(".")
	clauseBuilder.WriteString(currentFK)
	clauseBuilder.WriteString(" = ")
	clauseBuilder.WriteString(tableName)
	clauseBuilder.WriteString(".id AND ")
	clauseBuilder.WriteString(joinTable)
	clauseBuilder.WriteString(".")
	clauseBuilder.WriteString(relationFK)
	clauseBuilder.WriteString(" IN ?)")

	return query.Where(clauseBuilder.String(), ids)
}

// Priority returns 30, giving many-to-many filters medium priority.
// This is checked after direct fields (10) and foreign keys (20).
func (m *ManyToManyFilter) Priority() int {
	return 30
}

// pluralize converts a singular word to plural form.
// This is a simple implementation that handles common English pluralization rules.
//
// Examples:
//   - "tag" → "tags"
//   - "course" → "courses"
//   - "category" → "categories"
//   - "child" → "children" (irregular form would need special handling)
//
// Note: For complex pluralization, consider using a library like inflection.
func pluralize(s string) string {
	s = strings.ToLower(s)

	// Handle special cases
	switch s {
	case "child":
		return "children"
	case "person":
		return "people"
	case "man":
		return "men"
	case "woman":
		return "women"
	case "tooth":
		return "teeth"
	case "foot":
		return "feet"
	case "mouse":
		return "mice"
	case "goose":
		return "geese"
	}

	// Handle words ending in 'y'
	if strings.HasSuffix(s, "y") && len(s) > 1 {
		// If preceded by consonant, replace 'y' with 'ies'
		penultimate := s[len(s)-2]
		if !isVowel(penultimate) {
			return s[:len(s)-1] + "ies"
		}
	}

	// Handle words ending in 's', 'ss', 'sh', 'ch', 'x', 'z'
	if strings.HasSuffix(s, "s") || strings.HasSuffix(s, "ss") ||
		strings.HasSuffix(s, "sh") || strings.HasSuffix(s, "ch") ||
		strings.HasSuffix(s, "x") || strings.HasSuffix(s, "z") {
		return s + "es"
	}

	// Handle words ending in 'f' or 'fe'
	if strings.HasSuffix(s, "f") {
		return s[:len(s)-1] + "ves"
	}
	if strings.HasSuffix(s, "fe") {
		return s[:len(s)-2] + "ves"
	}

	// Default: just add 's'
	return s + "s"
}

// isVowel checks if a character is a vowel.
func isVowel(c byte) bool {
	c = byte(strings.ToLower(string(c))[0])
	return c == 'a' || c == 'e' || c == 'i' || c == 'o' || c == 'u'
}
