package middleware

import (
	"net/http"
	"strings"

	"soli/formations/src/auth/models"
	"soli/formations/src/auth/services"
	config "soli/formations/src/configuration"

	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type AuthMiddleware struct {
	DB     *gorm.DB
	Config *config.Configuration
}

func (am AuthMiddleware) CheckIsLogged() gin.HandlerFunc {
	return func(ctx *gin.Context) {

		splitAuthorization := strings.Split(ctx.GetHeader("Authorization"), " ")

		if len(splitAuthorization) != 2 {
			ctx.JSON(http.StatusUnauthorized, errors.APIError{ErrorCode: http.StatusUnauthorized, ErrorMessage: "Token header is invalid"})
			ctx.Abort()
			return
		}

		if strings.Compare(splitAuthorization[0], "bearer") != 0 {
			ctx.JSON(http.StatusUnauthorized, errors.APIError{ErrorCode: http.StatusUnauthorized, ErrorMessage: "bearer key is not found"})
			ctx.Abort()
			return
		}

		token := splitAuthorization[1]
		jwtService := &services.JwtService{}
		genericService := services.NewGenericService(am.DB)
		id, err := jwtService.ParseJWT(token, am.Config.SecretJwt)

		o := errors.APIError{ErrorCode: http.StatusUnauthorized, ErrorMessage: "Token is not valid"}
		if err != nil {
			ctx.JSON(http.StatusUnauthorized, o)
			ctx.Abort()
			return
		}

		user, userError := genericService.GetEntity(*id, models.User{})

		if userError != nil {
			ctx.JSON(http.StatusUnauthorized, o)
			ctx.Abort()
			return
		}

		permissionService := services.NewPermissionService(am.DB)
		permissions, permissionError := permissionService.GetPermissionsByUser(*id)

		if permissionError != nil {
			ctx.JSON(http.StatusUnauthorized, o)
			ctx.Abort()
			return
		}

		ctx.Set("permissions", permissions)
		ctx.Set("user", user)
		ctx.Next()
	}
}
