package organisationController

import (
	"github.com/gin-gonic/gin"

	"soli/formations/src/auth/middleware"
	config "soli/formations/src/configuration"

	"gorm.io/gorm"
)

func OrganisationsRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	organisationController := NewOrganisationController(db, config)

	middleware := &middleware.AuthMiddleware{
		DB:     db,
		Config: config,
	}

	routes := router.Group("/organisations")

	routes.POST("/", middleware.CheckIsLogged(), organisationController.AddOrganisation)
	routes.GET("/", middleware.CheckIsLogged(), organisationController.GetOrganisations)
	routes.GET("/:id", middleware.CheckIsLogged(), organisationController.GetOrganisation)
	routes.DELETE("/:id", middleware.CheckIsLogged(), organisationController.DeleteOrganisation)
	routes.PUT("/:id", middleware.CheckIsLogged(), organisationController.EditOrganisation)
}
