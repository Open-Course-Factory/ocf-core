package services

import (
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/models"
	"soli/formations/src/auth/repositories"

	config "soli/formations/src/configuration"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type OrganisationService interface {
	CreateOrganisation(organisationCreateDTO dto.CreateOrganisationInput, config *config.Configuration) (*dto.OrganisationOutput, error)
	GetOrganisation(id uuid.UUID) (*models.Organisation, error)
	GetOrganisations() ([]dto.OrganisationOutput, error)
	EditOrganisation(editedOrganisationInput *dto.OrganisationEditInput, id uuid.UUID, isSelf bool) (*dto.OrganisationEditOutput, error)
	DeleteOrganisation(id uuid.UUID) error
}

type organisationService struct {
	repository repositories.OrganisationRepository
}

func NewOrganisationService(db *gorm.DB) OrganisationService {
	return &organisationService{
		repository: repositories.NewOrganisationRepository(db),
	}
}

func (o *organisationService) EditOrganisation(editedOrganisationInput *dto.OrganisationEditInput, id uuid.UUID, isSelf bool) (*dto.OrganisationEditOutput, error) {

	editOrganisation := editedOrganisationInput

	editedOrganisation, organisationError := o.repository.EditOrganisation(editOrganisation)

	if organisationError != nil {
		return nil, organisationError
	}

	return editedOrganisation, nil
}

func (o *organisationService) DeleteOrganisation(id uuid.UUID) error {
	errorDelete := o.repository.DeleteOrganisation(id)
	if errorDelete != nil {
		return errorDelete
	}
	return nil
}

func (o *organisationService) GetOrganisation(id uuid.UUID) (*models.Organisation, error) {
	organisation, err := o.repository.GetOrganisation(id)

	if err != nil {
		return nil, err
	}

	return organisation, nil

}

func (o *organisationService) GetOrganisations() ([]dto.OrganisationOutput, error) {

	organisationModel, err := o.repository.GetAllOrganisations()

	if err != nil {
		return nil, err
	}

	var organisationsDto []dto.OrganisationOutput

	for _, s := range organisationModel {
		organisationsDto = append(organisationsDto, *dto.OrganisationModelToOrganisationOutput(*s))
	}

	return organisationsDto, nil
}

func (o *organisationService) CreateOrganisation(organisationCreateDTO dto.CreateOrganisationInput, config *config.Configuration) (*dto.OrganisationOutput, error) {

	organisation, createOrganisationError := o.repository.CreateOrganisation(organisationCreateDTO)

	if createOrganisationError != nil {
		return nil, createOrganisationError
	}

	return &dto.OrganisationOutput{
		ID:     organisation.ID,
		Name:   organisation.OrganisationName,
		Groups: organisation.Groups,
	}, nil

}
