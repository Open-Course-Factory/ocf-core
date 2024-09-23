package authController

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/errors"

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
//	@Summary		Callback
//	@Description	callback pour casdoor
//	@Tags			callback
//	@Accept			json
//	@Produce		json
//
//	@Success		200
//
//	@Failure		404	{object}	errors.APIError	"Utilisateur non trouvé"
//
//	@Router			/auth/callback [get]
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
//	@Summary		Login
//	@Description	Login utilisateur
//	@Tags			login
//	@Accept			json
//	@Produce		json
//
//	@Param			login	body		dto.LoginInput	true	"login"
//	@Success		201		{object}	dto.LoginOutput
//
//	@Failure		404		{object}	errors.APIError	"Utilisateur non trouvé"
//
//	@Router			/auth/login [post]
func (ac *authController) Login(ctx *gin.Context) {

	user, shouldReturn := getUserFromContext(ctx)
	if shouldReturn {
		return
	}

	url := fmt.Sprintf("%s/api/login/oauth/access_token?grant_type=password&client_id=%s&client_secret=%s&username=%s&password=%s",
		os.Getenv("CASDOOR_ENDPOINT"),
		os.Getenv("CASDOOR_CLIENT_ID"),
		os.Getenv("CASDOOR_CLIENT_SECRET"),
		user.Name,
		user.Password,
	)

	resp, errPostToCasdoor := http.Post(url, "application/json", nil)
	if errPostToCasdoor != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: errPostToCasdoor.Error(),
		})
		return
	}
	defer resp.Body.Close()

	body, errReadBody := io.ReadAll(resp.Body)
	if errReadBody != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: errReadBody.Error(),
		})
		return
	}

	if resp.StatusCode >= 400 {
		ctx.JSON(resp.StatusCode, &errors.APIError{
			ErrorCode:    resp.StatusCode,
			ErrorMessage: string(body),
		})
		return
	}

	var response struct {
		AccessToken  string `json:"access_token"`
		ExpireIn     string `json:"expire_in"`
		IdToken      string `json:"id_token"`
		RefreshToken string `json:"refresh_token"`
		Scope        string `json:"scope"`
		TokenType    string `json:"token_type"`
	}

	errUnmarshall := json.Unmarshal(body, &response)
	if errUnmarshall != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: errUnmarshall.Error(),
		})
		return
	}

	if response.AccessToken == "" {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusUnauthorized,
			ErrorMessage: "Invalid auth parameters",
		})
		return
	}

	roles := getUserRoles(user)

	loginOutputDto := &dto.LoginOutput{UserName: user.Name, UserId: user.Id, AccessToken: response.AccessToken, RenewAccessToken: response.RefreshToken, UserRoles: roles}

	fmt.Println("Login successful.\nYou are connected as: " + loginOutputDto.UserName)

	ctx.JSON(http.StatusCreated, loginOutputDto)
}

func getUserFromContext(ctx *gin.Context) (*casdoorsdk.User, bool) {
	loginInputDto := dto.LoginInput{}

	bindError := ctx.BindJSON(&loginInputDto)
	if bindError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Impossible de parser le json",
		})
		return nil, true
	}

	user, errUser := casdoorsdk.GetUserByEmail(loginInputDto.Email)

	if errUser != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: errUser.Error(),
		})
		return nil, true
	}

	if user == nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "User not found",
		})
		return nil, true
	}

	user.Password = loginInputDto.Password
	return user, false
}

func getUserRoles(user *casdoorsdk.User) []string {
	var roles []string

	for _, role := range user.Roles {
		roles = append(roles, role.Name)
	}
	return roles
}
