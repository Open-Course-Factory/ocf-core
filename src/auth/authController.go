package authController

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
//	@Param			login	body		dto.LoginInput	true	"Données de connexion"
//	@Success		201		{object}	dto.LoginOutput "Connexion réussie"
//
//	@Failure		400		{object}	errors.APIError	"Données invalides"
//	@Failure		404		{object}	errors.APIError	"Utilisateur non trouvé"
//	@Failure		401		{object}	errors.APIError	"Identifiants incorrects"
//	@Failure		500		{object}	errors.APIError	"Erreur serveur"
//
//	@Router			/auth/login [post]
func (ac *authController) Login(ctx *gin.Context) {

	user, shouldReturn := getUserFromContext(ctx)
	if shouldReturn {
		return
	}

	resp, errPostToCasdoor := LoginToCasdoor(user, "")
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

	// SECURITY: Validate that the token's user ID matches the expected user
	claims, errParse := casdoorsdk.ParseJwtToken(response.AccessToken)
	if errParse != nil {
		fmt.Printf("[SECURITY ERROR] Failed to parse JWT token during validation: %v\n", errParse)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to validate authentication token",
		})
		return
	}

	// Verify the token's user ID matches the expected user
	if claims.Id != user.Id {
		fmt.Printf("[SECURITY ALERT] Token user ID mismatch! Expected: %s, Got: %s (Expected user: %s, Token user: %s)\n",
			user.Id, claims.Id, user.Name, claims.User.Name)
		ctx.JSON(http.StatusUnauthorized, &errors.APIError{
			ErrorCode:    http.StatusUnauthorized,
			ErrorMessage: "Authentication token validation failed - user mismatch",
		})
		return
	}

	fmt.Printf("[SECURITY] Token validation passed for user %s (ID: %s)\n", user.Name, user.Id)

	roles := getUserRoles(user)

	loginOutputDto := &dto.LoginOutput{
		UserName:         user.Name,
		DisplayName:      user.DisplayName,
		UserId:           user.Id,
		AccessToken:      response.AccessToken,
		RenewAccessToken: response.RefreshToken,
		UserRoles:        roles,
	}

	fmt.Println("Login successful.\nYou are connected as: " + loginOutputDto.UserName)

	ctx.JSON(http.StatusCreated, loginOutputDto)
}

func LoginToCasdoor(user *casdoorsdk.User, password string) (*http.Response, error) {
	passwordToTest := user.Password
	if password != "" {
		passwordToTest = password
	}

	// SECURITY FIX: Properly URL-encode all parameters to prevent injection and handle special characters
	requestURL := fmt.Sprintf("%s/api/login/oauth/access_token?grant_type=password&client_id=%s&client_secret=%s&username=%s&password=%s",
		os.Getenv("CASDOOR_ENDPOINT"),
		url.QueryEscape(os.Getenv("CASDOOR_CLIENT_ID")),
		url.QueryEscape(os.Getenv("CASDOOR_CLIENT_SECRET")),
		url.QueryEscape(user.Name),
		url.QueryEscape(passwordToTest),
	)

	// DEBUG: Log the request details
	fmt.Printf("[DEBUG LOGIN] User.Name=%s, User.Id=%s, User.Email=%s\n", user.Name, user.Id, user.Email)
	fmt.Printf("[DEBUG LOGIN] Calling Casdoor OAuth URL (password hidden)\n")

	resp, errPostToCasdoor := http.Post(requestURL, "application/json", nil)
	if errPostToCasdoor != nil {
		return nil, errPostToCasdoor
	}

	// DEBUG: Log response
	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("[DEBUG LOGIN] Casdoor response (truncated): %.200s...\n", string(body))

	// Return a new response with the body we just read
	resp.Body = io.NopCloser(bytes.NewReader(body))

	return resp, nil
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

	if user.Name == "" {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Invalid user data",
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
