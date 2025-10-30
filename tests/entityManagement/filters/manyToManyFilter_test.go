package filters_test

import (
	"testing"

	"soli/formations/src/entityManagement/repositories/filters"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestManyToManyFilter_Matches tests the Matches method
func TestManyToManyFilter_Matches(t *testing.T) {
	filter := &filters.ManyToManyFilter{}

	tests := []struct {
		name     string
		key      string
		value    any
		expected bool
	}{
		{
			name:     "Many-to-many - tagIDs",
			key:      "tagIDs",
			value:    "1,2,3",
			expected: true,
		},
		{
			name:     "Many-to-many - authorIds",
			key:      "authorIds",
			value:    []string{"a", "b"},
			expected: true,
		},
		{
			name:     "Many-to-many - categoryIDs",
			key:      "categoryIDs",
			value:    []any{"1", "2"},
			expected: true,
		},
		{
			name:     "Foreign key - courseId (should not match)",
			key:      "courseId",
			value:    "123",
			expected: false,
		},
		{
			name:     "Foreign key - userID (should not match)",
			key:      "userID",
			value:    "456",
			expected: false,
		},
		{
			name:     "Regular field - title (should not match)",
			key:      "title",
			value:    "Golang",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filter.Matches(tt.key, tt.value)
			assert.Equal(t, tt.expected, result, "Matches() should return %v for key %s", tt.expected, tt.key)
		})
	}
}

// TestManyToManyFilter_Apply tests the Apply method with join tables
func TestManyToManyFilter_Apply(t *testing.T) {
	// Setup in-memory SQLite database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	// Create test tables: courses, tags, and the join table tag_courses
	db.Exec("CREATE TABLE courses (id TEXT PRIMARY KEY, title TEXT)")
	db.Exec("CREATE TABLE tags (id TEXT PRIMARY KEY, name TEXT)")
	db.Exec("CREATE TABLE tag_courses (tag_id TEXT, course_id TEXT)")

	// Insert test data
	db.Exec("INSERT INTO courses (id, title) VALUES ('course-1', 'Golang')")
	db.Exec("INSERT INTO courses (id, title) VALUES ('course-2', 'Python')")
	db.Exec("INSERT INTO courses (id, title) VALUES ('course-3', 'JavaScript')")

	db.Exec("INSERT INTO tags (id, name) VALUES ('tag-1', 'backend')")
	db.Exec("INSERT INTO tags (id, name) VALUES ('tag-2', 'frontend')")

	// Course 1 has tag 1 (backend)
	db.Exec("INSERT INTO tag_courses (tag_id, course_id) VALUES ('tag-1', 'course-1')")
	// Course 2 has tag 1 (backend)
	db.Exec("INSERT INTO tag_courses (tag_id, course_id) VALUES ('tag-1', 'course-2')")
	// Course 3 has tag 2 (frontend)
	db.Exec("INSERT INTO tag_courses (tag_id, course_id) VALUES ('tag-2', 'course-3')")

	filter := &filters.ManyToManyFilter{}

	t.Run("Single tag ID", func(t *testing.T) {
		query := db.Model(&struct{}{}).Table("courses")
		query = filter.Apply(query, "tagIDs", "tag-1", "courses")

		var count int64
		query.Count(&count)
		assert.Equal(t, int64(2), count, "Should find 2 courses with tag_id='tag-1'")
	})

	t.Run("Comma-separated tag IDs", func(t *testing.T) {
		query := db.Model(&struct{}{}).Table("courses")
		query = filter.Apply(query, "tagIDs", "tag-1,tag-2", "courses")

		var count int64
		query.Count(&count)
		assert.Equal(t, int64(3), count, "Should find 3 courses with tag_id IN ('tag-1', 'tag-2')")
	})

	t.Run("Tag ID array", func(t *testing.T) {
		query := db.Model(&struct{}{}).Table("courses")
		query = filter.Apply(query, "tagIDs", []string{"tag-1"}, "courses")

		var count int64
		query.Count(&count)
		assert.Equal(t, int64(2), count, "Should find 2 courses")
	})

	t.Run("Empty IDs array", func(t *testing.T) {
		query := db.Model(&struct{}{}).Table("courses")
		originalSQL := query.ToSQL(func(tx *gorm.DB) *gorm.DB {
			return tx.Find(&struct{}{})
		})

		query = filter.Apply(query, "tagIDs", []string{}, "courses")
		newSQL := query.ToSQL(func(tx *gorm.DB) *gorm.DB {
			return tx.Find(&struct{}{})
		})

		// Should not modify query when empty IDs
		assert.Equal(t, originalSQL, newSQL, "Query should not be modified for empty IDs")
	})
}

// TestManyToManyFilter_Priority tests the Priority method
func TestManyToManyFilter_Priority(t *testing.T) {
	filter := &filters.ManyToManyFilter{}
	assert.Equal(t, 30, filter.Priority(), "ManyToManyFilter should have priority 30")
}
