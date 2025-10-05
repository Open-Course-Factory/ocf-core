package filters_test

import (
	"testing"

	"soli/formations/src/entityManagement/repositories/filters"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestForeignKeyFilter_Matches tests the Matches method
func TestForeignKeyFilter_Matches(t *testing.T) {
	filter := &filters.ForeignKeyFilter{}

	tests := []struct {
		name     string
		key      string
		value    interface{}
		expected bool
	}{
		{
			name:     "Foreign key - courseId",
			key:      "courseId",
			value:    "123",
			expected: true,
		},
		{
			name:     "Foreign key - userID",
			key:      "userID",
			value:    "456",
			expected: true,
		},
		{
			name:     "Foreign key - authorId",
			key:      "authorId",
			value:    []string{"a", "b"},
			expected: true,
		},
		{
			name:     "Many-to-many - tagIDs (should not match)",
			key:      "tagIDs",
			value:    "1,2,3",
			expected: false, // Should not match many-to-many
		},
		{
			name:     "Many-to-many - authorIds (should not match)",
			key:      "authorIds",
			value:    []string{"a", "b"},
			expected: false,
		},
		{
			name:     "Regular field - title",
			key:      "title",
			value:    "Golang",
			expected: false, // Should not match regular fields
		},
		{
			name:     "Regular field - status",
			key:      "status",
			value:    "published",
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

// TestForeignKeyFilter_Apply tests the Apply method with different value types
func TestForeignKeyFilter_Apply(t *testing.T) {
	// Setup in-memory SQLite database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	// Create test tables
	db.Exec("CREATE TABLE chapters (id TEXT PRIMARY KEY, title TEXT, course_id TEXT)")
	db.Exec("INSERT INTO chapters (id, title, course_id) VALUES ('1', 'Chapter 1', 'course-1')")
	db.Exec("INSERT INTO chapters (id, title, course_id) VALUES ('2', 'Chapter 2', 'course-1')")
	db.Exec("INSERT INTO chapters (id, title, course_id) VALUES ('3', 'Chapter 3', 'course-2')")

	filter := &filters.ForeignKeyFilter{}

	t.Run("Single foreign key value", func(t *testing.T) {
		query := db.Model(&struct{}{}).Table("chapters")
		query = filter.Apply(query, "courseId", "course-1", "chapters")

		var count int64
		query.Count(&count)
		assert.Equal(t, int64(2), count, "Should find 2 chapters with course_id='course-1'")
	})

	t.Run("Comma-separated foreign key values", func(t *testing.T) {
		query := db.Model(&struct{}{}).Table("chapters")
		query = filter.Apply(query, "courseId", "course-1,course-2", "chapters")

		var count int64
		query.Count(&count)
		assert.Equal(t, int64(3), count, "Should find 3 chapters with course_id IN ('course-1', 'course-2')")
	})

	t.Run("Foreign key array", func(t *testing.T) {
		query := db.Model(&struct{}{}).Table("chapters")
		query = filter.Apply(query, "courseId", []string{"course-1", "course-2"}, "chapters")

		var count int64
		query.Count(&count)
		assert.Equal(t, int64(3), count, "Should find 3 chapters")
	})

	t.Run("Foreign key interface array", func(t *testing.T) {
		query := db.Model(&struct{}{}).Table("chapters")
		query = filter.Apply(query, "courseId", []interface{}{"course-1"}, "chapters")

		var count int64
		query.Count(&count)
		assert.Equal(t, int64(2), count, "Should find 2 chapters")
	})
}

// TestForeignKeyFilter_Priority tests the Priority method
func TestForeignKeyFilter_Priority(t *testing.T) {
	filter := &filters.ForeignKeyFilter{}
	assert.Equal(t, 20, filter.Priority(), "ForeignKeyFilter should have priority 20")
}
