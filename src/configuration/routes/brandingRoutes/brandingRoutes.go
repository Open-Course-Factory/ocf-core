package brandingRoutes

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	auth "soli/formations/src/auth"
	"soli/formations/src/configuration/services"
)

func BrandingRoutes(router *gin.RouterGroup, db *gorm.DB) {
	controller := NewBrandingController(services.NewBrandingService(db))
	MountBrandingRoutes(router, controller, auth.NewAuthMiddleware(db).AuthManagement())
}

// MountBrandingRoutes takes the authentication middleware as a parameter so
// tests can drive the real route table without Casdoor. The reads stay public:
// the login page shows the identity before anyone signs in.
func MountBrandingRoutes(router *gin.RouterGroup, controller *BrandingController, authManagement gin.HandlerFunc) {
	router.GET("/branding", controller.GetBranding)
	router.GET("/branding/logo", controller.GetLogo)
	router.PUT("/branding", authManagement, controller.UpdateBranding)
}
