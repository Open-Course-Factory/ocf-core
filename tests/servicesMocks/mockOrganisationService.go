package services

import (
	"soli/formations/src/auth/dto"
	config "soli/formations/src/configuration"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
)

type MockOrganisationService struct {
	mock.Mock
}

func (m *MockOrganisationService) CreateOrganisation(organisationCreateDTO dto.CreateOrganisationInput, config *config.Configuration) (*dto.OrganisationOutput, error) {
	args := m.Called(organisationCreateDTO, config)
	return args.Get(0).(*dto.OrganisationOutput), args.Error(1)
}

func (m *MockOrganisationService) EditOrganisation(editedOrganisationInput *dto.OrganisationEditInput, id uuid.UUID) (*dto.OrganisationEditOutput, error) {
	args := m.Called(editedOrganisationInput, id)
	return args.Get(0).(*dto.OrganisationEditOutput), args.Error(1)
}
