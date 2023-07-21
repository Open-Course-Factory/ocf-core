package permissionController

import (
	"github.com/gin-gonic/gin"
)

// Delete permission godoc
//
//	@Summary		Suppression permission
//	@Description	Suppression d'un permission dans la base de données
//	@Tags			permissions
//	@Accept			json
//	@Produce		json
//	@Param			id	path		string	true	"ID permission"
//	@Param Authorization header string true "Insert your access token" default(bearer <Add access token here>)
//
//	@Success		204	{object}	string
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		404	{object}	errors.APIError	"Permission non trouvée - Impossible de la supprimer "
//
//	@Router			/permissions/{id} [delete]
func (p permissionController) DeletePermission(ctx *gin.Context) {
	p.DeleteEntity(ctx)
}
