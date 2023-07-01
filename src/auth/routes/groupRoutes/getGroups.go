package groupController

import (
	"net/http"
	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

// Get all groups godoc
//
//	@Summary		Récupération groupes
//	@Description	Récupération de tous les groupes dans la base données
//	@Tags			groups
//	@Accept			json
//	@Produce		json
//
//	@Param Authorization header string true "Insert your access token" default(bearer <Add access token here>)
//
//	@Success		200	{object}	[]dto.GroupOutput
//
//	@Failure		404	{object}	errors.APIError	"Groupes inexistants"
//
//	@Router			/groups [get]
func (g groupController) GetGroups(ctx *gin.Context) {

	groups, err := g.service.GetGroups()

	if err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Group not found",
		})
		return
	}

	ctx.JSON(http.StatusOK, groups)
}
