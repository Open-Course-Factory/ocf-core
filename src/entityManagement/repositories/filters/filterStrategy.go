package filters

import (
	"gorm.io/gorm"
)

// FilterStrategy defines the interface for different filter handling strategies.
// Each strategy implements a specific filtering approach (direct field, foreign key, many-to-many, etc.).
//
// The strategy pattern allows for:
//   - Independent testing of each filter type
//   - Easy addition of new filter types without modifying existing code
//   - Clear separation of concerns
//
// Example usage:
//
//	strategy := &DirectFieldFilter{}
//	if strategy.Matches("title", "Golang") {
//	    query = strategy.Apply(query, "title", "Golang", "courses")
//	}
type FilterStrategy interface {
	// Matches returns true if this strategy can handle the given filter key and value.
	// This method is called first to determine which strategy should process the filter.
	//
	// Parameters:
	//   - key: The filter key (e.g., "title", "courseId", "tagIDs")
	//   - value: The filter value (can be string, []string, or other types)
	//
	// Returns:
	//   - true if this strategy should handle this filter
	//   - false otherwise
	Matches(key string, value interface{}) bool

	// Apply applies the filter to the GORM query and returns the modified query.
	//
	// Parameters:
	//   - query: The GORM query to modify
	//   - key: The filter key
	//   - value: The filter value
	//   - tableName: The name of the current table being queried
	//
	// Returns:
	//   - Modified GORM query with the filter applied
	Apply(query *gorm.DB, key string, value interface{}, tableName string) *gorm.DB

	// Priority returns the execution priority of this strategy.
	// Lower numbers are checked first. This allows more specific strategies
	// to take precedence over generic ones.
	//
	// Recommended priorities:
	//   - 10: Direct field filters (most specific)
	//   - 20: Foreign key filters
	//   - 30: Many-to-many filters
	//   - 40: Relationship path filters
	//   - 100+: Generic/fallback filters
	Priority() int
}

// BaseFilterStrategy provides default implementations for common strategy methods.
// Concrete strategies can embed this to inherit the default Priority() method.
type BaseFilterStrategy struct{}

// Priority returns the default priority for strategies.
// Override this method in concrete strategies to set a specific priority.
func (b *BaseFilterStrategy) Priority() int {
	return 100
}
