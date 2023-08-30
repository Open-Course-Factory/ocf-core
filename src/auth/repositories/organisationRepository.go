package repositories

import (
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/models"

	"gorm.io/gorm"
)

type OrganisationRepository interface {
	CreateOrganisation(organisationdto dto.CreateOrganisationInput) (*models.Organisation, error)
	EditOrganisation(organisation *dto.OrganisationEditInput) (*dto.OrganisationEditOutput, error)
}

type organisationRepository struct {
	db *gorm.DB
}

func NewOrganisationRepository(db *gorm.DB) OrganisationRepository {
	repository := &organisationRepository{
		db: db,
	}
	return repository
}

func (r *organisationRepository) CreateOrganisation(organisationdto dto.CreateOrganisationInput) (*models.Organisation, error) {
	organisation := models.Organisation{
		OrganisationName: organisationdto.Name,
	}
	err := r.db.Create(&organisation).Error
	if err != nil {
		return nil, err
		//return nil, err
	}
	return &organisation, nil
}

func (o *organisationRepository) EditOrganisation(organisation *dto.OrganisationEditInput) (*dto.OrganisationEditOutput, error) {
	result := o.db.Save(&organisation)
	if result.Error != nil {
		return nil, result.Error
	}
	return &dto.OrganisationEditOutput{
		Name: organisation.Name,
	}, nil
}
