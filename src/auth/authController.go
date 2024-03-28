package authController

import (
	"encoding/json"
	"net/http"
	"soli/formations/src/auth/dto"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/gin-gonic/gin"
)

type AuthController interface {
	Callback(ctx *gin.Context)
	Login(ctx *gin.Context)
}

type authController struct {
}

func NewAuthController() AuthController {
	return &authController{}
}

// Callback godoc
//
// @Summary Callback
// @Description callback pour casdoor
// @Tags callback
// @Accept json
// @Produce json
//
// @Success 200
//
// @Failure 404 {object} errors.APIError "Utilisateur non trouvé"
//
// @Router /auth/callback [get]
func (ac *authController) Callback(ctx *gin.Context) {
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

// Login godoc
//
// @Summary Login
// @Description Login utilisateur
// @Tags login
// @Accept json
// @Produce json
//
// @Param		login	body		dto.LoginInput	true	"login"
// @Success		201		{object}	dto.LoginOutput
//
// @Failure 404 {object} errors.APIError "Utilisateur non trouvé"
//
// @Router /auth/login [post]
func (ac *authController) Login(ctx *gin.Context) {
	var loginOutput *dto.LoginOutput

	ctx.JSON(http.StatusCreated, loginOutput)
}
