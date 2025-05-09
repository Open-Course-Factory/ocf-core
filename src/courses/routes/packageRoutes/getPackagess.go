package packageController

import (
	errors "soli/formations/src/auth/errors"
	"soli/formations/src/courses/dto"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}
var _ = dto.PackageOutput{}

// GetPackages godoc
//
//	@Summary		Récupération des packages
//	@Description	Récupération de toutes les packages disponibles
//	@Tags			packages
//	@Accept			json
//	@Produce		json
//
//	@Security		Bearer
//
//	@Success		200	{object}	[]dto.PackageOutput
//
//	@Failure		404	{object}	errors.APIError	"Packages inexistantes"
//
//	@Router			/packages [get]
func (s packageController) GetPackages(ctx *gin.Context) {

	s.GetEntities(ctx)
}
