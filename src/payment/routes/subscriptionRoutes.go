// src/payment/routes/subscriptionRoutes.go
package paymentController

import (
	"github.com/gin-gonic/gin"

	auth "soli/formations/src/auth"
	config "soli/formations/src/configuration"

	"gorm.io/gorm"
)

// SubscriptionRoutes définit les routes pour les abonnements
func SubscriptionRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	subscriptionController := NewSubscriptionController(db)
	authMiddleware := auth.NewAuthMiddleware(db)
	//usageLimitMiddleware := middleware.NewUsageLimitMiddleware(db)

	routes := router.Group("/subscriptions")

	// Routes génériques pour les plans d'abonnement (admin seulement)
	routes.GET("/plans", authMiddleware.AuthManagement(), subscriptionController.GetEntities)
	routes.POST("/plans", authMiddleware.AuthManagement(), subscriptionController.AddEntity)
	routes.GET("/plans/:id", authMiddleware.AuthManagement(), subscriptionController.GetEntity)
	routes.PATCH("/plans/:id", authMiddleware.AuthManagement(), subscriptionController.EditEntity)
	routes.DELETE("/plans/:id", authMiddleware.AuthManagement(), subscriptionController.DeleteEntity)

	// Routes spécialisées pour les abonnements utilisateur
	routes.POST("/checkout", authMiddleware.AuthManagement(), subscriptionController.CreateCheckoutSession)
	routes.POST("/portal", authMiddleware.AuthManagement(), subscriptionController.CreatePortalSession)
	routes.GET("/current", authMiddleware.AuthManagement(), subscriptionController.GetUserSubscription)
	routes.POST("/:id/cancel", authMiddleware.AuthManagement(), subscriptionController.CancelSubscription)
	routes.POST("/:id/reactivate", authMiddleware.AuthManagement(), subscriptionController.ReactivateSubscription)

	// Analytics (admin seulement)
	routes.GET("/analytics", authMiddleware.AuthManagement(), subscriptionController.GetSubscriptionAnalytics)

	// Usage monitoring
	routes.POST("/usage/check", authMiddleware.AuthManagement(), subscriptionController.CheckUsageLimit)
	routes.GET("/usage", authMiddleware.AuthManagement(), subscriptionController.GetUserUsage)
}

// PaymentMethodRoutes définit les routes pour les moyens de paiement
func PaymentMethodRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	paymentMethodController := NewPaymentMethodController(db)
	authMiddleware := auth.NewAuthMiddleware(db)

	routes := router.Group("/payment-methods")

	// Routes génériques
	routes.GET("", authMiddleware.AuthManagement(), paymentMethodController.GetEntities)
	routes.POST("", authMiddleware.AuthManagement(), paymentMethodController.AddEntity)
	routes.DELETE("/:id", authMiddleware.AuthManagement(), paymentMethodController.DeleteEntity)

	// Routes spécialisées
	routes.GET("/user", authMiddleware.AuthManagement(), paymentMethodController.GetUserPaymentMethods)
	routes.POST("/:id/set-default", authMiddleware.AuthManagement(), paymentMethodController.SetDefaultPaymentMethod)
}

// InvoiceRoutes définit les routes pour les factures
func InvoiceRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	invoiceController := NewInvoiceController(db)
	authMiddleware := auth.NewAuthMiddleware(db)

	routes := router.Group("/invoices")

	// Routes génériques (admin principalement)
	routes.GET("", authMiddleware.AuthManagement(), invoiceController.GetEntities)
	routes.GET("/:id", authMiddleware.AuthManagement(), invoiceController.GetEntity)

	// Routes spécialisées
	routes.GET("/user", authMiddleware.AuthManagement(), invoiceController.GetUserInvoices)
	routes.GET("/:id/download", authMiddleware.AuthManagement(), invoiceController.DownloadInvoice)
}

// BillingAddressRoutes définit les routes pour les adresses de facturation
func BillingAddressRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	billingController := NewBillingAddressController(db)
	authMiddleware := auth.NewAuthMiddleware(db)

	routes := router.Group("/billing-addresses")

	// Routes génériques
	routes.GET("", authMiddleware.AuthManagement(), billingController.GetEntities)
	routes.POST("", authMiddleware.AuthManagement(), billingController.AddEntity)
	routes.GET("/:id", authMiddleware.AuthManagement(), billingController.GetEntity)
	routes.PATCH("/:id", authMiddleware.AuthManagement(), billingController.EditEntity)
	routes.DELETE("/:id", authMiddleware.AuthManagement(), billingController.DeleteEntity)

	// Routes spécialisées
	routes.GET("/user", authMiddleware.AuthManagement(), billingController.GetUserBillingAddresses)
	routes.POST("/:id/set-default", authMiddleware.AuthManagement(), billingController.SetDefaultBillingAddress)
}

// WebhookRoutes définit les routes pour les webhooks (pas d'authentification)
func WebhookRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	webhookController := NewWebhookController(db)

	routes := router.Group("/webhooks")

	// Webhook Stripe (pas d'auth car Stripe appelle directement)
	routes.POST("/stripe", webhookController.HandleStripeWebhook)
}

// UsageMetricsRoutes définit les routes pour les métriques d'utilisation
func UsageMetricsRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	usageController := NewUsageMetricsController(db)
	authMiddleware := auth.NewAuthMiddleware(db)

	routes := router.Group("/usage-metrics")

	// Routes génériques (principalement pour les admins)
	routes.GET("", authMiddleware.AuthManagement(), usageController.GetEntities)
	routes.GET("/:id", authMiddleware.AuthManagement(), usageController.GetEntity)
	routes.PATCH("/:id", authMiddleware.AuthManagement(), usageController.EditEntity)

	// Routes spécialisées
	routes.GET("/user", authMiddleware.AuthManagement(), usageController.GetUserUsageMetrics)
	routes.POST("/increment", authMiddleware.AuthManagement(), usageController.IncrementUsageMetric)
	routes.POST("/reset", authMiddleware.AuthManagement(), usageController.ResetUserUsage)
}
