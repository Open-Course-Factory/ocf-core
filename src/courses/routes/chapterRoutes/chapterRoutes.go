package chapterController

import (
	"github.com/gin-gonic/gin"

	config "soli/formations/src/configuration"

	"gorm.io/gorm"

	auth "soli/formations/src/auth"
)

func ChaptersRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	chapterController := NewChapterController(db)

	routes := router.Group("/chapters")

	middleware := auth.NewAuthMiddleware(db)

	routes.GET("", middleware.AuthManagement(), chapterController.GetChapters)
	routes.GET("/:id", middleware.AuthManagement(), chapterController.GetChapter)
	routes.POST("", middleware.AuthManagement(), chapterController.AddChapter)
	routes.PATCH("/:id", middleware.AuthManagement(), chapterController.EditChapter)

	routes.DELETE("/:id", middleware.AuthManagement(), chapterController.DeleteChapter)
}
