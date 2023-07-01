package organisationController

import (
	"net/http"

	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Get organisation godoc
//
//	@Summary		Récupération organisation
//	@Description	Récupération des informations de l'organisation
//	@Tags			organisations
//	@Accept			json
//	@Produce		json
//	@Param		 	id	path		string	true	"ID organisation"
//	@Param Authorization header string true "Insert your access token" default(bearer <Add access token here>)
//	@Success		200	{object}	dto.OrganisationOutput
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		404	{object}	errors.APIError	"Organisation inexistante - Impossible de la récupérer"
//
//	@Router			/organisations/{id} [get]
func (u organisationController) GetOrganisation(ctx *gin.Context) {

	id, err := uuid.Parse(ctx.Param("id"))

	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: err.Error(),
		})
		return
	}

	organisation, organisationError := u.service.GetOrganisation(id)

	if organisationError != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotAcceptable,
			ErrorMessage: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, dto.OrganisationModelToOrganisationOutput(*organisation))
}
