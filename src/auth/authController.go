package authController

import (
	"encoding/json"
	"net/http"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/gin-gonic/gin"
)

type AuthController interface {
	Login(ctx *gin.Context)
}

type authController struct {
}

func NewAuthController() AuthController {
	return &authController{}
}

// Login godoc
//
// @Summary Login
// @Description Login pour casdoor, pas censé être utilisé par un humain
// @Tags login
// @Accept json
// @Produce json
//
// @Success 200
//
// @Failure 404 {object} errors.APIError "Utilisateur non trouvé"
//
// @Router /auth/login [get]
func (ac *authController) Login(ctx *gin.Context) {
	codeParam := ctx.Query("code")
	stateParam := ctx.Query("state")

	token, err := casdoorsdk.GetOAuthToken(codeParam, stateParam)
	if err != nil {
		panic(err)
	}

	claims, err := casdoorsdk.ParseJwtToken(token.AccessToken)
	if err != nil {
		panic(err)
	}

	claims.AccessToken = token.AccessToken

	data, _ := json.Marshal(claims)
	ctx.Set("user", data)

	// Temporary redirect to Swagger, should be to the frontend !
	ctx.Redirect(http.StatusFound, "/swagger/index.html")

}
