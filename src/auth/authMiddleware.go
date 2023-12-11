package authController

import (
	"fmt"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/gin-gonic/gin"
)

type AuthMiddleware struct {
}

func (am AuthMiddleware) AuthManagement() gin.HandlerFunc {
	return func(ctx *gin.Context) {

		res, err := casdoorsdk.GetUsers()

		// get user
		// get user roles
		// get permissions for each role
		// check whether there is a permission about the ressource requested
		// depending on type of request, get the allowed ressources list or the specific details about the ressource

		fmt.Println(res)
		fmt.Println(err)

		ctx.Next()
	}
}
