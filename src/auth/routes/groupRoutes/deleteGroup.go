package groupController

import (
	"github.com/gin-gonic/gin"
)

// Delete user godoc
//
//	@Summary		Suppression groupe
//	@Description	Suppression d'un groupe dans la base de données
//	@Tags			groups
//	@Accept			json
//	@Produce		json
//	@Param			id	path		string	true	"ID group"
//	@Param Authorization header string true "Insert your access token" default(bearer <Add access token here>)
//
//	@Success		204	{object}	string
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		404	{object}	errors.APIError	"Groupe non trouvé - Impossible de le supprimer "
//
//	@Router			/groups/{id} [delete]
func (g groupController) DeleteGroup(ctx *gin.Context) {
	g.DeleteEntity(ctx)
}
