package controller

import (
	"net/http"

	"soli/formations/src/auth/errors"
	ems "soli/formations/src/entityManagement/entityManagementService"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/mitchellh/mapstructure"
)

func (genericController genericController) EditEntity(ctx *gin.Context) {
	entityName := GetEntityNameFromPath(ctx.FullPath())

	entityPatchDtoInput := genericController.entityRegistrationService.GetEntityDtos(entityName, ems.InputEditDto)
	decodedData := ems.GlobalEntityRegistrationService.GetEntityDtos(entityName, ems.InputEditDto)

	bindError := ctx.BindJSON(&entityPatchDtoInput)
	if errors.HandleError(http.StatusBadRequest, bindError, ctx) {
		return
	}

	config := &mapstructure.DecoderConfig{
		WeaklyTypedInput: true,
		Result:           &decodedData,
	}

	decoder, err := mapstructure.NewDecoder(config)
	if err != nil {
		panic(err)
	}

	errDecode := decoder.Decode(entityPatchDtoInput)
	if errors.HandleError(http.StatusInternalServerError, errDecode, ctx) {
		return
	}

	id, parseErr := uuid.Parse(ctx.Param("id"))
	if errors.HandleError(http.StatusBadRequest, parseErr, ctx) {
		return
	}

	entityModelInterface := genericController.genericService.GetEntityModelInterface(entityName)

	errorDelete := genericController.genericService.EditEntity(id, entityName, entityModelInterface, decodedData)
	if errors.HandleError(http.StatusNotFound, errorDelete, ctx) {
		return
	}

	ctx.JSON(http.StatusNoContent, "Done")
}
