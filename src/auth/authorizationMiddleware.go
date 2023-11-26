package auth

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

		fmt.Println(res)
		fmt.Println(err)

		ctx.Next()
	}
}
