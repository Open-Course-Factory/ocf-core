package organisationController

import (
	"net/http"
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/errors"
	"soli/formations/src/auth/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Get all organisations godoc
//
//	@Summary		Récupération organisations
//	@Description	Récupération de toutes les organisations dans la base données
//	@Tags			organisations
//	@Accept			json
//	@Produce		json
//
//	@Param Authorization header string true "Insert your access token" default(bearer <Add access token here>)
//
//	@Success		200	{object}	[]dto.OrganisationOutput
//
//	@Failure		404	{object}	errors.APIError	"Organisations inexistantes"
//
//	@Router			/organisations [get]
func (u organisationController) GetOrganisations(ctx *gin.Context) {

	var organisations []dto.OrganisationOutput
	var err error

	userId := uuid.Nil

	rawUser, ok := ctx.Get("user")

	if ok {
		userModel, isUser := rawUser.(*models.User)
		if isUser {
			userId = userModel.ID
		}
	}

	organisations, err = u.service.GetOrganisations(userId)

	if err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, organisations)
}
