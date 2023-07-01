package groupController

import (
	"net/http"

	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Edit group godoc
//
//	@Summary		Modification groupe
//	@Description	Modification d'un groupe dans la base de données
//	@Tags			groups
//	@Accept			json
//	@Produce		json
//	@Param			id	path		int	true	"ID group"
//	@Param 			group 	body	dto.GroupEditInput	true	"Group"
//	@Param Authorization header string true "Insert your access token" default(bearer <Add access token here>)
//	@Success		204	{object}	string
//
//	@Failure		403	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		400	{object}	errors.APIError	"Impossible de récupérer l'ID groupe"
//	@Failure		400	{object}	errors.APIError	"Impossible de modifier le groupe"
//
//	@Router			/groups/{id} [put]
func (g groupController) EditGroup(ctx *gin.Context) {

	editGroup := &dto.GroupEditInput{}
	id, errParse := uuid.Parse(ctx.Param("id"))

	errBind := ctx.BindJSON(&editGroup)

	if errParse != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: errParse.Error(),
		})
		return
	}

	if errBind != nil {
		ctx.JSON(http.StatusForbidden,
			&errors.APIError{ErrorCode: http.StatusForbidden, ErrorMessage: errBind.Error()})
		return
	}

	edit, editGroupError := g.service.EditGroup(editGroup, id)

	if editGroupError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: editGroupError.Error()})
		return
	}
	ctx.JSON(http.StatusNoContent, edit)
}
