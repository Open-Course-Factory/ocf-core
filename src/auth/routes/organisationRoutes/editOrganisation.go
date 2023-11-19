package organisationController

import (
	"net/http"

	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/errors"

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
//	@Param			id	path		string	true	"ID organisation"
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

	edit, editOrganisationError := u.service.EditOrganisation(editOrganisation, id)

	if editOrganisationError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: editOrganisationError.Error()})
		return
	}
	ctx.JSON(http.StatusNoContent, edit)
}
