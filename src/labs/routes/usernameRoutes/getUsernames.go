package usernameController

import (
	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}

// GetUsernames godoc
//
//	@Summary		Récupération des username
//	@Description	Récupération de tous les usernames disponibles
//	@Tags			usernames
//	@Accept			json
//	@Produce		json
//
//	@Security		Bearer
//
//	@Success		200	{object}	[]dto.UsernameOutput
//
//	@Failure		404	{object}	errors.APIError	"Username inexistant"
//
//	@Router			/usernames [get]
func (c usernameController) GetUsernames(ctx *gin.Context) {
	c.GetEntities(ctx)
}
