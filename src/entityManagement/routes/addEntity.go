package controller

import (
	"net/http"

	authErrors "soli/formations/src/auth/errors"
	"soli/formations/src/entityManagement/utils"

	"github.com/gin-gonic/gin"
)

func (genericController genericController) AddEntity(ctx *gin.Context) {
	entityName := GetEntityNameFromPath(ctx.FullPath())

	decodedData, decodeError := genericController.genericService.DecodeInputDtoForEntityCreation(entityName, ctx)
	if decodeError != nil {
		HandleEntityError(ctx, decodeError)
		return
	}

	userId := ctx.GetString("userId")
	entity, entityCreationError := genericController.genericService.CreateEntityWithUser(decodedData, entityName, userId)
	if entityCreationError != nil {
		HandleEntityError(ctx, entityCreationError)
		return
	}

	entity, entitySavingError := genericController.addOwnerIDs(entity, userId)
	if entitySavingError != nil {
		HandleEntityError(ctx, entitySavingError)
		return
	}

	outputDto, errEntityDto := genericController.genericService.GetEntityFromResult(entityName, entity)
	if errEntityDto {
		// Legacy error handling for backward compatibility
		if authErrors.HandleError(http.StatusNotFound, &authErrors.APIError{ErrorMessage: "Entity Not Found"}, ctx) {
			return
		}
	}

	resourceName := GetResourceNameFromPath(ctx.FullPath())
	errorSettingDefaultAccesses := genericController.genericService.AddDefaultAccessesForEntity(resourceName, entity, userId)
	if errorSettingDefaultAccesses != nil {
		HandleEntityError(ctx, errorSettingDefaultAccesses)
		return
	}

	ctx.JSON(http.StatusCreated, outputDto)
}

func (genericController genericController) addOwnerIDs(entity any, userId string) (any, error) {
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
