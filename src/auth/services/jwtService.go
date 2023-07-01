package services

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
)

type UserClaim struct {
	jwt.RegisteredClaims
	Id int
}

type JwtService struct {
}

func (u JwtService) CreateJWT(userId uuid.UUID, time time.Time, secret string) (string, error) {
	claims := jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(time),
		Subject:   userId.String(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	return token.SignedString([]byte(secret))
}

func (u JwtService) ParseJWT(tokenString string, secret string) (*uuid.UUID, error) {

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	})

	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(jwt.MapClaims)

	if !ok {
		return nil, fmt.Errorf("cannot get claims from token")
	}

	idString := claims["sub"].(string)

	id, errParse := uuid.Parse(idString)

	if errParse != nil {
		return nil, err
	}

	return &id, nil
}

func (u JwtService) AddTimeInJwt(userClaim UserClaim, time *jwt.NumericDate, secret string) (string, error) {
	userClaim.ExpiresAt = time
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, userClaim)
	return token.SignedString([]byte(secret))
}
