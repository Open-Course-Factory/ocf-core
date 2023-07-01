package loginController

import (
	"soli/formations/src/auth/services"
	config "soli/formations/src/configuration"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type LoginController interface {
	Login(ctx *gin.Context)
	RefreshToken(ctx *gin.Context)
}

type loginController struct {
	db      *gorm.DB
	service services.UserService
	config  *config.Configuration
}

func NewLoginController(db *gorm.DB, config *config.Configuration) LoginController {

	controller := &loginController{
		db:      db,
		service: services.NewUserService(db),
		config:  config,
	}
	return controller
}
