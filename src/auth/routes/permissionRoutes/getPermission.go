package permissionController

import (
	"github.com/gin-gonic/gin"
)

// Get permission godoc
//
//	@Summary		Récupération permission
//	@Description	Récupération des informations de la permission
//	@Tags			permissions
//	@Accept			json
//	@Produce		json
//	@Param		 	id	path		string	true	"ID permission"
//	@Param Authorization header string true "Insert your access token" default(bearer <Add access token here>)
//	@Success		200	{object}	dto.PermissionOutput
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		404	{object}	errors.APIError	"Permission inexistante - Impossible de la récupérer"
//
//	@Router			/permissions/{id} [get]
func (p permissionController) GetPermission(ctx *gin.Context) {
	p.GetEntity(ctx)
}
