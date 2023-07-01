package groupController

import (
	"net/http"

	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Get group godoc
//
//	@Summary		Récupération groupe
//	@Description	Récupération des informations du groupe
//	@Tags			groups
//	@Accept			json
//	@Produce		json
//	@Param		 	id	path		int	true	"ID group"
//	@Param Authorization header string true "Insert your access token" default(bearer <Add access token here>)
//	@Success		200	{object}	dto.GroupOutput
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		404	{object}	errors.APIError	"Groupe inexistant - Impossible de le récupérer"
//
//	@Router			/groups/{id} [get]
func (g groupController) GetGroup(ctx *gin.Context) {

	id, err := uuid.Parse(ctx.Param("id"))

	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: err.Error(),
		})
		return
	}

	group, groupError := g.service.GetGroup(id)

	if groupError != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotAcceptable,
			ErrorMessage: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, dto.GroupModelToGroupOutput(*group))
}
