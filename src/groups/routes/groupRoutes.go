package routes

import (
	config "soli/formations/src/configuration"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GroupRoutes registers custom (non-CRUD) group endpoints. CRUD is handled by
// the entityManagement framework. There are currently no custom group routes
// — this function is kept for future extensions and to preserve the registration
// call site in main.go.
func GroupRoutes(_ *gin.RouterGroup, _ *config.Configuration, _ *gorm.DB) {
}
