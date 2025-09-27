package userController

import (
	"net/http"

	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

// Get Users Batch godoc
//
//	@Summary		Récupérer des utilisateurs par IDs
//	@Description	Récupère plusieurs utilisateurs par leurs IDs
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Param			request	body		dto.BatchUsersInput	true	"User IDs"
//	@Security		Bearer
//	@Success		200	{array}		dto.UserOutput
//	@Failure		400	{object}	errors.APIError	"Bad request"
//	@Failure		500	{object}	errors.APIError	"Internal server error"
//	@Router			/users/batch [post]
func (u userController) GetUsersBatch(ctx *gin.Context) {
	var batchInput dto.BatchUsersInput

	bindError := ctx.ShouldBindJSON(&batchInput)
	if bindError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid request format",
		})
		return
	}

	if len(batchInput.UserIds) == 0 {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "User IDs are required",
		})
		return
	}

	// Limit to reasonable number of users
	if len(batchInput.UserIds) > 50 {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Too many user IDs (max 50)",
		})
		return
	}

	users, userError := u.service.GetUsersByIds(batchInput.UserIds)
	if userError != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: userError.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, users)
}