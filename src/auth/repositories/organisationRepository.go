package repositories

import (
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type OrganisationRepository interface {
	CreateOrganisation(organisationdto dto.CreateOrganisationInput) (*models.Organisation, error)
	GetOrganisation(id uuid.UUID) (*models.Organisation, error)
	GetAllOrganisations() ([]*models.Organisation, error)
	DeleteOrganisation(id uuid.UUID) error
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

func (o *organisationRepository) GetOrganisation(id uuid.UUID) (*models.Organisation, error) {
	var organisation models.Organisation
	result := o.db.Preload("Groups").First(&organisation, id)

	if result.Error != nil {
		return nil, result.Error
	}

	return &organisation, nil
}

func (o *organisationRepository) GetAllOrganisations() ([]*models.Organisation, error) {
	var organisations []*models.Organisation
	result := o.db.Preload("Groups").Find(&organisations)
	if result.Error != nil {
		return nil, result.Error
	}

	return organisations, nil
}

func (o *organisationRepository) DeleteOrganisation(id uuid.UUID) error {
	result := o.db.Delete(&models.Organisation{}, id)
	if result.Error != nil {
		return result.Error
	}
	return nil
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
