package userController

import (
	"net/http"
	"soli/formations/src/auth/casdoor"
	"soli/formations/src/courses/errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Delete user godoc
//
//	@Summary		Suppression user
//	@Description	Suppression d'un user dans la base de données
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Param			id	path	string	true	"ID user"
//
//	@Security		Bearer
//
//	@Success		204	{object}	string
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		404	{object}	errors.APIError	"User non trouvé - Impossible de le supprimer "
//
//	@Router			/users/{id} [delete]
func (u userController) DeleteUser(ctx *gin.Context) {
	idParam := ctx.Param("id")

	id, parseError := uuid.Parse(idParam)

	if parseError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: parseError.Error(),
		})
		ctx.Abort()
		return
	}

	errorDelete := u.service.DeleteUser(id.String())
	if errorDelete != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "User not found",
		})
		ctx.Abort()
		return
	}

	//ToDo : handle error
	casdoor.Enforcer.RemovePolicy(id.String())

	ctx.JSON(http.StatusNoContent, "Done")
}
