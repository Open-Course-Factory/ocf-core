package authController

import (
	"fmt"
	"net/http"
	"soli/formations/src/auth/errors"
	"soli/formations/src/auth/models"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/gin-gonic/gin"
)

type AuthMiddleware struct {
}

func (am AuthMiddleware) AuthManagement() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		token := ctx.Request.Header.Get("Authorization")

		fmt.Println(ctx.HandlerName())

		claims, err := casdoorsdk.ParseJwtToken(token)

		userName := fmt.Sprintf("%s/%s", claims.Owner, claims.Name)

		if err != nil {
			ctx.JSON(http.StatusInternalServerError, &errors.APIError{
				ErrorCode:    http.StatusInternalServerError,
				ErrorMessage: err.Error(),
			})
			ctx.Abort()
		}

		if claims.Name != "" {
			fmt.Println("Accessing as " + userName)
		} else {
			fmt.Println("User not found")
		}

		var userRoles []*casdoorsdk.Role

		roles, err := casdoorsdk.GetRoles()

		if err != nil {
			ctx.JSON(http.StatusInternalServerError, &errors.APIError{
				ErrorCode:    http.StatusInternalServerError,
				ErrorMessage: err.Error(),
			})
			ctx.Abort()
		}

		for _, role := range roles {
			for _, user := range role.Users {
				fmt.Println(user)
				if user == userName {
					userRoles = append(userRoles, role)
				}
			}
		}

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
		}

	}
}
