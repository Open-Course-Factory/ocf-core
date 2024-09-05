package services

import (
	"fmt"
	"reflect"
	"soli/formations/src/auth/casdoor"
	"soli/formations/src/entityManagement/repositories"

	ems "soli/formations/src/entityManagement/entityManagementService"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type GenericService interface {
	CreateEntity(inputDto interface{}, entityName string) (interface{}, error)
	SaveEntity(entity interface{}) (interface{}, error)
	GetEntity(id uuid.UUID, data interface{}) (interface{}, error)
	GetEntities(data interface{}) ([]interface{}, error)
	DeleteEntity(id uuid.UUID, entity interface{}, scoped bool) error
	EditEntity(id uuid.UUID, entityName string, entity interface{}, data interface{}) error
	GetEntityModelInterface(entityName string) interface{}
	AddOwnerIDs(entity interface{}, userId string) (interface{}, error)
	ExtractUuidFromReflectEntity(entity interface{}) uuid.UUID
	GetDtoArrayFromEntitiesPages(allEntitiesPages []interface{}, entityModelInterface interface{}, entityName string) ([]interface{}, bool)
	GetEntityFromResult(entityName string, item interface{}) (interface{}, bool)
	AddDefaultAccessesForEntity(resourceName string, entity interface{}, userId string) error
}

type genericService struct {
	genericRepository repositories.GenericRepository
}

func NewGenericService(db *gorm.DB) GenericService {
	return &genericService{
		genericRepository: repositories.NewGenericRepository(db),
	}
}

func (g *genericService) CreateEntity(inputDto interface{}, entityName string) (interface{}, error) {

	entity, createEntityError := g.genericRepository.CreateEntity(inputDto, entityName)
	if createEntityError != nil {
		return nil, createEntityError
	}

	return entity, nil
}

func (g *genericService) SaveEntity(entity interface{}) (interface{}, error) {

	entity, saveEntityError := g.genericRepository.SaveEntity(entity)
	if saveEntityError != nil {
		return nil, saveEntityError
	}

	return entity, nil
}

func (g *genericService) GetEntity(id uuid.UUID, data interface{}) (interface{}, error) {
	entity, err := g.genericRepository.GetEntity(id, data)

	if err != nil {
		return nil, err
	}

	return entity, nil

}

// should return an array of dtoEntityOutput
func (g *genericService) GetEntities(data interface{}) ([]interface{}, error) {

	allPages, err := g.genericRepository.GetAllEntities(data, 20)

	if err != nil {
		return nil, err
	}

	return allPages, nil
}

func (g *genericService) DeleteEntity(id uuid.UUID, entity interface{}, scoped bool) error {
	errorDelete := g.genericRepository.DeleteEntity(id, entity, scoped)
	if errorDelete != nil {
		return errorDelete
	}
	return nil
}

func (g *genericService) EditEntity(id uuid.UUID, entityName string, entity interface{}, data interface{}) error {
	errorPatch := g.genericRepository.EditEntity(id, entityName, entity, data)
	if errorPatch != nil {
		return errorPatch
	}
	return nil
}

func (g *genericService) GetEntityModelInterface(entityName string) interface{} {
	var result interface{}
	result, _ = ems.GlobalEntityRegistrationService.GetEntityInterface(entityName)
	return result
}

func (g *genericService) AddOwnerIDs(entity interface{}, userId string) (interface{}, error) {
	entityReflectValue := reflect.ValueOf(entity).Elem()
	ownerIdsField := entityReflectValue.FieldByName("OwnerIDs")
	if ownerIdsField.IsValid() {

		if ownerIdsField.CanSet() {

			fmt.Println(ownerIdsField.Kind())
			if ownerIdsField.Kind() == reflect.Slice {
				ownerIdsField.Set(reflect.MakeSlice(ownerIdsField.Type(), 1, 1))
				ownerIdsField.Index(0).Set(reflect.ValueOf(userId))

				entityWithOwnerIds, entitySavingError := g.SaveEntity(entity)

				if entitySavingError != nil {
					return nil, entitySavingError
				}

				entity = entityWithOwnerIds
			}
		}

	}
	return entity, nil
}

func (g *genericService) ExtractUuidFromReflectEntity(entity interface{}) uuid.UUID {
	entityReflectValue := reflect.ValueOf(entity).Elem()
	field := entityReflectValue.FieldByName("ID")

	var mon_uuid uuid.UUID

	result, ok := field.Interface().(string)
	if ok {
		mon_uuid, _ = uuid.Parse(result)
	} else {
		mon_uuid = uuid.UUID(field.Bytes())
	}

	return mon_uuid
}

func (g *genericService) GetDtoArrayFromEntitiesPages(allEntitiesPages []interface{}, entityModelInterface interface{}, entityName string) ([]interface{}, bool) {
	var entitiesDto []interface{}
	entitiesDto = []interface{}{}

	for _, page := range allEntitiesPages {

		entityModel := reflect.SliceOf(reflect.TypeOf(entityModelInterface))

		pageValue := reflect.ValueOf(page)

		if pageValue.Type().ConvertibleTo(entityModel) {
			convertedPage := pageValue.Convert(entityModel)

			for i := 0; i < convertedPage.Len(); i++ {

				item := convertedPage.Index(i).Interface()

				var shouldReturn bool
				entitiesDto, shouldReturn = g.appendEntityFromResult(entityName, item, entitiesDto)
				if shouldReturn {
					return nil, true
				}
			}
		} else {
			return nil, true
		}

	}
	return entitiesDto, false
}

// used in get
func (g *genericService) appendEntityFromResult(entityName string, item interface{}, entitiesDto []interface{}) ([]interface{}, bool) {
	result, ko := g.GetEntityFromResult(entityName, item)
	if !ko {
		entitiesDto = append(entitiesDto, result)
		return entitiesDto, false
	}

	return nil, true
}

// used in post and get
func (g *genericService) GetEntityFromResult(entityName string, item interface{}) (interface{}, bool) {
	var result interface{}
	if funcRef, ok := ems.GlobalEntityRegistrationService.GetConversionFunction(entityName, ems.OutputModelToDto); ok {
		val := reflect.ValueOf(funcRef)

		if val.IsValid() && val.Kind() == reflect.Func {
			args := []reflect.Value{reflect.ValueOf(item)}
			entityDto := val.Call(args)
			if len(entityDto) == 1 {
				result = entityDto[0].Interface()
			}

		} else {
			return nil, true
		}
	} else {
		return nil, true
	}
	return result, false
}

func (g *genericService) AddDefaultAccessesForEntity(resourceName string, entity interface{}, userId string) error {
	errPolicyLoading := casdoor.Enforcer.LoadPolicy()
	if errPolicyLoading != nil {
		return errPolicyLoading
	}

	entityUuid := g.ExtractUuidFromReflectEntity(entity)

	_, errAddingPolicy := casdoor.Enforcer.AddPolicy(userId, "/api/v1/"+resourceName+"/"+entityUuid.String(), "(GET|DELETE|PATCH|PUT)")
	if errAddingPolicy != nil {
		return errAddingPolicy
	}

	return nil
}
