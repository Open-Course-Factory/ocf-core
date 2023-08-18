package controller

import (
	"net/http"
	"reflect"
	"soli/formations/src/auth/errors"
	"soli/formations/src/auth/models"

	entityManagementModels "soli/formations/src/entityManagement/models"

	"github.com/gin-gonic/gin"
)

func (genericController genericController) GetEntitiesWithPermissionCheck(ctx *gin.Context) {

	entitiesDto, shouldReturn1 := genericController.getEntities(ctx, true)
	if shouldReturn1 {
		return
	}

	ctx.JSON(http.StatusOK, entitiesDto)
}

func (genericController genericController) GetEntitiesWithoutPermissionCheck(ctx *gin.Context) {

	entitiesDto, shouldReturn1 := genericController.getEntities(ctx, false)
	if shouldReturn1 {
		return
	}

	ctx.JSON(http.StatusOK, entitiesDto)
}

func (genericController genericController) getEntities(ctx *gin.Context, permCheck bool) ([]interface{}, bool) {
	entityName := GetEntityNameFromPath(ctx.FullPath())

	userRolesArray, _, userRoleFound := GetRolesFromContext(ctx)
	if !userRoleFound {
		return nil, true
	}

	entitiesDto, shouldReturn := genericController.getEntitiesFromName(entityName, userRolesArray, permCheck)
	if shouldReturn {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Entities not found",
		})
		return nil, true
	}
	return entitiesDto, false
}

func (genericController genericController) getEntitiesFromName(entityName string, userRoles *[]models.UserRole, permCheck bool) ([]interface{}, bool) {
	entityModelInterface := genericController.genericService.GetEntityModelInterface(entityName)
	allEntitiesPages, err := genericController.genericService.GetEntities(entityModelInterface)
	isUserInstanceAdmin := genericController.genericService.IsUserInstanceAdmin(userRoles)

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

			for i := 0; i < convertedPage.Len(); i++ {

				item := convertedPage.Index(i).Interface()

				entityBaseModel, isOk := entityManagementModels.ExtractBaseModelFromAny(item)
				if !isOk {
					continue
				}

				if permCheck {
					organisation := genericController.genericService.GetObjectOrganisation(entityName, item)
					isUserOrganisationAdmin := genericController.genericService.IsUserOrganisationAdmin(userRoles, organisation)

					if !isUserInstanceAdmin && !isUserOrganisationAdmin {
						// Here we check permissions for the logged in user through userRoles added in context,
						// maybe should be done within the request (to avoid this)
						proceed := HasUserRolesPermissionForEntity(userRoles, http.MethodGet, entityName, entityBaseModel.ID)
						if !proceed {
							continue
						}
					}
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
