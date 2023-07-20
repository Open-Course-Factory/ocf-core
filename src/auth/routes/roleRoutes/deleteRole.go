package roleController

import (
	"github.com/gin-gonic/gin"
)

// Delete role godoc
//
//	@Summary		Suppression role
//	@Description	Suppression d'un role dans la base de données
//	@Tags			roles
//	@Accept			json
//	@Produce		json
//	@Param			id	path		string	true	"ID role"
//	@Param Authorization header string true "Insert your access token" default(bearer <Add access token here>)
//
//	@Success		204	{object}	string
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		404	{object}	errors.APIError	"Role non trouvé - Impossible de le supprimer "
//
//	@Router			/roles/{id} [delete]
func (roleController roleController) DeleteRole(ctx *gin.Context) {
	roleController.GenericController.DeleteEntity(ctx)
}
