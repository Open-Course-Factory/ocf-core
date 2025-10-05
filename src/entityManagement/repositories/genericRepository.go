package repositories

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"reflect"
	errors "soli/formations/src/auth/errors"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type GenericRepository interface {
	CreateEntity(data any, entityName string) (any, error)
	SaveEntity(entity any) (any, error)
	GetEntity(id uuid.UUID, data any, entityName string, includes []string) (any, error)
	GetAllEntities(data any, page int, pageSize int, filters map[string]interface{}, includes []string) ([]any, int64, error)
	GetAllEntitiesCursor(data any, cursor string, limit int, filters map[string]interface{}, includes []string) ([]any, string, bool, int64, error)
	EditEntity(id uuid.UUID, entityName string, entity any, data any) error
	DeleteEntity(id uuid.UUID, entity any, scoped bool) error
}

type genericRepository struct {
	db *gorm.DB
}

func NewGenericRepository(db *gorm.DB) GenericRepository {
	repository := &genericRepository{
		db: db,
	}
	return repository
}

func (o *genericRepository) CreateEntity(entityInputDto any, entityName string) (any, error) {
	conversionFunctionRef, found := ems.GlobalEntityRegistrationService.GetConversionFunction(entityName, ems.CreateInputDtoToModel)

	if !found {
		return nil, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Entity conversion function does not exist",
		}
	}

	val := reflect.ValueOf(conversionFunctionRef)
	if val.IsValid() && val.Kind() == reflect.Func {
		args := []reflect.Value{reflect.ValueOf(entityInputDto)}
		entityModel := val.Call(args)

		result := o.db.Create(entityModel[0].Interface())
		if result.Error != nil {
			return nil, result.Error
		}

		return result.Statement.Model, nil
	}

	return 1, nil
}

func (o *genericRepository) SaveEntity(entity any) (any, error) {

	result := o.db.Save(entity)
	if result.Error != nil {
		return nil, result.Error
	}
	return result.Statement.Model, nil

}

func (r genericRepository) EditEntity(id uuid.UUID, entityName string, entity any, data any) error {

	result := r.db.Model(&entity).Where("id = ?", id).Updates(data)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

// GetEntity retrieves a single entity by ID with optional relationship preloading.
//
// Parameters:
//   - id: UUID of the entity to retrieve
//   - data: Empty instance of the entity type to query
//   - entityName: Name of the entity (for legacy preload support)
//   - includes: Slice of relation names to preload (same format as GetAllEntities)
//              If nil or empty, uses legacy getPreloadString behavior for backward compatibility
//
// Example:
//
//	// No preloading (uses legacy behavior)
//	entity, err := repo.GetEntity(id, &Course{}, "Course", nil)
//
//	// Selective preloading
//	entity, err := repo.GetEntity(id, &Course{}, "Course", []string{"Chapters", "Authors"})
func (o *genericRepository) GetEntity(id uuid.UUID, data any, entityName string, includes []string) (any, error) {

	model := reflect.New(reflect.TypeOf(data)).Interface()
	query := o.db.Model(model)

	// Apply selective preloading
	// If no includes specified, use legacy behavior for backward compatibility
	if includes == nil || len(includes) == 0 {
		queryPreloadString := ""
		getPreloadString(entityName, &queryPreloadString, true)
		if queryPreloadString != "" {
			query = query.Preload(queryPreloadString)
		}
	} else {
		query = applyIncludes(query, includes)
	}

	result := query.Find(model, id)

	if result.Error != nil {
		return nil, result.Error
	}

	return model, nil
}

func getPreloadString(entityName string, queryPreloadsString *string, firstIteration bool) {
	subEntities := ems.GlobalEntityRegistrationService.GetSubEntites(entityName)
	if len(subEntities) > 0 {
		for _, subEntity := range subEntities {
			subEntityName := reflect.TypeOf(subEntity).Name()
			resourceName := ems.Pluralize(subEntityName)
			if firstIteration {
				*queryPreloadsString = resourceName
			} else {
				*queryPreloadsString = *queryPreloadsString + "." + resourceName
			}

			getPreloadString(subEntityName, queryPreloadsString, false)
		}
	}
}

// GetAllEntities retrieves a paginated list of entities with optional filtering.
//
// Parameters:
//   - data: Empty instance of the entity type to query
//   - page: Page number (1-indexed)
//   - pageSize: Number of items per page
//   - filters: Map of field names to filter values
//
// Supported filter types:
//   - Direct fields: map[string]interface{}{"title": "Golang"}
//   - Foreign keys: map[string]interface{}{"courseId": "uuid-string"}
//   - Many-to-many: map[string]interface{}{"tagIDs": []string{"id1", "id2"}}
//   - Registered relationship filters: Complex multi-level joins
//
// Returns:
//   - Slice containing one element: a slice of entities for the requested page
//   - Total count of entities matching filters
//   - Error if database operation fails
//
// Parameters:
//   - includes: Slice of relation names to preload (e.g., ["Chapters", "Chapters.Sections"])
//              If nil or empty, no preloading is performed.
//              If contains "*", all associations are preloaded (Preload(clause.Associations))
//
// Example:
//
//	// No preloading
//	result, total, err := repo.GetAllEntities(&Course{}, 1, 20, filters, nil)
//
//	// Selective preloading
//	result, total, err := repo.GetAllEntities(&Course{}, 1, 20, filters, []string{"Chapters", "Authors"})
//
//	// Nested preloading
//	result, total, err := repo.GetAllEntities(&Course{}, 1, 20, filters, []string{"Chapters.Sections"})
//
//	// All associations (backward compatible)
//	result, total, err := repo.GetAllEntities(&Course{}, 1, 20, filters, []string{"*"})
func (o *genericRepository) GetAllEntities(data any, page int, pageSize int, filters map[string]interface{}, includes []string) ([]any, int64, error) {
	pageSlice := createEmptySliceOfCalledType(data)

	// Get entity name for relationship filters lookup
	t := reflect.TypeOf(data)
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	entityName := t.Name()

	// Start building the query
	query := o.db.Model(pageSlice)

	// Apply filters
	query = applyFilters(o.db, query, filters, data, entityName)

	// Get total count with filters applied
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Calculate offset
	offset := (page - 1) * pageSize

	// Apply pagination
	query = query.Limit(pageSize).Offset(offset)

	// Apply selective preloading
	query = applyIncludes(query, includes)

	result := query.Find(&pageSlice)

	if result.Error != nil {
		return nil, 0, result.Error
	}

	return []any{pageSlice}, total, nil
}

// GetAllEntitiesCursor retrieves entities using cursor-based pagination for efficient traversal.
//
// Unlike offset pagination which scans all skipped rows, cursor-based pagination uses
// the last seen ID to fetch the next batch, providing O(1) performance regardless of page depth.
//
// Parameters:
//   - data: Empty instance of the entity type to query
//   - cursor: Base64-encoded UUID of the last entity from previous page (empty string for first page)
//   - limit: Maximum number of items to return
//   - filters: Map of field names to filter values (same as GetAllEntities)
//   - includes: Slice of relation names to preload (same as GetAllEntities)
//
// Returns:
//   - Slice containing one element: a slice of entities for the requested cursor position
//   - nextCursor: Base64-encoded UUID for fetching the next page (empty if no more results)
//   - hasMore: Boolean indicating if more results exist
//   - Error if database operation fails
//
// Example:
//
//	// First page with includes
//	results, nextCursor, hasMore, err := repo.GetAllEntitiesCursor(&Course{}, "", 20, filters, []string{"Chapters"})
//	// Next page
//	results, nextCursor, hasMore, err := repo.GetAllEntitiesCursor(&Course{}, nextCursor, 20, filters, []string{"Chapters"})
func (o *genericRepository) GetAllEntitiesCursor(data any, cursor string, limit int, filters map[string]interface{}, includes []string) ([]any, string, bool, int64, error) {
	pageSlice := createEmptySliceOfCalledType(data)

	// Get entity name for relationship filters lookup
	t := reflect.TypeOf(data)
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	entityName := t.Name()

	// Build base query for count (without cursor, limit, or preloads)
	countQuery := o.db.Model(pageSlice)
	countQuery = applyFilters(o.db, countQuery, filters, data, entityName)

	// Get total count of all items matching filters
	var total int64
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, "", false, 0, err
	}

	// Start building the main query
	query := o.db.Model(pageSlice)

	// Apply cursor if provided
	if cursor != "" {
		// Decode base64 cursor to UUID
		decodedBytes, err := base64.StdEncoding.DecodeString(cursor)
		if err != nil {
			fmt.Printf("❌ Cursor decode error for '%s': %v\n", cursor, err)
			return nil, "", false, 0, fmt.Errorf("invalid cursor: %w", err)
		}

		// Ensure we have exactly 16 bytes for UUID
		if len(decodedBytes) != 16 {
			fmt.Printf("❌ Invalid cursor length for entity '%s': cursor='%s' decoded to %d bytes (expected 16)\n", entityName, cursor, len(decodedBytes))
			fmt.Printf("   Decoded content: %v\n", string(decodedBytes))
			return nil, "", false, 0, fmt.Errorf("invalid cursor UUID: expected 16 bytes, got %d. Cursor must be the nextCursor value from a previous response, not a manually constructed value", len(decodedBytes))
		}

		// Convert bytes directly to UUID
		var cursorID uuid.UUID
		copy(cursorID[:], decodedBytes)

		fmt.Printf("✅ Cursor pagination for %s: cursor UUID = %s\n", entityName, cursorID.String())

		// Apply cursor filter: only get entities with ID > cursor
		query = query.Where("id > ?", cursorID)
	}

	// Apply filters
	query = applyFilters(o.db, query, filters, data, entityName)

	// Fetch limit+1 to determine if there are more results
	query = query.Limit(limit + 1).Order("id ASC")

	// Apply selective preloading
	query = applyIncludes(query, includes)

	result := query.Find(&pageSlice)
	if result.Error != nil {
		return nil, "", false, 0, result.Error
	}

	// Get the slice value to check length
	// pageSlice is already a slice (from createEmptySliceOfCalledType), not a pointer
	sliceValue := reflect.ValueOf(pageSlice)
	actualCount := sliceValue.Len()

	// Check if there are more results
	hasMore := actualCount > limit

	// Trim to limit if we fetched limit+1
	if hasMore {
		sliceValue = sliceValue.Slice(0, limit)
		// Update pageSlice to the trimmed slice
		pageSlice = sliceValue.Interface()
		// Update sliceValue to point to trimmed slice for cursor generation
		sliceValue = reflect.ValueOf(pageSlice)
	}

	// Generate next cursor if there are more results
	var nextCursor string
	if hasMore && sliceValue.Len() > 0 {
		// Get the last entity's ID
		lastEntity := sliceValue.Index(sliceValue.Len() - 1)
		idField := lastEntity.FieldByName("ID")
		if idField.IsValid() && idField.Type() == reflect.TypeOf(uuid.UUID{}) {
			lastID := idField.Interface().(uuid.UUID)
			nextCursor = base64.StdEncoding.EncodeToString(lastID[:])
			fmt.Printf("📄 Generated nextCursor for %s: UUID=%s, cursor=%s\n", entityName, lastID.String(), nextCursor)
		}
	}

	fmt.Printf("📊 Cursor pagination result for %s: returned %d items, hasMore=%v, total=%d, nextCursor=%s\n", entityName, sliceValue.Len(), hasMore, total, nextCursor)
	return []any{pageSlice}, nextCursor, hasMore, total, nil
}

// applyFilters applies query filters based on the filter map
func applyFilters(db *gorm.DB, query *gorm.DB, filters map[string]interface{}, modelData any, entityName string) *gorm.DB {
	// Get model information for table name
	stmt := &gorm.Statement{DB: db}
	stmt.Parse(modelData)
	currentTable := stmt.Table

	// Get registered relationship filters for this entity
	relationshipFilters := ems.GlobalEntityRegistrationService.GetRelationshipFilters(entityName)
	relationshipFilterMap := make(map[string]entityManagementInterfaces.RelationshipFilter)
	for _, rf := range relationshipFilters {
		relationshipFilterMap[rf.FilterName] = rf
	}

	for key, value := range filters {
		// Check if this filter has a registered relationship path
		if relFilter, hasRelationship := relationshipFilterMap[key]; hasRelationship {
			// Handle relationship filter
			query = applyRelationshipFilter(query, relFilter, value, currentTable)
			continue
		}

		// Handle regular filters (existing logic)
		// Handle different value types
		switch v := value.(type) {
		case string:
			// Check if it's a many-to-many relationship filter (ends with IDs or Ids)
			if strings.HasSuffix(key, "IDs") || strings.HasSuffix(key, "Ids") {
				// Extract relation name: "courseIDs" -> "course"
				relationName := strings.TrimSuffix(strings.TrimSuffix(key, "IDs"), "Ids")
				relationTable := pluralize(relationName)
				singularRelation := strings.TrimSuffix(relationTable, "s")
				singularCurrent := strings.TrimSuffix(currentTable, "s")

				// Split comma-separated IDs
				var ids []string
				if strings.Contains(v, ",") {
					ids = strings.Split(v, ",")
				} else {
					ids = []string{v}
				}

				// For many-to-many, use EXISTS with the join table
				// Try different join table naming patterns
				// Pattern 1: singular_relation + "_" + current_table (e.g., course_chapters)
				// Pattern 2: current_singular + "_" + relation_table (e.g., chapter_courses)

				joinTable := singularRelation + "_" + currentTable
				relationFK := singularRelation + "_id"
				currentFK := singularCurrent + "_id"

				// Use raw SQL for flexibility - EXISTS is more reliable than subquery
				existsClause := "EXISTS (SELECT 1 FROM " + joinTable +
					" WHERE " + joinTable + "." + currentFK + " = " + currentTable + ".id" +
					" AND " + joinTable + "." + relationFK + " IN ?)"

				query = query.Where(existsClause, ids)
			} else if strings.HasSuffix(key, "Id") || strings.HasSuffix(key, "ID") {
				// Single ID filter - treat as direct foreign key column
				dbColumn := camelToSnake(key)

				var ids []string
				if strings.Contains(v, ",") {
					ids = strings.Split(v, ",")
				} else {
					ids = []string{v}
				}

				// Try as a direct foreign key column
				if len(ids) > 1 {
					query = query.Where(dbColumn+" IN ?", ids)
				} else {
					query = query.Where(dbColumn+" = ?", ids[0])
				}
			} else {
				// Regular field filter
				dbColumn := camelToSnake(key)

				// Check if comma-separated
				if strings.Contains(v, ",") {
					values := strings.Split(v, ",")
					query = query.Where(dbColumn+" IN ?", values)
				} else {
					query = query.Where(dbColumn+" = ?", v)
				}
			}
		case []string:
			// Array of strings
			dbColumn := camelToSnake(key)
			query = query.Where(dbColumn+" IN ?", v)
		case []interface{}:
			// Array of interfaces
			dbColumn := camelToSnake(key)
			query = query.Where(dbColumn+" IN ?", v)
		default:
			// Other types (int, bool, etc.)
			dbColumn := camelToSnake(key)
			query = query.Where(dbColumn+" = ?", v)
		}
	}
	return query
}

// applyRelationshipFilter applies filters through relationship paths
func applyRelationshipFilter(query *gorm.DB, relFilter entityManagementInterfaces.RelationshipFilter, value interface{}, currentTable string) *gorm.DB {
	// Convert value to string array
	var ids []string
	switch v := value.(type) {
	case string:
		if strings.Contains(v, ",") {
			ids = strings.Split(v, ",")
		} else {
			ids = []string{v}
		}
	case []string:
		ids = v
	case []interface{}:
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

	// Start with the first join table
	if len(relFilter.Path) == 0 {
		return query
	}

	firstStep := relFilter.Path[0]
	existsClause.WriteString(firstStep.JoinTable)
	existsClause.WriteString(" WHERE ")
	existsClause.WriteString(firstStep.JoinTable)
	existsClause.WriteString(".")
	existsClause.WriteString(firstStep.SourceColumn)
	existsClause.WriteString(" = ")
	existsClause.WriteString(currentTable)
	existsClause.WriteString(".id")

	// Add subsequent joins
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

	// Add final condition
	lastStep := relFilter.Path[len(relFilter.Path)-1]
	existsClause.WriteString(" AND ")
	existsClause.WriteString(lastStep.JoinTable)
	existsClause.WriteString(".")
	existsClause.WriteString(lastStep.TargetColumn)
	existsClause.WriteString(" IN ?")

	// Close all parentheses
	for i := 1; i < len(relFilter.Path); i++ {
		existsClause.WriteString(")")
	}
	existsClause.WriteString(")")

	return query.Where(existsClause.String(), ids)
}

// pluralize converts singular to plural form (simple version)
func pluralize(s string) string {
	s = strings.ToLower(s)
	if strings.HasSuffix(s, "s") || strings.HasSuffix(s, "x") || strings.HasSuffix(s, "z") {
		return s + "es"
	}
	if strings.HasSuffix(s, "y") {
		return strings.TrimSuffix(s, "y") + "ies"
	}
	return s + "s"
}

// camelToSnake converts camelCase to snake_case
// Special handling for ID/IDs to prevent "i_ds" conversion
func camelToSnake(s string) string {
	// Handle special cases
	s = strings.ReplaceAll(s, "IDs", "_ids")
	s = strings.ReplaceAll(s, "ID", "_id")

	var result strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			// Don't add underscore if previous char was already underscore
			if i > 0 && s[i-1] != '_' {
				result.WriteRune('_')
			}
		}
		result.WriteRune(r)
	}
	return strings.ToLower(result.String())
}

func createEmptySliceOfCalledType(data any) any {
	t := reflect.TypeOf(data)
	if t.Kind() == reflect.Ptr {
		t = t.Elem().Elem()
	}

	sliceType := reflect.SliceOf(t)
	emptySlice := reflect.MakeSlice(sliceType, 0, 0)

	return emptySlice.Interface()
}

func (o *genericRepository) DeleteEntity(id uuid.UUID, entity any, scoped bool) error {
	var result *gorm.DB
	if scoped {
		result = o.db.Delete(&entity, id)
	} else {
		result = o.db.Unscoped().Delete(&entity, id)
	}

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Entity not found",
		}
	}
	return nil
}

// applyIncludes applies selective preloading to a GORM query based on the includes parameter.
//
// This is a generic helper function that works for all entities.
//
// Parameters:
//   - query: The GORM query to apply preloads to
//   - includes: Slice of relation names to preload
//
// Supported formats:
//   - nil or empty slice: No preloading
//   - ["*"]: Load all associations (using clause.Associations)
//   - ["Relation1", "Relation2"]: Load specific top-level relations
//   - ["Relation1.Nested"]: Load nested relations (dot notation)
//   - ["Relation1.Nested.Deep"]: Multi-level nesting supported
//
// Examples:
//
//	// No preloading
//	query = applyIncludes(query, nil)
//	query = applyIncludes(query, []string{})
//
//	// All associations (backward compatible)
//	query = applyIncludes(query, []string{"*"})
//
//	// Specific relations
//	query = applyIncludes(query, []string{"Chapters", "Authors"})
//
//	// Nested relations
//	query = applyIncludes(query, []string{"Chapters.Sections", "Chapters.Sections.Pages"})
//
// Technical notes:
//   - Relation names must match the struct field names exactly (case-sensitive)
//   - GORM will automatically handle the join logic
//   - Nested relations use dot notation (e.g., "Chapters.Sections")
//   - Invalid relation names are silently ignored by GORM
func applyIncludes(query *gorm.DB, includes []string) *gorm.DB {
	// No includes specified - return query without preloading
	if includes == nil || len(includes) == 0 {
		return query
	}

	// Check for wildcard - load all associations
	for _, include := range includes {
		if include == "*" {
			return query.Preload(clause.Associations)
		}
	}

	// Apply selective preloading for each specified relation
	for _, include := range includes {
		// Trim whitespace
		include = strings.TrimSpace(include)
		if include != "" {
			// GORM's Preload handles both top-level and nested relations with dot notation
			query = query.Preload(include)
		}
	}

	return query
}
