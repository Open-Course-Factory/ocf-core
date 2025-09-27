package userController

import (
	"net/http"

	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

// Get User godoc
//
//	@Summary		Récupérer un utilisateur
//	@Description	Récupère un utilisateur par son ID
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Param			id	path		string	true	"User ID"
//	@Security		Bearer
//	@Success		200	{object}	dto.UserOutput
//	@Failure		400	{object}	errors.APIError	"Bad request"
//	@Failure		404	{object}	errors.APIError	"User not found"
//	@Router			/users/{id} [get]
func (u userController) GetUser(ctx *gin.Context) {
	userID := ctx.Param("id")
	if userID == "" {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "User ID is required",
		})
		return
	}

	user, userError := u.service.GetUserById(userID)
	if userError != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: userError.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, user)
}