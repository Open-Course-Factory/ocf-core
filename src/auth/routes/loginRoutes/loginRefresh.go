package loginController

import (
	"net/http"

	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/errors"
	"soli/formations/src/auth/services"

	"github.com/gin-gonic/gin"
)

// Refresh godoc
//
//	@Summary		Refresh token
//	@Description	Rafraichissement du Refresh Token
//	@Tags			refresh
//	@Accept			json
//	@Produce		json
//	@Param			refresh	body		dto.UserRefreshTokenInput	true	"refresh token"
//	@Success		200		{object}	dto.UserTokens
//
//	@Failure		403		{object}	errors.APIError	"Refresh Token non valide"
//	@Failure		403		{object}	errors.APIError	"Impossible de générer le token"
//	@Router			/refresh [post]
func (l loginController) RefreshToken(ctx *gin.Context) {
	refreshToken := &dto.UserRefreshTokenInput{}
	refreshService := services.RefreshService{DB: l.db}

	errBind := ctx.BindJSON(&refreshToken)

	if errBind != nil {
		ctx.JSON(http.StatusForbidden,
			&errors.APIError{ErrorCode: 403, ErrorMessage: errBind.Error()})
		return
	}

	userTokens, err := refreshService.RefreshTokens(refreshToken, l.config)

	if err != nil {
		ctx.JSON(http.StatusForbidden,
			&errors.APIError{ErrorCode: http.StatusForbidden, ErrorMessage: err.Error()})
		return
	}

	ctx.JSON(http.StatusOK,
		&dto.UserTokens{Token: userTokens.Token, RefreshToken: userTokens.RefreshToken})
}
