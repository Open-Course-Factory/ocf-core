package packageController

import (
	"soli/formations/src/auth/errors"
	"soli/formations/src/courses/dto"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}
var _ = dto.PackageInput{}

// Add Package godoc
//
//	@Summary		Création package
//	@Description	Ajoute une nouvelle package dans la base de données
//	@Tags			packages
//	@Accept			json
//	@Produce		json
//	@Param			package	body	dto.PackageInput	true	"package"
//
//	@Security		Bearer
//
//	@Success		201	{object}	dto.PackageOutput
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		400	{object}	errors.APIError	"Impossible de créer une package"
//	@Failure		409	{object}	errors.APIError	"La package existe déjà"
//	@Router			/packages [post]
func (s packageController) AddPackage(ctx *gin.Context) {
	s.AddEntity(ctx)
}
