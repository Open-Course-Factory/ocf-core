package courseController

import (
	"github.com/gin-gonic/gin"
)

// Delete course godoc
//
// @Summary		Suppression cours
// @Description	Suppression d'un cours dans la base de données
// @Tags			courses
// @Accept			json
// @Produce		json
// @Param			id	path		string	true	"ID cours"
//
// @Security Bearer
//
// @Success		204	{object}	string
//
// @Failure		400	{object}	errors.APIError	"Impossible de parser le json"
// @Failure		404	{object}	errors.APIError	"Cours non trouvé - Impossible de le supprimer "
//
// @Router			/courses/{id} [delete]
func (c courseController) DeleteCourse(ctx *gin.Context) {
	c.DeleteEntity(ctx)
}
