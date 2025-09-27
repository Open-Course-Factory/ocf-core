package userController

import (
	"net/http"

	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

// Search Users godoc
//
//	@Summary		Rechercher des utilisateurs
//	@Description	Recherche des utilisateurs par nom ou email
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Param			q	query		string	true	"Search query (minimum 2 characters)"
//	@Security		Bearer
//	@Success		200	{array}		dto.UserOutput
//	@Failure		400	{object}	errors.APIError	"Bad request"
//	@Failure		500	{object}	errors.APIError	"Internal server error"
//	@Router			/users/search [get]
func (u userController) SearchUsers(ctx *gin.Context) {
	query := ctx.Query("q")
	if query == "" {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Search query is required",
		})
		return
	}

	if len(query) < 2 {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Search query must be at least 2 characters",
		})
		return
	}

	users, userError := u.service.SearchUsers(query)
	if userError != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: userError.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, users)
}