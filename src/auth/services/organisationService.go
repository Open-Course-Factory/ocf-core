package services

import (
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/repositories"

	config "soli/formations/src/configuration"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type OrganisationService interface {
	CreateOrganisation(organisationCreateDTO dto.CreateOrganisationInput, config *config.Configuration) (*dto.OrganisationOutput, error)
	EditOrganisation(editedOrganisationInput *dto.OrganisationEditInput, id uuid.UUID, isSelf bool) (*dto.OrganisationEditOutput, error)
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

func (o *organisationService) CreateOrganisation(organisationCreateDTO dto.CreateOrganisationInput, config *config.Configuration) (*dto.OrganisationOutput, error) {

	organisation, createOrganisationError := o.repository.CreateOrganisation(organisationCreateDTO)

	if createOrganisationError != nil {
		return nil, createOrganisationError
	}

	var groupOutputs []dto.GroupOutput
	for _, group := range organisation.Groups {
		groupOutputs = append(groupOutputs, *dto.GroupModelToGroupOutput(group))
	}

	return &dto.OrganisationOutput{
		ID:     organisation.ID,
		Name:   organisation.OrganisationName,
		Groups: groupOutputs,
	}, nil

}
