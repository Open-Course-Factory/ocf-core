package filters

import (
	"sort"

	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"

	"gorm.io/gorm"
)

// FilterManager orchestrates multiple filter strategies to process query filters.
// It maintains a prioritized list of strategies and delegates each filter to the
// first matching strategy.
//
// The manager follows these principles:
//   - Strategies are checked in priority order (lower priority number = checked first)
//   - The first strategy that matches a filter handles it (first-match-wins)
//   - Unknown filters are silently ignored (allowing for flexible API parameters)
//
// Example usage:
//
//	// Create manager with relationship filters from entity registration
//	relationshipFilters := ems.GlobalEntityRegistrationService.GetRelationshipFilters("Course")
//	manager := NewFilterManager(relationshipFilters)
//
//	// Apply all filters to query
//	filters := map[string]any{
//	    "title": "Golang",
//	    "authorId": "123",
//	    "tagIDs": "1,2,3",
//	}
//	query = manager.ApplyFilters(query, filters, "courses")
type FilterManager struct {
	strategies []FilterStrategy
}

// NewFilterManager creates a new filter manager with all standard strategies.
// The manager is initialized with:
//   - DirectFieldFilter (priority 10) - regular fields
//   - ForeignKeyFilter (priority 20) - foreign keys (Id/ID suffix)
//   - ManyToManyFilter (priority 30) - many-to-many (IDs/Ids suffix)
//   - RelationshipPathFilter (priority 40) - registered relationship paths
//
// Strategies are automatically sorted by priority after initialization.
//
// Parameters:
//   - relationshipFilters: Registered relationship filters from entity registration
//
// Returns:
//   - Configured FilterManager ready to process filters
func NewFilterManager(relationshipFilters []entityManagementInterfaces.RelationshipFilter) *FilterManager {
	fm := &FilterManager{
		strategies: []FilterStrategy{
			&OrganizationMemberFilter{}, // Priority 5 - Organization membership filter
			&DirectFieldFilter{},
			&ForeignKeyFilter{},
			&ManyToManyFilter{},
			NewRelationshipPathFilter(relationshipFilters),
		},
	}

	// Sort strategies by priority (lower number = higher priority)
	sort.Slice(fm.strategies, func(i, j int) bool {
		return fm.strategies[i].Priority() < fm.strategies[j].Priority()
	})

	return fm
}

// ApplyFilters applies all filters to the query using the appropriate strategies.
// Each filter is processed by the first strategy that matches it.
//
// The process:
//  1. Iterate through each filter key-value pair
//  2. Check strategies in priority order
//  3. First matching strategy processes the filter
//  4. Unknown filters are ignored (no strategy matches)
//
// Parameters:
//   - query: The GORM query to modify
//   - filters: Map of filter keys to values
//   - tableName: Current table being queried (used for column references)
//
// Returns:
//   - Modified query with all applicable filters applied
//
// Example:
//
//	filters := map[string]any{
//	    "title": "Golang",        // → DirectFieldFilter
//	    "courseId": "123",        // → ForeignKeyFilter
//	    "tagIDs": "1,2,3",        // → ManyToManyFilter
//	    "unknown": "value",       // → Ignored (no matching strategy)
//	}
//	query = manager.ApplyFilters(query, filters, "courses")
func (fm *FilterManager) ApplyFilters(
	query *gorm.DB,
	filters map[string]any,
	tableName string,
) *gorm.DB {
	// Process each filter
	for key, value := range filters {
		// Find the first strategy that can handle this filter
		for _, strategy := range fm.strategies {
			if strategy.Matches(key, value) {
				query = strategy.Apply(query, key, value, tableName)
				break // First match wins - move to next filter
			}
		}
		// If no strategy matches, the filter is silently ignored
		// This allows for flexible API parameters that may not all be filters
	}

	return query
}

// AddStrategy adds a custom strategy to the manager.
// This allows for extending the filter system with custom filter types.
//
// After adding strategies, the manager re-sorts by priority.
//
// Example:
//
//	// Add a custom "contains" filter for text search
//	containsFilter := &ContainsFilter{}
//	manager.AddStrategy(containsFilter)
func (fm *FilterManager) AddStrategy(strategy FilterStrategy) {
	fm.strategies = append(fm.strategies, strategy)

	// Re-sort by priority
	sort.Slice(fm.strategies, func(i, j int) bool {
		return fm.strategies[i].Priority() < fm.strategies[j].Priority()
	})
}

// GetStrategies returns all registered strategies in priority order.
// This is primarily useful for testing and debugging.
func (fm *FilterManager) GetStrategies() []FilterStrategy {
	return fm.strategies
}
