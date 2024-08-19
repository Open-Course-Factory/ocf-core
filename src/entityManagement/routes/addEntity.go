package controller

import (
	"fmt"
	"net/http"
	"reflect"

	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/errors"

	ems "soli/formations/src/entityManagement/entityManagementService"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

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

	userId := ctx.GetString("userId")
	entity, entitySavingError := genericController.addOwnerIDs(entity, userId)
	if entitySavingError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: entitySavingError.Error(),
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

	errPolicyLoading := casdoor.Enforcer.LoadPolicy()
	if errPolicyLoading != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Policy not Found",
		})
		return
	}

	resourceName := GetResourceNameFromPath(ctx.FullPath())
	entityUuid := extractUuidFromReflectEntity(entity)

	_, errAddingPolicy := casdoor.Enforcer.AddPolicy(userId, "/api/v1/"+resourceName+"/"+entityUuid.String(), "(GET|DELETE|PATCH|PUT)")
	if errAddingPolicy != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Policy not added",
		})
		return
	}

	ctx.JSON(http.StatusCreated, outputDto)
}

func extractUuidFromReflectEntity(entity interface{}) uuid.UUID {
	entityReflectValue := reflect.ValueOf(entity).Elem()
	field := entityReflectValue.FieldByName("ID")
	mon_uuid := uuid.UUID(field.Bytes())
	return mon_uuid
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
