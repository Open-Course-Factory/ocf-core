package services

import (
	"soli/formations/src/entityManagement/repositories"

	ems "soli/formations/src/entityManagement/entityManagementService"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type GenericService interface {
	CreateEntity(inputDto interface{}, entityName string) (interface{}, error)
	GetEntity(id uuid.UUID, data interface{}) (interface{}, error)
	GetEntities(data interface{}) ([]interface{}, error)
	DeleteEntity(id uuid.UUID, data interface{}) error
	GetEntityModelInterface(entityName string) interface{}
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

	entity, creatEntityError := g.genericRepository.CreateEntity(inputDto, entityName)
	if creatEntityError != nil {
		return nil, creatEntityError
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

func (g *genericService) DeleteEntity(id uuid.UUID, data interface{}) error {
	errorDelete := g.genericRepository.DeleteEntity(id, data)
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
