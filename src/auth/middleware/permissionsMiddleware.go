package middleware

import (
	"net/http"
	"soli/formations/src/auth/models"
	controller "soli/formations/src/auth/routes"
	"soli/formations/src/auth/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type PermissionsMiddleware struct {
	DB *gorm.DB
}

func (opm PermissionsMiddleware) IsAuthorized() gin.HandlerFunc {

	return func(ctx *gin.Context) {

		userRoleObjectAssociationsArray, isRolesArray, roleFound := controller.GetRolesFromContext(ctx)
		if !roleFound {
			ctx.JSON(http.StatusForbidden, "You do not have permission to access this resource")
			ctx.Abort()
			return
		}

		// permissionService := services.NewPermissionService(opm.DB)
		// isUserInstanceAdmin := permissionService.IsUserInstanceAdmin(permissionsArray)
		// if isUserInstanceAdmin {
		// 	ctx.Next()
		// }

		if isRolesArray {
			switch ctx.Request.Method {
			case http.MethodPost:
				ctx.Next()
			case http.MethodGet:
				_, idFound := controller.GetEntityIdFromContext(ctx)
				if idFound {
					opm.callAboutSpecificEntityWithId(ctx, userRoleObjectAssociationsArray)
				} else {
					ctx.Next()
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

func (opm PermissionsMiddleware) callAboutSpecificEntityWithId(ctx *gin.Context, rolesArray *[]models.UserRole) bool {
	entityUUID, idFound := controller.GetEntityIdFromContext(ctx)
	if !idFound {
		return false
	}
	genericService := services.NewGenericService(opm.DB)

	entityName := controller.GetEntityNameFromPath(ctx.FullPath())
	entityModelInterface := genericService.GetEntityModelInterface(entityName)

	entity, entityError := genericService.GetEntity(entityUUID, entityModelInterface)
	if entityError != nil {
		ctx.JSON(http.StatusForbidden, "You do not have permission to access this resource")
		ctx.Abort()
		return false
	}

	isUserInstanceAdmin := genericService.IsUserInstanceAdmin(rolesArray)
	organisation := genericService.GetObjectOrganisation(entityName, entity)
	isUserOrganisationAdmin := genericService.IsUserOrganisationAdmin(rolesArray, organisation)

	var proceed bool
	if !isUserInstanceAdmin && !isUserOrganisationAdmin {
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
