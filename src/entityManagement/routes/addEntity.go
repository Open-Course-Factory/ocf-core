package controller

import (
	"fmt"
	"net/http"
	"reflect"

	"soli/formations/src/auth/casdoor"
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

	errDecode := decoder.Decode(entityCreateDtoInput)
	if errors.HandleError(http.StatusInternalServerError, errDecode, ctx) {
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

	errPolicyLoading := casdoor.Enforcer.LoadPolicy()
	if errors.HandleError(http.StatusInternalServerError, errPolicyLoading, ctx) {
		return
	}

	resourceName := GetResourceNameFromPath(ctx.FullPath())
	entityUuid := genericController.genericService.ExtractUuidFromReflectEntity(entity)

	_, errAddingPolicy := casdoor.Enforcer.AddPolicy(userId, "/api/v1/"+resourceName+"/"+entityUuid.String(), "(GET|DELETE|PATCH|PUT)")
	if errors.HandleError(http.StatusInternalServerError, errAddingPolicy, ctx) {
		return
	}

	ctx.JSON(http.StatusCreated, outputDto)
}

func (genericController genericController) addOwnerIDs(entity interface{}, userId string) (interface{}, error) {
	entityReflectValue := reflect.ValueOf(entity).Elem()
	ownerIdsField := entityReflectValue.FieldByName("OwnerIDs")
	if ownerIdsField.IsValid() {

		if ownerIdsField.CanSet() {

			fmt.Println(ownerIdsField.Kind())
			if ownerIdsField.Kind() == reflect.Slice {
				ownerIdsField.Set(reflect.MakeSlice(ownerIdsField.Type(), 1, 1))
				ownerIdsField.Index(0).Set(reflect.ValueOf(userId))

				entityWithOwnerIds, entitySavingError := genericController.genericService.SaveEntity(entity)

				if entitySavingError != nil {
					return nil, entitySavingError
				}

				entity = entityWithOwnerIds
			}
		}

	}
	return entity, nil
}
