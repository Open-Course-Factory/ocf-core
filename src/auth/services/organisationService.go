package services

import (
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/models"
	"soli/formations/src/auth/repositories"
	sqldb "soli/formations/src/db"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type OrganisationService interface {
	CreateOrganisation(organisationCreateDTO dto.CreateOrganisationInput) (*dto.OrganisationOutput, error)
	EditOrganisation(editedOrganisationInput *dto.OrganisationEditInput, id uuid.UUID) (*dto.OrganisationEditOutput, error)
	CreateOrganisationComplete(name string, userID uuid.UUID) (*dto.OrganisationOutput, error)
}

type organisationService struct {
	repository repositories.OrganisationRepository
}

func NewOrganisationService(db *gorm.DB) OrganisationService {
	return &organisationService{
		repository: repositories.NewOrganisationRepository(db),
	}
}

func (o *organisationService) EditOrganisation(editedOrganisationInput *dto.OrganisationEditInput, id uuid.UUID) (*dto.OrganisationEditOutput, error) {

	editOrganisation := editedOrganisationInput

	editedOrganisation, organisationError := o.repository.EditOrganisation(editOrganisation)

	if organisationError != nil {
		return nil, organisationError
	}

	return editedOrganisation, nil
}

func (o *organisationService) CreateOrganisation(organisationCreateDTO dto.CreateOrganisationInput) (*dto.OrganisationOutput, error) {

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

func (o *organisationService) CreateOrganisationComplete(name string, userID uuid.UUID) (*dto.OrganisationOutput, error) {

	organisationInput := dto.CreateOrganisationInput{Name: name}
	organisationOutputDto, createOrgError := o.CreateOrganisation(organisationInput)

	if createOrgError != nil {
		return nil, createOrgError
	}

	roleService := NewRoleService(sqldb.DB)
	roleOrganisationAdminId, getRoleError := roleService.GetRoleByType(models.RoleTypeOrganisationAdmin)

	if getRoleError != nil {
		return nil, getRoleError
	}

	roleService.CreateUserRoleObjectAssociation(userID, roleOrganisationAdminId, organisationOutputDto.ID, "Organisation")
	return organisationOutputDto, nil
}
