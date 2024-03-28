package sshKeyController

import (
	"net/http"
	"soli/formations/src/auth/dto"
	"soli/formations/src/courses/errors"

	"github.com/gin-gonic/gin"
)

// GetSshKeys godoc
// @Summary Récupération des sshKeys
// @Description Récupération de toutes les sshKeys disponibles
// @Tags sshKeys
// @Accept json
// @Produce json
//
// @Security Bearer
//
// @Success 200 {object} []dto.SshKeyOutput
//
// @Failure 404 {object} errors.APIError "SshKeys inexistantes"
//
// @Router /sshkeys [get]
func (s sshKeyController) GetSshKeys(ctx *gin.Context) {

	var sshKeys *[]dto.SshKeyOutput
	sshKeys, err := s.service.GetAllKeys()

	if err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "SshKeys not found",
		})
		return
	}

	ctx.JSON(http.StatusOK, sshKeys)
}
