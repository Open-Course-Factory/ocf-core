package sshKeyController

import (
	"net/http"
	"soli/formations/src/courses/errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Delete sshKey godoc
//
//	@Summary		Suppression sshKey
//	@Description	Suppression d'une sshKey dans la base de données
//	@Tags			sshKeys
//	@Accept			json
//	@Produce		json
//	@Param			id	path		string	true	"ID sshKey"
//	@Param Authorization header string true "Insert your access token" default(bearer <Add access token here>)
//
//	@Success		204	{object}	string
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		404	{object}	errors.APIError	"SshKey non trouvée - Impossible de le supprimer "
//
//	@Router			/sshkeys/{id} [delete]
func (s sshKeyController) DeleteSshKey(ctx *gin.Context) {
	idParam := ctx.Param("id")

	id, parseError := uuid.Parse(idParam)
	if parseError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: parseError.Error(),
		})
		return
	}

	errorDelete := s.service.DeleteKey(id.String())
	if errorDelete != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "SshKey not found",
		})
		return
	}
	ctx.JSON(http.StatusNoContent, "Done")
}
