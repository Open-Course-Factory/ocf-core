package authController

import (
	"fmt"
	"net/http"
	"soli/formations/src/auth/errors"
	"soli/formations/src/auth/models"

	gormadapter "github.com/casbin/gorm-adapter/v3"
	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type AuthMiddleware interface {
	AuthManagement() gin.HandlerFunc
}

type authMiddleware struct {
	adapter *gormadapter.Adapter
}

func NewAuthMiddleware(db *gorm.DB) AuthMiddleware {
	mAdapter, err := gormadapter.NewAdapterByDB(db)
	if err != nil {
		panic(fmt.Sprintf("failed to initialize casbin adapter: %v", err))
	}

	return &authMiddleware{adapter: mAdapter}
}

func (am *authMiddleware) AuthManagement() gin.HandlerFunc {
	return func(ctx *gin.Context) {
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
