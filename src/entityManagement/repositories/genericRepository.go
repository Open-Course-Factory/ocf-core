package repositories

import (
	"reflect"
	ems "soli/formations/src/entityManagement/entityManagementService"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type GenericRepository interface {
	CreateEntity(data interface{}, entityName string) (interface{}, error)
	GetEntity(id uuid.UUID, data interface{}) (interface{}, error)
	GetAllEntities(data interface{}, pageSize int) ([]interface{}, error)
	DeleteEntity(id uuid.UUID, data interface{}) error
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

func (o *genericRepository) CreateEntity(entityInputDto interface{}, entityName string) (interface{}, error) {
	conversionFunctionRef, _ := ems.GlobalEntityRegistrationService.GetConversionFunction(entityName, ems.InputDtoToModel)
	val := reflect.ValueOf(conversionFunctionRef)
	if val.IsValid() && val.Kind() == reflect.Func {
		args := []reflect.Value{reflect.ValueOf(entityInputDto)}
		entityModel := val.Call(args)

		// test, _ := ems.GlobalEntityRegistrationService.GetEntityInterface(entityName)
		// testIface := reflect.TypeOf(test)

		// entity := reflect.ValueOf(entityModel)
		// iface := entity.Interface().(test)
		// fmt.Println(iface)
		// fmt.Println(reflect.TypeOf(entityModel[0].Interface()))

		// err := mapstructure.Decode(entityModel[0], &test)
		// if err != nil {
		// 	fmt.Println(err.Error())
		// }

		result := o.db.Create(entityModel[0].Interface())
		if result.Error != nil {
			return nil, result.Error
		}

		return entityModel[0].Interface(), nil
	}

	return 1, nil
}

func (o *genericRepository) GetEntity(id uuid.UUID, data interface{}) (interface{}, error) {
	model := reflect.New(reflect.TypeOf(data)).Interface()

	result := o.db.First(model, id)

	if result.Error != nil {
		return nil, result.Error
	}

	return model, nil
}

func (o *genericRepository) GetAllEntities(data interface{}, pageSize int) ([]interface{}, error) {
	var allPages []interface{}

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

func createEmptySliceOfCalledType(data interface{}) any {
	t := reflect.TypeOf(data)
	if t.Kind() == reflect.Ptr {
		t = t.Elem().Elem()
	}

	sliceType := reflect.SliceOf(t)
	emptySlice := reflect.MakeSlice(sliceType, 0, 0)

	return emptySlice.Interface()
}

func (o *genericRepository) DeleteEntity(id uuid.UUID, data interface{}) error {

	model := reflect.New(reflect.TypeOf(data)).Interface()
	result := o.db.Delete(&model, id)
	if result.Error != nil {
		return result.Error
	}
	return nil
}
