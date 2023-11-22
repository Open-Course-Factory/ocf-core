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
		casdoorsdk.InitConfig("http://casdoor:8000", "82b29aa895790d9fd23e", "81cdbfa7e7b02e0fd92ecb78cdc282e9fa25752c", "", "sdv", "ocf")

		fmt.Println(casdoorsdk.GetUsers())

		ctx.Next()
	}
}
