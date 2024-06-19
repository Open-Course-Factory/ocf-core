package authController

import (
	"fmt"
	"log"
	"net/http"
	"soli/formations/src/auth/errors"
	"soli/formations/src/auth/models"
	"strings"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/gertd/go-pluralize"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type AuthMiddleware interface {
	AuthManagement() gin.HandlerFunc
}

type authMiddleware struct {
}

func NewAuthMiddleware(db *gorm.DB) AuthMiddleware {
	return &authMiddleware{}
}

func (am *authMiddleware) AuthManagement() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		userId, err := getUserIdFromToken(ctx)

		if err != nil {
			ctx.AbortWithStatusJSON(http.StatusNotFound, gin.H{"msg": err.Error()})
		}

		obj := GetEntityNameFromPath(ctx.FullPath())

		// Casbin enforces policy, subject = user currently logged in, obj = ressource URI obtained from request path, action (http verb))
		ok, errEnforce := casdoor.Enforcer.Enforce(fmt.Sprint(userId), obj, ctx.Request.Method)

		if errEnforce != nil {
			ctx.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"msg": "Error occurred when authorizing user"})
			return
		}

		if !ok {
			ctx.AbortWithStatusJSON(http.StatusForbidden, gin.H{"msg": "You are not authorized"})
			return
		}

		userName, err := getUserNameFromToken(ctx)

		if err != nil {
			ctx.JSON(http.StatusBadRequest, &errors.APIError{
				ErrorCode:    http.StatusBadRequest,
				ErrorMessage: err.Error(),
			})
			ctx.Abort()
			return
		}

		var userRoles []*casdoorsdk.Role
		roles, err := casdoorsdk.GetRoles()

		if err != nil {
			ctx.JSON(http.StatusInternalServerError, &errors.APIError{
				ErrorCode:    http.StatusInternalServerError,
				ErrorMessage: err.Error(),
			})
			ctx.Abort()
			return
		}

		for _, role := range roles {
			for _, user := range role.Users {
				fmt.Println(user)
				if user == userName {
					userRoles = append(userRoles, role)
				}
			}
		}

		// ToDo: refactoring
		var isAdmin bool
		var ocfRoles []*models.Role
		for _, role := range userRoles {
			ocfRole, err := models.FromString(role.Name)
			if err != nil {
				ctx.Abort()
			}

			adminString, _ := models.FromString(models.Admin.String())
			if ocfRole.String() == adminString.String() {
				isAdmin = true
				break
			}

			ocfRoles = append(ocfRoles, ocfRole)
		}

		if isAdmin {
			ctx.Next()
		} else {
			// ToDo: get permissions for each role
			// check whether there is a permission about the ressource requested
			// depending on type of request, get the allowed ressources list or the specific details about the ressource
			fmt.Println(ocfRoles)
			ctx.JSON(http.StatusUnauthorized, &errors.APIError{
				ErrorCode:    http.StatusUnauthorized,
				ErrorMessage: "Unauthorized",
			})
			ctx.Abort()
			return
		}

	}
}

func getUserNameFromToken(ctx *gin.Context) (string, error) {
	token := ctx.Request.Header.Get("Authorization")

	claims, err := casdoorsdk.ParseJwtToken(token)

	if err != nil {
		return "", err
	}

	userName := fmt.Sprintf("%s/%s", claims.Owner, claims.Name)
	return userName, nil
}

func getUserIdFromToken(ctx *gin.Context) (string, error) {
	token := ctx.Request.Header.Get("Authorization")

	claims, err := casdoorsdk.ParseJwtToken(token)

	if err != nil {
		return "", err
	}

	userId := claims.ID
	return userId, nil
}

func GetEntityNameFromPath(path string) string {

	// Trim any trailing slashes
	path = strings.TrimRight(path, "/")

	// Split the path into segments
	segments := strings.Split(path, "/")
	segment := ""

	// Take resource name segment
	if len(segments) > 3 {
		segment = segments[3]
	} else {
		segment = segments[1]
	}

	// Take resource name and resource type

	client := pluralize.NewClient()
	singular := client.Singular(segment)
	return strings.ToUpper(string(singular[0])) + singular[1:]
}

// func GetRolesFromContext(ctx *gin.Context) (*[]models.UserRoles, bool, bool) {
// 	rawRoles, ok := ctx.Get("roles")

// 	if !ok {
// 		ctx.JSON(http.StatusOK, "[]")
// 		ctx.Abort()
// 		return nil, false, false
// 	}

// 	userRoleObjectAssociationModels, isRole := rawRoles.(*[]models.UserRoles)
// 	return userRoleObjectAssociationModels, isRole, true
// }

func GetEntityIdFromContext(ctx *gin.Context) (uuid.UUID, bool) {
	entityID := ctx.Param("id")

	if entityID == "" {
		ctx.JSON(http.StatusBadRequest, "Entities Not Found")
		log.Default().Fatal("Permission Middleware has been called on a method without entity ID")
		ctx.Abort()
		return uuid.Nil, false
	}

	entityUUID, errUUID := uuid.Parse(entityID)

	if errUUID != nil {
		ctx.JSON(http.StatusNotFound, "Entity Not Found")
		ctx.Abort()
		return uuid.Nil, false
	}
	return entityUUID, true
}
