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

	routes := router.Group("/terminal-sessions")

	// Routes spécialisées pour les fonctionnalités Terminal Trainer
	routes.POST("/start-session", middleware.AuthManagement(), terminalController.StartSession)
	routes.GET("/:id/console", middleware.AuthManagement(), terminalController.ConnectConsole)
	routes.POST("/:id/stop", middleware.AuthManagement(), terminalController.StopSession)
	routes.GET("/user-sessions", middleware.AuthManagement(), terminalController.GetUserSessions)

	// Routes de partage de terminaux
	routes.POST("/:id/share", middleware.AuthManagement(), terminalController.ShareTerminal)                  // Partager un terminal
	routes.DELETE("/:id/share/:user_id", middleware.AuthManagement(), terminalController.RevokeTerminalAccess) // Révoquer l'accès
	routes.GET("/:id/shares", middleware.AuthManagement(), terminalController.GetTerminalShares)              // Voir les partages d'un terminal
	routes.GET("/shared-with-me", middleware.AuthManagement(), terminalController.GetSharedTerminals)         // Terminaux partagés avec moi
	routes.GET("/:id/info", middleware.AuthManagement(), terminalController.GetSharedTerminalInfo)            // Info détaillée d'un terminal

	// Routes de masquage de terminaux
	routes.POST("/:id/hide", middleware.AuthManagement(), terminalController.HideTerminal)     // Masquer un terminal
	routes.DELETE("/:id/hide", middleware.AuthManagement(), terminalController.UnhideTerminal) // Afficher à nouveau un terminal

	routes.POST("/:id/sync", middleware.AuthManagement(), terminalController.SyncSession)       // Sync une session spécifique
	routes.POST("/sync-all", middleware.AuthManagement(), terminalController.SyncAllSessions)   // Sync toutes les sessions
	routes.GET("/:id/status", middleware.AuthManagement(), terminalController.GetSessionStatus) // Comparer statuts local/API

	// Configuration
	routes.GET("/instance-types", middleware.AuthManagement(), terminalController.GetInstanceTypes) // Liste des types d'instances disponibles
}

func UserTerminalKeyRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	userTerminalKeyController := NewUserTerminalKeyController(db)
	middleware := auth.NewAuthMiddleware(db)

	routes := router.Group("/user-terminal-keys")

	// Routes spécialisées
	routes.POST("/regenerate", middleware.AuthManagement(), userTerminalKeyController.RegenerateKey)
	routes.GET("/my-key", middleware.AuthManagement(), userTerminalKeyController.GetMyKey)
}
