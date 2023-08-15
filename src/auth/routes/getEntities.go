package controller

import (
	"fmt"
	"net/http"
	"reflect"
	"soli/formations/src/auth/errors"
	"soli/formations/src/auth/models"

	"github.com/gin-gonic/gin"
)

func (genericController genericController) GetEntities(ctx *gin.Context) {

	entityName := GetEntityNameFromPath(ctx.FullPath())

	permissionsArray, _, permissionFound := GetPermissionsFromContext(ctx)
	if !permissionFound {
		return
	}

	entitiesDto, shouldReturn := genericController.getEntitiesFromName(entityName, permissionsArray)
	if shouldReturn {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Entities not found",
		})
		return
	}

	ctx.JSON(http.StatusOK, entitiesDto)
}

func (genericController genericController) getEntitiesFromName(entityName string, permissions *[]models.Permission) ([]interface{}, bool) {
	entityModelInterface := GetEntityModelInterface(entityName)

	allEntitiesPages, err := genericController.genericService.GetEntities(entityModelInterface)

	if err != nil {
		return nil, true
	}

	funcName := entityName + "ModelTo" + entityName + "Output"
	var entitiesDto []interface{}

	for _, page := range allEntitiesPages {

		entityModel := reflect.SliceOf(reflect.TypeOf(entityModelInterface))

		pageValue := reflect.ValueOf(page)

		if pageValue.Type().ConvertibleTo(entityModel) {
			convertedPage := pageValue.Convert(entityModel)
			fmt.Println(convertedPage)

			for i := 0; i < convertedPage.Len(); i++ {

				item := convertedPage.Index(i).Interface()

				// Here we check permissions for the logged in user, should be done within the request (to avoid this)
				entityBaseModel, isOk := models.ExtractBaseFromAny(item)
				var proceed bool
				if isOk {
					proceed = HasLoggedInUserPermissionForEntity(permissions, http.MethodGet, entityName, entityBaseModel.ID)
				}
				if !proceed {
					continue
				}

				var shouldReturn bool
				entitiesDto, shouldReturn = genericController.appendEntityFromResult(funcName, item, entitiesDto)
				if shouldReturn {
					return nil, true
				}
			}
		} else {
			return nil, true
		}

	}
	return entitiesDto, false
}
