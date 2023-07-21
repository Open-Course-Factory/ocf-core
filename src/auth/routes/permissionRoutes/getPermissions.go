package permissionController

import (
	"github.com/gin-gonic/gin"
)

// Get all permissions godoc
//
//	@Summary		Récupération permissions
//	@Description	Récupération de tous les permissions dans la base données
//	@Tags			permissions
//	@Accept			json
//	@Produce		json
//
//	@Param Authorization header string true "Insert your access token" default(bearer <Add access token here>)
//
//	@Success		200	{object}	[]dto.PermissionOutput
//
//	@Failure		404	{object}	errors.APIError	"Permissions inexistants"
//
//	@Router			/permissions [get]
func (p permissionController) GetPermissions(ctx *gin.Context) {
	p.GetEntities(ctx)
}
