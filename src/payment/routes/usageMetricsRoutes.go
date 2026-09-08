package paymentController

import (
	"github.com/gin-gonic/gin"

	auth "soli/formations/src/auth"

	"gorm.io/gorm"
)

// UsageMetricsRoutes définit les routes pour les métriques d'utilisation
func UsageMetricsRoutes(router *gin.RouterGroup, db *gorm.DB) {
	usageController := NewUsageMetricsController(db)
	authMiddleware := auth.NewAuthMiddleware(db)

	routes := router.Group("/usage-metrics")

	// Routes spécialisées
	routes.GET("/user", authMiddleware.AuthManagement(), usageController.GetUserUsageMetrics)
	routes.POST("/increment", authMiddleware.AuthManagement(), usageController.IncrementUsageMetric)
	routes.POST("/reset", authMiddleware.AuthManagement(), usageController.ResetUserUsage)
}
