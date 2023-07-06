package organisationController

import (
	"github.com/gin-gonic/gin"

	"soli/formations/src/auth/middleware"
	config "soli/formations/src/configuration"

	"gorm.io/gorm"
)

func OrganisationsRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	organisationController := NewOrganisationController(db, config)

	authMiddleware := &middleware.AuthMiddleware{
		DB:     db,
		Config: config,
	}

	permissionMiddleware := &middleware.OrganisationPermissionsMiddleware{
		DB: db,
	}

	routes := router.Group("/organisations")

	routes.POST("/", authMiddleware.CheckIsLogged(), organisationController.AddOrganisation)
	routes.GET("/", authMiddleware.CheckIsLogged(), organisationController.GetOrganisations)
	routes.GET("/:id", authMiddleware.CheckIsLogged(), permissionMiddleware.IsAuthorized(), organisationController.GetOrganisation)
	routes.DELETE("/:id", authMiddleware.CheckIsLogged(), permissionMiddleware.IsAuthorized(), organisationController.DeleteOrganisation)
	routes.PUT("/:id", authMiddleware.CheckIsLogged(), permissionMiddleware.IsAuthorized(), organisationController.EditOrganisation)
}
