package chapterController

import (
	controller "soli/formations/src/entityManagement/routes"
	services "soli/formations/src/entityManagement/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ChapterController interface {
	AddChapter(ctx *gin.Context)
	DeleteChapter(ctx *gin.Context)
	GetChapters(ctx *gin.Context)
	GetChapter(ctx *gin.Context)
	EditChapter(ctx *gin.Context)
}

type chapterController struct {
	controller.GenericController
	service services.GenericService
}

func NewChapterController(db *gorm.DB) ChapterController {
	return &chapterController{
		GenericController: controller.NewGenericController(db),
		service:           services.NewGenericService(db),
	}
}
