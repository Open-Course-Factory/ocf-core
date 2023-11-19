package dto

import (
	"soli/formations/src/auth/models"

	"github.com/google/uuid"
)

type OrganisationOutput struct {
	ID     uuid.UUID     `json:"id"`
	Name   string        `json:"name"`
	Groups []GroupOutput `json:"groups"`
}

type CreateOrganisationInput struct {
	Name string `binding:"required"`
}

type OrganisationEditInput struct {
	Name   string         `json:"name"`
	Groups []models.Group `json:"groups"`
}

type OrganisationEditOutput struct {
	Name   string        `json:"name"`
	Groups []GroupOutput `json:"groups"`
}

func OrganisationModelToOrganisationOutput(OrganisationModel models.Organisation) *OrganisationOutput {

	var groupOutputs []GroupOutput

	for _, group := range OrganisationModel.Groups {
		groupOutputs = append(groupOutputs, *GroupModelToGroupOutput(group))
	}

	return &OrganisationOutput{
		ID:     OrganisationModel.ID,
		Name:   OrganisationModel.OrganisationName,
		Groups: groupOutputs,
	}
}
