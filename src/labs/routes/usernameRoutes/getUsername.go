package usernameController

import (
	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}

// GetUsername godoc
//
//	@Summary		Récupération des username
//	@Description	Récupération de toutes les username disponibles
//	@Tags			usernames
//	@Accept			json
//	@Produce		json
//	@Param			id	path	string	true	"ID username"
//
//	@Security		Bearer
//
//	@Success		200	{object}	[]dto.UsernameOutput
//
//	@Failure		404	{object}	errors.APIError	"Cours inexistants"
//
//	@Router			/usernames/{id} [get]
func (c usernameController) GetUsername(ctx *gin.Context) {
	c.GetEntity(ctx)
}
