package organisationController

import (
	"net/http"

	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/errors"
	"soli/formations/src/auth/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Edit organisation godoc
//
//	@Summary		Modification organisation (Admin)
//	@Description	Modification d'une organisation dans la base de données par un administrateur
//	@Tags			organisations
//	@Accept			json
//	@Produce		json
//	@Param			id	path		int	true	"ID organisation"
//	@Param 			organisation 	body	dto.OrganisationEditInput	true	"Utilisateur"
//	@Param Authorization header string true "Insert your access token" default(bearer <Add access token here>)
//	@Success		204	{object}	string
//
//	@Failure		403	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		400	{object}	errors.APIError	"Impossible de récupérer l'ID organisation"
//	@Failure		400	{object}	errors.APIError	"Impossible de modifier l'organisation"
//
//	@Router			/organisations/{id} [put]
func (u organisationController) EditOrganisation(ctx *gin.Context) {

	editOrganisation := &dto.OrganisationEditInput{}
	id, errParse := uuid.Parse(ctx.Param("id"))

	errBind := ctx.BindJSON(&editOrganisation)

	if errParse != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: errParse.Error(),
		})
		return
	}

	if errBind != nil {
		ctx.JSON(http.StatusForbidden,
			&errors.APIError{ErrorCode: http.StatusForbidden, ErrorMessage: errBind.Error()})
		return
	}

	edit, editOrganisationError := u.service.EditOrganisation(editOrganisation, id, false)

	if editOrganisationError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: editOrganisationError.Error()})
		return
	}
	ctx.JSON(http.StatusNoContent, edit)
}

// Edit organisation self godoc
//
//	@Summary		Modification organisation
//	@Description	Modification des informations de l'organisation par lui même dans la base de données
//	@Tags			organisations
//	@Accept			json
//	@Produce		json
//	@Success		204	{object}	string
//	@Param 			organisation 	body	dto.OrganisationEditInput	true	"Utilisateur"
//	@Param Authorization header string true "Insert your access token" default(bearer <Add access token here>)
//	@Failure		403	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		404	{object}	errors.APIError	"Impossible de récupérer les informations de l'organisation"
//	@Failure		400	{object}	errors.APIError	"Impossible de modifier l'organisation"
//
//	@Router			/organisations [patch]
func (u organisationController) EditOrganisationSelf(ctx *gin.Context) {

	editOrganisation := &dto.OrganisationEditInput{}

	rawOrganisation, ok := ctx.Get("organisation")

	if !ok {
		return
	}

	organisationModel, isOrganisation := rawOrganisation.(*models.Organisation)

	if !isOrganisation {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Error check organisation"})
		return
	}

	errBind := ctx.BindJSON(&editOrganisation)

	if errBind != nil {
		ctx.JSON(http.StatusForbidden,
			&errors.APIError{ErrorCode: http.StatusForbidden, ErrorMessage: errBind.Error()})
		return
	}

	edit, editOrganisationError := u.service.EditOrganisation(editOrganisation, organisationModel.ID, true)

	if editOrganisationError != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: editOrganisationError.Error()})
		return
	}
	ctx.JSON(http.StatusNoContent, edit)
}
