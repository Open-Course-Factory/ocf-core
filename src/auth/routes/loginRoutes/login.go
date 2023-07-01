package loginController

import (
	"net/http"

	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

// Login godoc
//
//	@Summary		Connexion d'un utilisateur
//	@Description	Connecte un utilisateur avec un identifiant et un mot de passe
//	@Tags			Login
//	@Accept			json
//	@Produce		json
//	@Param			login	body		dto.UserLoginInput	true	"Identifiant et mot de passe"
//	@Success		200		{object}	dto.UserTokens
//	@Failure		400		{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		404		{object}	errors.APIError	"Utilisateur non trouvé ou mot de passe incorrecte"
//	@Router			/login [post]
func (l loginController) Login(ctx *gin.Context) {
	userLoginDTO := &dto.UserLoginInput{}
	bindError := ctx.BindJSON(&userLoginDTO)

	if bindError != nil {
		ctx.JSON(http.StatusBadRequest,
			&errors.APIError{ErrorCode: http.StatusBadRequest, ErrorMessage: bindError.Error()})
		return
	}

	userLogin, userLoginError := l.service.UserLogin(userLoginDTO, l.config)

	if userLoginError != nil {
		ctx.JSON(http.StatusNotFound,
			&errors.APIError{ErrorCode: http.StatusNotFound, ErrorMessage: userLoginError.Error()})
		return
	}

	ctx.JSON(http.StatusOK,
		&dto.UserTokens{Token: userLogin.Token, RefreshToken: userLogin.RefreshToken})
}
