package organisationController

import (
	"github.com/gin-gonic/gin"

	"soli/formations/src/auth/middleware"
	"soli/formations/src/auth/services"
	config "soli/formations/src/configuration"

	"gorm.io/gorm"
)

func OrganisationsRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	organisationService := services.NewOrganisationService(db)
	organisationController := NewOrganisationController(db, organisationService, config)

	authMiddleware := &middleware.AuthMiddleware{
		DB:     db,
		Config: config,
	}

	genericService := services.NewGenericService(db)
	permissionMiddleware := middleware.NewPermissionsMiddleware(db, genericService)

	routes := router.Group("/organisations")

	routes.POST("/", authMiddleware.CheckIsLogged(), organisationController.AddOrganisation)
	routes.GET("/", authMiddleware.CheckIsLogged(), organisationController.GetOrganisations)
	routes.GET("/:id", authMiddleware.CheckIsLogged(), permissionMiddleware.IsAuthorized(), organisationController.GetOrganisation)
	routes.DELETE("/:id", authMiddleware.CheckIsLogged(), permissionMiddleware.IsAuthorized(), organisationController.DeleteOrganisation)
	routes.PUT("/:id", authMiddleware.CheckIsLogged(), permissionMiddleware.IsAuthorized(), organisationController.EditOrganisation)
}
