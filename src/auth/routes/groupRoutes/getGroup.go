package groupController

import (
	"github.com/gin-gonic/gin"
)

// Get group godoc
//
//	@Summary		Récupération groupe
//	@Description	Récupération des informations du groupe
//	@Tags			groups
//	@Accept			json
//	@Produce		json
//	@Param		 	id	path		string	true	"ID group"
//	@Param Authorization header string true "Insert your access token" default(bearer <Add access token here>)
//	@Success		200	{object}	dto.GroupOutput
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		404	{object}	errors.APIError	"Groupe inexistant - Impossible de le récupérer"
//
//	@Router			/groups/{id} [get]
func (g groupController) GetGroup(ctx *gin.Context) {
	g.GetEntity(ctx)
}
