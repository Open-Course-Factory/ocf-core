package services

import (
	"fmt"
	"reflect"
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
	DeleteEntity(id uuid.UUID, entity interface{}) error
	GetEntityModelInterface(entityName string) interface{}
	AddOwnerIDs(entity interface{}, userId string) (interface{}, error)
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

func (g *genericService) DeleteEntity(id uuid.UUID, entity interface{}) error {
	errorDelete := g.genericRepository.DeleteEntity(id, entity)
	if errorDelete != nil {
		return errorDelete
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
