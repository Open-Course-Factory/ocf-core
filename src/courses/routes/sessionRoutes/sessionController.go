package sessionController

import (
	services "soli/formations/src/courses/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type SessionController interface {
	AddSession(ctx *gin.Context)
	DeleteSession(ctx *gin.Context)
	GetSessions(ctx *gin.Context)
}

type sessionController struct {
	service services.SessionService
}

func NewSessionController(db *gorm.DB) SessionController {
	return &sessionController{
		service: services.NewSessionService(db),
	}
}
