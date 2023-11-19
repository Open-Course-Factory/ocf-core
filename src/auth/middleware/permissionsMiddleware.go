package middleware

import (
	"bytes"
	"io"
	"net/http"
	"reflect"
	"soli/formations/src/auth/models"
	controller "soli/formations/src/auth/routes"
	"soli/formations/src/auth/services"
	"strings"

	entityManagementModels "soli/formations/src/entityManagement/models"
	entityManagementServices "soli/formations/src/entityManagement/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type PermissionsMiddleware struct {
	DB             *gorm.DB
	genericService services.GenericService
}

func NewPermissionsMiddleware(DB *gorm.DB, genericService services.GenericService) *PermissionsMiddleware {
	return &PermissionsMiddleware{
		DB:             DB,
		genericService: genericService,
	}
}

func (opm PermissionsMiddleware) IsAuthorized() gin.HandlerFunc {

	return func(ctx *gin.Context) {

		userRoleObjectAssociationsArray, isRolesArray, roleFound := controller.GetRolesFromContext(ctx)
		if !roleFound {
			ctx.JSON(http.StatusForbidden, "You do not have permission to access this resource")
			ctx.Abort()
			return
		}

		if isRolesArray {

			isUserInstanceAdmin := opm.genericService.IsUserInstanceAdmin(userRoleObjectAssociationsArray)

			if isUserInstanceAdmin {
				ctx.Next()
				return
			}

			switch ctx.Request.Method {
			case http.MethodPost:
				entityName := controller.GetEntityNameFromPath(ctx.FullPath())
				resType, entityTypeOk := entityManagementServices.GlobalEntityRegistrationService.GetEntityType(entityName)
				if entityTypeOk {
					instance := reflect.New(resType).Elem()
					baseModelInstance, isInterfaceWithBaseModel := instance.Interface().(entityManagementModels.InterfaceWithBaseModel)
					if isInterfaceWithBaseModel {
						refObject := baseModelInstance.GetReferenceObject()
						var json map[string]interface{}
						ByteBody, _ := io.ReadAll(ctx.Request.Body)
						ctx.Request.Body = io.NopCloser(bytes.NewBuffer(ByteBody))
						if err := ctx.ShouldBindJSON(&json); err != nil {
							ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
							return
						}
						ctx.Request.Body = io.NopCloser(bytes.NewBuffer(ByteBody))
						resObjectId, found := json[strings.ToLower(refObject)]
						if found {
							resObjectUuid, uuidError := uuid.Parse(resObjectId.(string))
							if uuidError != nil {
								ctx.JSON(http.StatusBadRequest, gin.H{"error": uuidError.Error()})
								return
							}

							proceed := controller.HasUserRolesPermissionForEntity(userRoleObjectAssociationsArray, ctx.Request.Method, refObject, resObjectUuid)
							if proceed {
								ctx.Next()
								return
							}

						}
						if refObject == entityName {
							ctx.Next()
							return
						}

					}
				}
				ctx.JSON(http.StatusForbidden, "You do not have permission to create this resource")
				ctx.Abort()
				return
			case http.MethodGet:
				_, idFound := controller.GetEntityIdFromContext(ctx)
				if idFound {
					opm.callAboutSpecificEntityWithId(ctx, userRoleObjectAssociationsArray)
				} else {
					ctx.Next()
					return
				}
			case http.MethodPut:
				roleFound = opm.callAboutSpecificEntityWithId(ctx, userRoleObjectAssociationsArray)
			case http.MethodPatch:
				roleFound = opm.callAboutSpecificEntityWithId(ctx, userRoleObjectAssociationsArray)
			case http.MethodDelete:
				roleFound = opm.callAboutSpecificEntityWithId(ctx, userRoleObjectAssociationsArray)
			default:
				ctx.JSON(http.StatusForbidden, "Unknown HTTP Method fot this endpoint")
				ctx.Abort()
				return
			}

			if !roleFound {
				ctx.JSON(http.StatusForbidden, "You do not have permission to access this resource")
				ctx.Abort()
				return
			}

		} else {
			ctx.JSON(http.StatusForbidden, "You do not have permission to access this resource")
			ctx.Abort()
			return
		}

	}

}

func (opm PermissionsMiddleware) callAboutSpecificEntityWithId(ctx *gin.Context, rolesArray *[]models.UserRoles) bool {
	entityUUID, idFound := controller.GetEntityIdFromContext(ctx)
	if !idFound {
		return false
	}

	entityName := controller.GetEntityNameFromPath(ctx.FullPath())
	resEntity, found := opm.GetEntityFromTypeAndId(entityName, entityUUID)
	if !found {
		ctx.JSON(http.StatusForbidden, "You do not have permission to access this resource")
		ctx.Abort()
		return false
	}

	organisation := opm.genericService.GetObjectOrganisation(entityName, resEntity)
	isUserOrganisationAdmin := opm.genericService.IsUserOrganisationAdmin(rolesArray, organisation)

	var proceed bool
	if !isUserOrganisationAdmin {
		proceed = controller.HasUserRolesPermissionForEntity(rolesArray, ctx.Request.Method, entityName, entityUUID)
	} else {
		proceed = true
	}

	if !proceed {
		ctx.JSON(http.StatusForbidden, "You do not have permission to access this resource")
		ctx.Abort()
		return false
	} else {
		ctx.Next()
	}
	return true
}

func (opm PermissionsMiddleware) GetEntityFromTypeAndId(entityName string, entityUUID uuid.UUID) (interface{}, bool) {
	entityModelInterface := opm.genericService.GetEntityModelInterface(entityName)

	entity, entityError := opm.genericService.GetEntity(entityUUID, entityModelInterface)
	if entityError != nil {
		return nil, false
	}
	return entity, true
}
