package sshKeyController

import (
	"net/http"

	"soli/formations/src/auth/dto"
	"soli/formations/src/courses/errors"

	"github.com/gin-gonic/gin"
)

// Add SshKey godoc
//
// @Summary		Création sshKey
// @Description	Ajoute une nouvelle sshKey dans la base de données
// @Tags		sshKeys
// @Accept		json
// @Produce		json
// @Param		sshKey	body		dto.CreateSshKeyInput	true	"sshKey"
//
// @Security Bearer
//
// @Success		201		{object}	dto.CreateSshKeyOutput
//
// @Failure		400		{object}	errors.APIError	"Impossible de parser le json"
// @Failure		400		{object}	errors.APIError	"Impossible de créer une sshKey"
// @Failure		409		{object}	errors.APIError	"La sshKey existe déjà"
// @Router			/sshkeys [post]
func (s sshKeyController) AddSshKey(ctx *gin.Context) {
	sshKeyCreateDTO := dto.CreateSshKeyInput{}

	bindError := ctx.BindJSON(&sshKeyCreateDTO)
	if bindError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Impossible de parser le json",
		})
		return
	}

	sshKey, sshKeyError := s.service.AddUserSshKey(sshKeyCreateDTO)

	if sshKeyError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: sshKeyError.Error(),
		})
		return
	}

	ctx.JSON(http.StatusCreated, sshKey)
}
