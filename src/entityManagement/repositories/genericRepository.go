package repositories

import (
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
	GetEntity(id uuid.UUID, data any, entityName string) (any, error)
	GetAllEntities(data any, page int, pageSize int, filters map[string]interface{}) ([]any, int64, error)
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

func (o *genericRepository) GetEntity(id uuid.UUID, data any, entityName string) (any, error) {

	model := reflect.New(reflect.TypeOf(data)).Interface()
	query := o.db.Model(model)

	queryPreloadString := ""
	getPreloadString(entityName, &queryPreloadString, true)

	if queryPreloadString != "" {
		query.Preload(queryPreloadString)
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
// Example:
//
//	result, total, err := repo.GetAllEntities(
//	    &Course{},
//	    1,
//	    20,
//	    map[string]interface{}{"published": true},
//	)
func (o *genericRepository) GetAllEntities(data any, page int, pageSize int, filters map[string]interface{}) ([]any, int64, error) {
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

	// Apply pagination and preload associations
	query = query.Limit(pageSize).Offset(offset).Preload(clause.Associations)

	result := query.Find(&pageSlice)

	if result.Error != nil {
		return nil, 0, result.Error
	}

	return []any{pageSlice}, total, nil
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
