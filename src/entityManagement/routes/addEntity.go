package controller

import (
	"net/http"

	"soli/formations/src/auth/errors"
	"soli/formations/src/entityManagement/utils"

	"github.com/gin-gonic/gin"
)

func (genericController genericController) AddEntity(ctx *gin.Context) {
	entityName := GetEntityNameFromPath(ctx.FullPath())

	decodedData, decodeError := genericController.genericService.DecodeInputDtoForEntityCreation(entityName, ctx)
	if errors.HandleError(http.StatusBadRequest, decodeError, ctx) {
		return
	}

	entity, entityCreationError := genericController.genericService.CreateEntity(decodedData, entityName)
	if errors.HandleError(http.StatusBadRequest, entityCreationError, ctx) {
		return
	}

	userId := ctx.GetString("userId")
	entity, entitySavingError := genericController.addOwnerIDs(entity, userId)
	if errors.HandleError(http.StatusBadRequest, entitySavingError, ctx) {
		return
	}

	outputDto, errEntityDto := genericController.genericService.GetEntityFromResult(entityName, entity)

	if errEntityDto {
		if errors.HandleError(http.StatusNotFound, &errors.APIError{ErrorMessage: "Entity Not Found"}, ctx) {
			return
		}
	}

	resourceName := GetResourceNameFromPath(ctx.FullPath())
	errorSettingDefaultAccesses := genericController.genericService.AddDefaultAccessesForEntity(resourceName, entity, userId)

	if errorSettingDefaultAccesses != nil {
		if errors.HandleError(http.StatusNotFound, errorSettingDefaultAccesses, ctx) {
			return
		}
	}

	ctx.JSON(http.StatusCreated, outputDto)
}

func (genericController genericController) addOwnerIDs(entity interface{}, userId string) (interface{}, error) {
	// Add owner ID to entity (modifies in-place)
	if err := utils.AddOwnerIDToEntity(entity, userId); err != nil {
		return nil, err
	}

	// Save entity with updated OwnerIDs
	entityWithOwnerIds, entitySavingError := genericController.genericService.SaveEntity(entity)
	if entitySavingError != nil {
		return nil, entitySavingError
	}

	return entityWithOwnerIds, nil
}
