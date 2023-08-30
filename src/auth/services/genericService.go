package services

import (
	"soli/formations/src/auth/models"
	"soli/formations/src/auth/repositories"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type GenericService interface {
	GetEntity(id uuid.UUID, data interface{}) (interface{}, error)
	GetEntities(data interface{}) ([]interface{}, error)
	DeleteEntity(id uuid.UUID, data interface{}) error
	IsUserInstanceAdmin(userRoleObjectAssociations *[]models.UserRoles) bool
	IsUserOrganisationAdmin(userRoles *[]models.UserRoles, organisation *models.Organisation) bool
	GetEntityModelInterface(entityName string) interface{}
	GetObjectOrganisation(entityModel string, object any) *models.Organisation
}

type genericService struct {
	genericRepository repositories.GenericRepository
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

func (g genericService) IsUserInstanceAdmin(userRoles *[]models.UserRoles) bool {
	var proceed bool
	for _, userRoleObjectAssociation := range *userRoles {
		if userRoleObjectAssociation.Role.RoleName == "instance_admin" {
			proceed = true
			break
		}
	}
	return proceed
}

func (g genericService) IsUserOrganisationAdmin(userRoles *[]models.UserRoles, organisation *models.Organisation) bool {
	var proceed bool

	for _, userRoleObjectAssociation := range *userRoles {
		if userRoleObjectAssociation.SubType == "Organisation" && userRoleObjectAssociation.SubObjectID.String() == organisation.ID.String() && userRoleObjectAssociation.Role.RoleName == "organisation_admin" {
			proceed = true
			break
		}
	}

	return proceed
}

func (g genericService) GetEntityModelInterface(entityName string) interface{} {
	var result interface{}
	switch entityName {
	case "Role":
		result = models.Role{}
	case "User":
		result = models.User{}
	case "Group":
		result = models.Group{}
	case "Organisation":
		result = models.Organisation{}
	}
	return result
}

func (g genericService) GetObjectOrganisation(entityModel string, object any) *models.Organisation {
	var organisation models.Organisation

	switch g.GetEntityModelInterface(entityModel).(type) {
	case models.Organisation:
		if value, ok := object.(models.Organisation); ok {
			object = &value
		}
		org := object.(*models.Organisation)
		organisation = *org
	case models.Group:
		if value, ok := object.(models.Group); ok {
			object = &value
		}
		group := object.(*models.Group)

		orgEntity, err := g.GetEntity(*group.OrganisationID, g.GetEntityModelInterface("Organisation"))
		if err != nil {
			return &organisation
		}
		orgPtr := orgEntity.(*models.Organisation)
		organisation = *orgPtr
	}

	return &organisation
}
