package sshKeyController

import (
	"net/http"
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/errors"
	"soli/formations/src/auth/models"
	sqldb "soli/formations/src/db"
	ems "soli/formations/src/entityManagement/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

var _ = errors.APIError{}

// Patch sshKey godoc
//
//	@Summary		modification sshKey name
//	@Description	Modification du nom d'une sshKey dans la base de données
//	@Tags			sshKeys
//	@Accept			json
//	@Produce		json
//	@Param			id		path	string	true	"ID sshKey"
//	@Param			newName	body	string	true	"Nouveau nom de la clé SSH"
//
//	@Security		Bearer
//
//	@Success		201	{object}	string
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		404	{object}	errors.APIError	"SshKey non trouvée - Impossible de le modifier "
//
//	@Router			/sshkeys/{id} [patch]
func (s sshKeyController) PatchSshKeyName(ctx *gin.Context) {
	idParam := ctx.Param("id")

	data := make(map[string]dto.PatchSshkey)

	if err := ctx.BindJSON(&data); err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid JSON format",
		})
		return
	}

	id, parseError := uuid.Parse(idParam)
	if parseError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: parseError.Error(),
		})
		return
	}

	genServ := ems.NewGenericService(sqldb.DB)
	errorUpdate := genServ.PatchEntity(id, &models.Sshkey{}, data["data"])
	if errorUpdate != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "SshKey not found",
		})
		return
	}
	ctx.JSON(http.StatusNoContent, "Done")
}
