package repositories

import (
	"net/http"
	"reflect"
	errors "soli/formations/src/auth/errors"
	ems "soli/formations/src/entityManagement/entityManagementService"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type GenericRepository interface {
	CreateEntity(data any, entityName string) (any, error)
	SaveEntity(entity any) (any, error)
	GetEntity(id uuid.UUID, data any, entityName string) (any, error)
	GetAllEntities(data any, pageSize int) ([]any, error)
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
			ErrorMessage: "Entity convertion function does not exist",
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

func (o *genericRepository) GetAllEntities(data any, pageSize int) ([]any, error) {
	var allPages []any

	page := 1
	for {
		pageSlice := createEmptySliceOfCalledType(data)

		offset := (page - 1) * pageSize
		query := o.db.Limit(pageSize).Offset(offset)
		// ToDo : update gorm when merged, works with the patch : https://github.com/go-gorm/gorm/pull/6417
		query = query.Preload(clause.Associations)

		result := query.Find(&pageSlice)

		// Fetch a page of records
		if result.Error != nil {
			return nil, result.Error
		}

		// If no more records found, break the loop
		if result.RowsAffected == 0 {
			break
		}

		// Append the entities from pagesToFill to the allEntities
		allPages = append(allPages, pageSlice)

		page++
	}

	return allPages, nil
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
