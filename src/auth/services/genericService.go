package services

import (
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/models"
	"soli/formations/src/auth/repositories"

	"gorm.io/gorm"
)

type GenericService interface {
	//GetRole(id uuid.UUID) (*models.Role, error)
	GetEntities(data interface{}) ([]interface{}, error)
	//DeleteRole(id uuid.UUID) error
}

type genericService struct {
	genericRepository repositories.EntityRepository
}

func NewGenericService(db *gorm.DB) GenericService {
	return &genericService{
		genericRepository: repositories.NewGenericRepository(db),
	}
}

// should return an array of dtoEntityOutput
func (g genericService) GetEntities(data interface{}) ([]interface{}, error) {

	var entitiesDto []interface{}

	allPages, err := g.genericRepository.GetAllEntities(data, 20)

	if err != nil {
		return nil, err
	}

	// Here we need to loop through the pages
	for _, page := range allPages {
		test := page.(*[]models.Role)

		for _, s := range *test {
			entitiesDto = append(entitiesDto, *dto.RoleModelToRoleOutput(s))
		}
	}

	return entitiesDto, nil
}
