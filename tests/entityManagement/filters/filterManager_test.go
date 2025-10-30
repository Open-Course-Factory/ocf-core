package filters_test

import (
	"testing"

	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/entityManagement/repositories/filters"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestFilterManager_NewFilterManager tests the creation of a filter manager
func TestFilterManager_NewFilterManager(t *testing.T) {
	relationshipFilters := []entityManagementInterfaces.RelationshipFilter{}
	manager := filters.NewFilterManager(relationshipFilters)

	assert.NotNil(t, manager, "FilterManager should not be nil")

	strategies := manager.GetStrategies()
	assert.Equal(t, 4, len(strategies), "Should have 4 strategies (Direct, ForeignKey, ManyToMany, RelationshipPath)")

	// Verify strategies are sorted by priority
	priorities := []int{}
	for _, strategy := range strategies {
		priorities = append(priorities, strategy.Priority())
	}

	// Priorities should be in ascending order: 5 (RelationshipPath), 10 (Direct), 20 (ForeignKey), 30 (ManyToMany)
	// RelationshipPath has priority 5 (highest) to ensure registered filters take precedence
	assert.Equal(t, []int{5, 10, 20, 30}, priorities, "Strategies should be sorted by priority")
}

// TestFilterManager_ApplyFilters_Integration tests the filter manager with multiple filter types
func TestFilterManager_ApplyFilters_Integration(t *testing.T) {
	// Setup in-memory SQLite database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	// Create comprehensive test schema
	db.Exec("CREATE TABLE courses (id TEXT PRIMARY KEY, title TEXT, status TEXT, price INTEGER, author_id TEXT)")
	db.Exec("CREATE TABLE tags (id TEXT PRIMARY KEY, name TEXT)")
	db.Exec("CREATE TABLE tag_courses (tag_id TEXT, course_id TEXT)")

	// Insert test data
	db.Exec("INSERT INTO courses (id, title, status, price, author_id) VALUES ('1', 'Golang', 'published', 100, 'author-1')")
	db.Exec("INSERT INTO courses (id, title, status, price, author_id) VALUES ('2', 'Python', 'published', 150, 'author-2')")
	db.Exec("INSERT INTO courses (id, title, status, price, author_id) VALUES ('3', 'JavaScript', 'draft', 200, 'author-1')")
	db.Exec("INSERT INTO courses (id, title, status, price, author_id) VALUES ('4', 'Ruby', 'published', 120, 'author-3')")

	db.Exec("INSERT INTO tags (id, name) VALUES ('tag-1', 'backend')")
	db.Exec("INSERT INTO tags (id, name) VALUES ('tag-2', 'frontend')")

	db.Exec("INSERT INTO tag_courses (tag_id, course_id) VALUES ('tag-1', '1')")
	db.Exec("INSERT INTO tag_courses (tag_id, course_id) VALUES ('tag-1', '2')")
	db.Exec("INSERT INTO tag_courses (tag_id, course_id) VALUES ('tag-2', '3')")

	manager := filters.NewFilterManager([]entityManagementInterfaces.RelationshipFilter{})

	t.Run("Single direct field filter", func(t *testing.T) {
		query := db.Model(&struct{}{}).Table("courses")
		filterMap := map[string]any{
			"status": "published",
		}

		query = manager.ApplyFilters(query, filterMap, "courses")

		var count int64
		query.Count(&count)
		assert.Equal(t, int64(3), count, "Should find 3 published courses")
	})

	t.Run("Single foreign key filter", func(t *testing.T) {
		query := db.Model(&struct{}{}).Table("courses")
		filterMap := map[string]any{
			"authorId": "author-1",
		}

		query = manager.ApplyFilters(query, filterMap, "courses")

		var count int64
		query.Count(&count)
		assert.Equal(t, int64(2), count, "Should find 2 courses by author-1")
	})

	t.Run("Single many-to-many filter", func(t *testing.T) {
		query := db.Model(&struct{}{}).Table("courses")
		filterMap := map[string]any{
			"tagIDs": "tag-1",
		}

		query = manager.ApplyFilters(query, filterMap, "courses")

		var count int64
		query.Count(&count)
		assert.Equal(t, int64(2), count, "Should find 2 courses with tag-1")
	})

	t.Run("Multiple filters combined", func(t *testing.T) {
		query := db.Model(&struct{}{}).Table("courses")
		filterMap := map[string]any{
			"status":   "published",
			"authorId": "author-1",
		}

		query = manager.ApplyFilters(query, filterMap, "courses")

		var count int64
		query.Count(&count)
		assert.Equal(t, int64(1), count, "Should find 1 published course by author-1")
	})

	t.Run("All filter types combined", func(t *testing.T) {
		query := db.Model(&struct{}{}).Table("courses")
		filterMap := map[string]any{
			"status":   "published",
			"authorId": "author-1",
			"tagIDs":   "tag-1",
		}

		query = manager.ApplyFilters(query, filterMap, "courses")

		var count int64
		query.Count(&count)
		assert.Equal(t, int64(1), count, "Should find 1 published course by author-1 with tag-1")
	})

	t.Run("Filters with valid syntax are processed", func(t *testing.T) {
		// Note: Unknown column names will cause SQL errors, which is expected behavior.
		// The filter manager doesn't validate column existence - that's the database's job.
		// This test verifies that valid filters work correctly.
		query := db.Model(&struct{}{}).Table("courses")
		filterMap := map[string]any{
			"status": "published",
			"title":  "Golang",
		}

		query = manager.ApplyFilters(query, filterMap, "courses")

		var count int64
		query.Count(&count)
		assert.Equal(t, int64(1), count, "Multiple valid filters should be combined with AND")
	})

	t.Run("Empty filter map", func(t *testing.T) {
		query := db.Model(&struct{}{}).Table("courses")
		filterMap := map[string]any{}

		query = manager.ApplyFilters(query, filterMap, "courses")

		var count int64
		query.Count(&count)
		assert.Equal(t, int64(4), count, "No filters should return all courses")
	})
}

// TestFilterManager_AddStrategy tests adding custom strategies
func TestFilterManager_AddStrategy(t *testing.T) {
	manager := filters.NewFilterManager([]entityManagementInterfaces.RelationshipFilter{})

	initialCount := len(manager.GetStrategies())
	assert.Equal(t, 4, initialCount, "Should start with 4 strategies")

	// Add a custom strategy (using DirectFieldFilter as example)
	customStrategy := &filters.DirectFieldFilter{}
	manager.AddStrategy(customStrategy)

	newCount := len(manager.GetStrategies())
	assert.Equal(t, 5, newCount, "Should have 5 strategies after adding one")

	// Verify strategies are still sorted by priority
	strategies := manager.GetStrategies()
	for i := 0; i < len(strategies)-1; i++ {
		assert.LessOrEqual(t, strategies[i].Priority(), strategies[i+1].Priority(),
			"Strategies should remain sorted by priority after adding new strategy")
	}
}

// TestFilterManager_StrategyPriority tests that strategies are applied in priority order
func TestFilterManager_StrategyPriority(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	// Create a table with a field named "status" and a field named "statusId"
	db.Exec("CREATE TABLE items (id TEXT PRIMARY KEY, status TEXT, status_id TEXT)")
	db.Exec("INSERT INTO items (id, status, status_id) VALUES ('1', 'active', 'status-1')")
	db.Exec("INSERT INTO items (id, status, status_id) VALUES ('2', 'inactive', 'status-2')")

	manager := filters.NewFilterManager([]entityManagementInterfaces.RelationshipFilter{})

	t.Run("Direct field takes precedence over foreign key pattern", func(t *testing.T) {
		query := db.Model(&struct{}{}).Table("items")
		filterMap := map[string]any{
			"status": "active",
		}

		query = manager.ApplyFilters(query, filterMap, "items")

		var count int64
		query.Count(&count)
		// "status" should be treated as a direct field, not a foreign key
		assert.Equal(t, int64(1), count, "Direct field filter should be applied")
	})

	t.Run("Foreign key pattern is correctly matched", func(t *testing.T) {
		query := db.Model(&struct{}{}).Table("items")
		filterMap := map[string]any{
			"statusId": "status-1",
		}

		query = manager.ApplyFilters(query, filterMap, "items")

		var count int64
		query.Count(&count)
		// "statusId" should be treated as a foreign key
		assert.Equal(t, int64(1), count, "Foreign key filter should be applied")
	})
}
