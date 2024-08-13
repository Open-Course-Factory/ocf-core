package sessionController

import (
	services "soli/formations/src/courses/services"
	controller "soli/formations/src/entityManagement/routes"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type SessionController interface {
	AddSession(ctx *gin.Context)
	DeleteSession(ctx *gin.Context)
	GetSessions(ctx *gin.Context)
}

type sessionController struct {
	controller.GenericController
	service services.SessionService
}

func NewSessionController(db *gorm.DB) SessionController {
	return &sessionController{
		GenericController: controller.NewGenericController(db),
		service:           services.NewSessionService(db),
	}
}
