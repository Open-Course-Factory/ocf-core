package repositories

import (
	"encoding/base64"
	"fmt"
	"reflect"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityErrors "soli/formations/src/entityManagement/errors"
	"soli/formations/src/entityManagement/repositories/filters"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type GenericRepository interface {
	CreateEntity(data any, entityName string) (any, error)
	CreateEntityFromModel(entityModel any) (any, error)
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

// getFilterManager creates a filter manager configured for the given entity.
// It retrieves registered relationship filters for the entity and initializes
// the manager with all standard filter strategies.
func (o *genericRepository) getFilterManager(entityName string) *filters.FilterManager {
	relationshipFilters := ems.GlobalEntityRegistrationService.GetRelationshipFilters(entityName)
	manager := filters.NewFilterManager(relationshipFilters)

	// Add custom organization member filter for user-based access control
	if entityName == "Organization" {
		manager.AddStrategy(&filters.OrganizationMemberFilter{})
	}

	return manager
}

// getTableName extracts the table name from a model instance using GORM's parser.
func (o *genericRepository) getTableName(modelData any) string {
	stmt := &gorm.Statement{DB: o.db}
	stmt.Parse(modelData)
	return stmt.Table
}

func (o *genericRepository) CreateEntity(entityInputDto any, entityName string) (any, error) {
	conversionFunctionRef, found := ems.GlobalEntityRegistrationService.GetConversionFunction(entityName, ems.CreateInputDtoToModel)

	if !found {
		return nil, entityErrors.NewConversionError(entityName, "conversion function does not exist")
	}

	val := reflect.ValueOf(conversionFunctionRef)
	if val.IsValid() && val.Kind() == reflect.Func {
		args := []reflect.Value{reflect.ValueOf(entityInputDto)}
		entityModel := val.Call(args)

		result := o.db.Create(entityModel[0].Interface())
		if result.Error != nil {
			return nil, entityErrors.WrapDatabaseError(result.Error, "create entity")
		}

		return result.Statement.Model, nil
	}

	return 1, nil
}

func (o *genericRepository) CreateEntityFromModel(entityModel any) (any, error) {
	result := o.db.Create(entityModel)
	if result.Error != nil {
		return nil, entityErrors.WrapDatabaseError(result.Error, "create entity from model")
	}

	return result.Statement.Model, nil
}

func (o *genericRepository) SaveEntity(entity any) (any, error) {
	result := o.db.Save(entity)
	if result.Error != nil {
		return nil, entityErrors.WrapDatabaseError(result.Error, "save entity")
	}
	return result.Statement.Model, nil
}

func (r genericRepository) EditEntity(id uuid.UUID, entityName string, entity any, data any) error {
	result := r.db.Model(&entity).Where("id = ?", id).Updates(data)
	if result.Error != nil {
		return entityErrors.WrapDatabaseError(result.Error, "update entity")
	}
	if result.RowsAffected == 0 {
		return entityErrors.NewEntityNotFound(entityName, id)
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
//     If nil or empty, uses legacy getPreloadString behavior for backward compatibility
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
		if result.Error == gorm.ErrRecordNotFound {
			return nil, entityErrors.NewEntityNotFound(entityName, id)
		}
		return nil, entityErrors.WrapDatabaseError(result.Error, "get entity")
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
//     If nil or empty, no preloading is performed.
//     If contains "*", all associations are preloaded (Preload(clause.Associations))
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

	// Apply filters using filter manager
	filterManager := o.getFilterManager(entityName)
	tableName := o.getTableName(data)
	query = filterManager.ApplyFilters(query, filters, tableName)

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
		return nil, 0, entityErrors.WrapDatabaseError(result.Error, "get all entities")
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
	filterManager := o.getFilterManager(entityName)
	tableName := o.getTableName(data)
	countQuery = filterManager.ApplyFilters(countQuery, filters, tableName)

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
			return nil, "", false, 0, entityErrors.NewInvalidCursorError(cursor, "failed to decode base64")
		}

		// Ensure we have exactly 16 bytes for UUID
		if len(decodedBytes) != 16 {
			fmt.Printf("❌ Invalid cursor length for entity '%s': cursor='%s' decoded to %d bytes (expected 16)\n", entityName, cursor, len(decodedBytes))
			fmt.Printf("   Decoded content: %v\n", string(decodedBytes))
			return nil, "", false, 0, entityErrors.NewInvalidCursorError(cursor, fmt.Sprintf("expected 16 bytes, got %d", len(decodedBytes)))
		}

		// Convert bytes directly to UUID
		var cursorID uuid.UUID
		copy(cursorID[:], decodedBytes)

		fmt.Printf("✅ Cursor pagination for %s: cursor UUID = %s\n", entityName, cursorID.String())

		// Apply cursor filter: only get entities with ID > cursor
		query = query.Where("id > ?", cursorID)
	}

	// Apply filters using filter manager (reuse from count query above)
	query = filterManager.ApplyFilters(query, filters, tableName)

	// Fetch limit+1 to determine if there are more results
	query = query.Limit(limit + 1).Order("id ASC")

	// Apply selective preloading
	query = applyIncludes(query, includes)

	result := query.Find(&pageSlice)
	if result.Error != nil {
		return nil, "", false, 0, entityErrors.WrapDatabaseError(result.Error, "get entities with cursor")
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
		return entityErrors.WrapDatabaseError(result.Error, "delete entity")
	}
	if result.RowsAffected == 0 {
		// Get entity name from type
		entityType := reflect.TypeOf(entity)
		if entityType.Kind() == reflect.Ptr {
			entityType = entityType.Elem()
		}
		entityName := entityType.Name()
		return entityErrors.NewEntityNotFound(entityName, id)
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
