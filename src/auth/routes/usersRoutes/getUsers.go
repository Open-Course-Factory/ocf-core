package userController

import (
	"net/http"

	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

// Get Users godoc
//
//	@Summary		Récupérer tous les utilisateurs
//	@Description	Récupère la liste de tous les utilisateurs
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Security		Bearer
//	@Success		200	{array}		dto.UserOutput
//	@Failure		500	{object}	errors.APIError	"Internal server error"
//	@Router			/users [get]
func (u userController) GetUsers(ctx *gin.Context) {
	users, userError := u.service.GetAllUsers()
	if userError != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: userError.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, users)
}
