package terminalController

import (
	"github.com/gin-gonic/gin"

	auth "soli/formations/src/auth"
	config "soli/formations/src/configuration"

	"gorm.io/gorm"
)

func TerminalRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	terminalController := NewTerminalController(db)
	middleware := auth.NewAuthMiddleware(db)

	routes := router.Group("/terminals")

	// Routes génériques (CRUD via le système générique)
	// routes.GET("", middleware.AuthManagement(), terminalController.GetEntities)
	// routes.POST("", middleware.AuthManagement(), terminalController.AddEntity)
	// routes.GET("/:id", middleware.AuthManagement(), terminalController.GetEntity)
	// routes.PATCH("/:id", middleware.AuthManagement(), terminalController.EditEntity)
	// routes.DELETE("/:id", middleware.AuthManagement(), terminalController.DeleteEntity)

	// Routes spécialisées pour les fonctionnalités Terminal Trainer
	routes.POST("/start-session", middleware.AuthManagement(), terminalController.StartSession)
	routes.GET("/:id/console", middleware.AuthManagement(), terminalController.ConnectConsole)
	routes.POST("/:id/stop", middleware.AuthManagement(), terminalController.StopSession)
	routes.GET("/user-sessions", middleware.AuthManagement(), terminalController.GetUserSessions)

	routes.POST("/:id/sync", middleware.AuthManagement(), terminalController.SyncSession)       // Sync une session spécifique
	routes.POST("/sync-all", middleware.AuthManagement(), terminalController.SyncAllSessions)   // Sync toutes les sessions
	routes.GET("/:id/status", middleware.AuthManagement(), terminalController.GetSessionStatus) // Comparer statuts local/API
}

func UserTerminalKeyRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	userTerminalKeyController := NewUserTerminalKeyController(db)
	middleware := auth.NewAuthMiddleware(db)

	routes := router.Group("/user-terminal-keys")

	// Routes génériques (CRUD via le système générique) - principalement pour les admins
	routes.GET("", middleware.AuthManagement(), userTerminalKeyController.GetEntities)
	routes.POST("", middleware.AuthManagement(), userTerminalKeyController.AddEntity)
	routes.GET("/:id", middleware.AuthManagement(), userTerminalKeyController.GetEntity)
	routes.PATCH("/:id", middleware.AuthManagement(), userTerminalKeyController.EditEntity)
	routes.DELETE("/:id", middleware.AuthManagement(), userTerminalKeyController.DeleteEntity)

	// Routes spécialisées
	routes.POST("/regenerate", middleware.AuthManagement(), userTerminalKeyController.RegenerateKey)
	routes.GET("/my-key", middleware.AuthManagement(), userTerminalKeyController.GetMyKey)
}
