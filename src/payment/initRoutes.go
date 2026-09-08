package payment

import (
	paymentController "soli/formations/src/payment/routes"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func InitPaymentRoutes(routerGroup *gin.RouterGroup, db *gorm.DB) {
	paymentController.UserSubscriptionRoutes(routerGroup, db)
	paymentController.SubscriptionPlanRoutes(routerGroup, db)
	paymentController.OrganizationSubscriptionRoutes(routerGroup, db) // Phase 2: Organization subscriptions
	paymentController.BulkLicenseRoutes(routerGroup, db)
	paymentController.PaymentMethodRoutes(routerGroup, db)
	paymentController.InvoiceRoutes(routerGroup, db)
	paymentController.OrganizationRolePlanRoutes(routerGroup, db)
	paymentController.BillingAddressRoutes(routerGroup, db)
	paymentController.UsageMetricsRoutes(routerGroup, db)
	paymentController.WebhookRoutes(routerGroup, db)
	paymentController.HooksRoutes(routerGroup, db)
}
