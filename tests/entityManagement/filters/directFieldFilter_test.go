package filters_test

import (
	"testing"

	"soli/formations/src/entityManagement/repositories/filters"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestDirectFieldFilter_Matches tests the Matches method
func TestDirectFieldFilter_Matches(t *testing.T) {
	filter := &filters.DirectFieldFilter{}

	tests := []struct {
		name     string
		key      string
		value    interface{}
		expected bool
	}{
		{
			name:     "Regular field - title",
			key:      "title",
			value:    "Golang",
			expected: true,
		},
		{
			name:     "Regular field - status",
			key:      "status",
			value:    "published",
			expected: true,
		},
		{
			name:     "Foreign key - courseId",
			key:      "courseId",
			value:    "123",
			expected: false, // Should not match foreign keys
		},
		{
			name:     "Foreign key - userID",
			key:      "userID",
			value:    "456",
			expected: false,
		},
		{
			name:     "Many-to-many - tagIDs",
			key:      "tagIDs",
			value:    "1,2,3",
			expected: false, // Should not match many-to-many
		},
		{
			name:     "Many-to-many - authorIds",
			key:      "authorIds",
			value:    []string{"a", "b"},
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

// TestDirectFieldFilter_Apply tests the Apply method with different value types
func TestDirectFieldFilter_Apply(t *testing.T) {
	// Setup in-memory SQLite database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	// Create test table
	db.Exec("CREATE TABLE courses (id TEXT PRIMARY KEY, title TEXT, status TEXT, price INTEGER)")
	db.Exec("INSERT INTO courses (id, title, status, price) VALUES ('1', 'Golang', 'published', 100)")
	db.Exec("INSERT INTO courses (id, title, status, price) VALUES ('2', 'Python', 'published', 150)")
	db.Exec("INSERT INTO courses (id, title, status, price) VALUES ('3', 'JavaScript', 'draft', 200)")

	filter := &filters.DirectFieldFilter{}

	t.Run("Single string value", func(t *testing.T) {
		query := db.Model(&struct{}{}).Table("courses")
		query = filter.Apply(query, "title", "Golang", "courses")

		var count int64
		query.Count(&count)
		assert.Equal(t, int64(1), count, "Should find 1 course with title='Golang'")
	})

	t.Run("Comma-separated string values", func(t *testing.T) {
		query := db.Model(&struct{}{}).Table("courses")
		query = filter.Apply(query, "title", "Golang,Python", "courses")

		var count int64
		query.Count(&count)
		assert.Equal(t, int64(2), count, "Should find 2 courses with title IN ('Golang', 'Python')")
	})

	t.Run("String array", func(t *testing.T) {
		query := db.Model(&struct{}{}).Table("courses")
		query = filter.Apply(query, "status", []string{"published", "draft"}, "courses")

		var count int64
		query.Count(&count)
		assert.Equal(t, int64(3), count, "Should find 3 courses with status IN ('published', 'draft')")
	})

	t.Run("Interface array", func(t *testing.T) {
		query := db.Model(&struct{}{}).Table("courses")
		query = filter.Apply(query, "title", []interface{}{"Golang", "JavaScript"}, "courses")

		var count int64
		query.Count(&count)
		assert.Equal(t, int64(2), count, "Should find 2 courses")
	})

	t.Run("Integer value", func(t *testing.T) {
		query := db.Model(&struct{}{}).Table("courses")
		query = filter.Apply(query, "price", 100, "courses")

		var count int64
		query.Count(&count)
		assert.Equal(t, int64(1), count, "Should find 1 course with price=100")
	})
}

// TestDirectFieldFilter_Priority tests the Priority method
func TestDirectFieldFilter_Priority(t *testing.T) {
	filter := &filters.DirectFieldFilter{}
	assert.Equal(t, 10, filter.Priority(), "DirectFieldFilter should have priority 10")
}
