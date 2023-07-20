package services

import (
	"soli/formations/src/auth/repositories"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type GenericService interface {
	GetEntity(id uuid.UUID, data interface{}) (interface{}, error)
	GetEntities(data interface{}) ([]interface{}, error)
	DeleteEntity(id uuid.UUID, data interface{}) error
}

type genericService struct {
	genericRepository repositories.EntityRepository
}

func NewGenericService(db *gorm.DB) GenericService {
	return &genericService{
		genericRepository: repositories.NewGenericRepository(db),
	}
}

func (g genericService) GetEntity(id uuid.UUID, data interface{}) (interface{}, error) {
	entity, err := g.genericRepository.GetEntity(id, data)

	if err != nil {
		return nil, err
	}

	return entity, nil

}

// should return an array of dtoEntityOutput
func (g genericService) GetEntities(data interface{}) ([]interface{}, error) {

	allPages, err := g.genericRepository.GetAllEntities(data, 20)

	if err != nil {
		return nil, err
	}

	return allPages, nil
}

func (g genericService) DeleteEntity(id uuid.UUID, data interface{}) error {
	errorDelete := g.genericRepository.DeleteEntity(id, data)
	if errorDelete != nil {
		return errorDelete
	}
	return nil
}
