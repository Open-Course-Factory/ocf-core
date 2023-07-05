package organisationController

import (
	"net/http"

	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

// Add Organisation godoc
//
//	@Summary		Création organisation
//	@Description	Ajoute une nouvelle organisation dans la base de données
//	@Tags			organisations
//	@Accept			json
//	@Produce		json
//	@Param			organisation	body		dto.CreateOrganisationInput	true	"organisation"
//	@Param Authorization header string true "Insert your access token" default(bearer <Add access token here>)
//	@Success		201		{object}	dto.OrganisationOutput
//
//	@Failure		400		{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		400		{object}	errors.APIError	"Impossible de créer une organisation"
//	@Router			/organisations [post]
func (organisationController organisationController) AddOrganisation(ctx *gin.Context) {
	organisationCreateDTO := dto.CreateOrganisationInput{}

	bindError := ctx.BindJSON(&organisationCreateDTO)
	if bindError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Impossible de parser le json",
		})
		return
	}

	organisation, organisationError := organisationController.service.CreateOrganisation(organisationCreateDTO, organisationController.config)

	if organisationError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: organisationError.Error(),
		})
		return
	}

	ctx.JSON(http.StatusCreated, organisation)
}
