package services

import (
	"time"

	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/repositories"

	config "soli/formations/src/configuration"

	"gorm.io/gorm"
)

type RefreshService struct {
	DB *gorm.DB
}

func (r RefreshService) getUserRepository() repositories.UserRepository {
	return repositories.NewUserRepository(r.DB)
}

func (r RefreshService) RefreshTokens(refreshToken *dto.UserRefreshTokenInput, config *config.Configuration) (*dto.UserTokens, error) {
	jwtService := &JwtService{}
	id, err := jwtService.ParseJWT(refreshToken.RefreshToken, config.SecretRefreshJwt)

	if err != nil {
		return nil, err
	}

	newToken, errToken := jwtService.CreateJWT(*id, time.Now().Add(time.Minute*15), config.SecretJwt)
	newRefreshToken, errRefreshToken := jwtService.CreateJWT(*id, time.Now().Add(time.Hour*24*7), config.SecretRefreshJwt)

	if errToken != nil {
		return nil, errToken
	}

	if errRefreshToken != nil {
		return nil, errRefreshToken
	}

	tokens := dto.UserTokens{Token: newToken, RefreshToken: newRefreshToken}

	_, errUpdate := r.getUserRepository().EditUserToken(*id, tokens)

	if errUpdate != nil {
		return nil, errUpdate
	}
	return &dto.UserTokens{Token: newToken, RefreshToken: newRefreshToken}, nil
}
