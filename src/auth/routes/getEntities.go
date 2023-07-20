package controller

import (
	"fmt"
	"net/http"
	"reflect"
	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

func (genericController genericController) GetEntities(ctx *gin.Context) {

	entityName := genericController.extractSingularResource(ctx.FullPath())

	entitiesDto, shouldReturn := genericController.getEntitiesFromName(entityName)
	if shouldReturn {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Entities not found",
		})
		return
	}

	ctx.JSON(http.StatusOK, entitiesDto)
}

func (genericController genericController) getEntitiesFromName(entityName string) ([]interface{}, bool) {
	entityModelInterface := genericController.getEntityModelInterface(entityName)

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
