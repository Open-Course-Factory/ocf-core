package sshKeyController

import (
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
	s.GetEntities(ctx)
}
