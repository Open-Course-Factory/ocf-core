package controller

import (
	"net/http"

	"soli/formations/src/auth/errors"

	ems "soli/formations/src/entityManagement/entityManagementService"

	"github.com/gin-gonic/gin"

	"github.com/mitchellh/mapstructure"
)

func (genericController genericController) AddEntity(ctx *gin.Context) {
	entityName := GetEntityNameFromPath(ctx.FullPath())

	entityCreateDtoInput := ems.GlobalEntityRegistrationService.GetEntityDtos(entityName, ems.InputDto)
	decodedData := ems.GlobalEntityRegistrationService.GetEntityDtos(entityName, ems.InputDto)

	bindError := ctx.BindJSON(&entityCreateDtoInput)
	if bindError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Impossible de parser le json",
		})
		return
	}

	errDecode := mapstructure.Decode(entityCreateDtoInput, &decodedData)
	if errDecode != nil {
		panic(errDecode)
	}

	entity, entityCreationError := genericController.genericService.CreateEntity(decodedData, entityName)
	if entityCreationError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: entityCreationError.Error(),
		})
		return
	}

	var errEntityDto bool

	outputDto, errEntityDto := genericController.getEntityFromResult(entityName, entity)

	if errEntityDto {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Not Found",
		})
		return
	}

	ctx.JSON(http.StatusCreated, outputDto)
}
