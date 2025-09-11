// src/entityManagement/swagger/routeGenerator.go
package swagger

import (
	"log"
	"strings"

	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	controller "soli/formations/src/entityManagement/routes"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// SwaggerRouteGenerator génère des routes documentées basées sur les métadonnées
type SwaggerRouteGenerator struct {
	genericController controller.GenericController
}

func NewSwaggerRouteGenerator(db *gorm.DB) *SwaggerRouteGenerator {
	return &SwaggerRouteGenerator{
		genericController: controller.NewGenericController(db),
	}
}

// RegisterDocumentedRoutes enregistre toutes les routes documentées
func (srg *SwaggerRouteGenerator) RegisterDocumentedRoutes(router *gin.RouterGroup, authMiddleware gin.HandlerFunc) {
	swaggerConfigs := ems.GlobalEntityRegistrationService.GetAllSwaggerConfigs()

	for entityName, config := range swaggerConfigs {
		srg.registerEntityRoutes(router, entityName, config, authMiddleware)
	}
}

// registerEntityRoutes crée les routes pour une entité spécifique
func (srg *SwaggerRouteGenerator) registerEntityRoutes(router *gin.RouterGroup, entityName string, config *entityManagementInterfaces.EntitySwaggerConfig, authMiddleware gin.HandlerFunc) {
	// Déterminer le path de base (pluriel, en minuscules)
	basePath := "/" + strings.ToLower(ems.Pluralize(entityName))
	entityGroup := router.Group(basePath)

	log.Printf("📚 Registering documented routes for %s at %s", entityName, basePath)

	// Route GET /entities (Get All)
	if config.GetAll != nil {
		handler := srg.createGetAllHandler(entityName, config.GetAll)
		if config.GetAll.Security {
			entityGroup.GET("", authMiddleware, handler)
		} else {
			entityGroup.GET("", handler)
		}
		log.Printf("  ✅ GET %s (GetAll)", basePath)
	}

	// Route GET /entities/:id (Get One)
	if config.GetOne != nil {
		handler := srg.createGetOneHandler(entityName, config.GetOne)
		if config.GetOne.Security {
			entityGroup.GET("/:id", authMiddleware, handler)
		} else {
			entityGroup.GET("/:id", handler)
		}
		log.Printf("  ✅ GET %s/:id (GetOne)", basePath)
	}

	// Route POST /entities (Create)
	if config.Create != nil {
		handler := srg.createCreateHandler(entityName, config.Create)
		if config.Create.Security {
			entityGroup.POST("", authMiddleware, handler)
		} else {
			entityGroup.POST("", handler)
		}
		log.Printf("  ✅ POST %s (Create)", basePath)
	}

	// Route PATCH /entities/:id (Update)
	if config.Update != nil {
		handler := srg.createUpdateHandler(entityName, config.Update)
		if config.Update.Security {
			entityGroup.PATCH("/:id", authMiddleware, handler)
		} else {
			entityGroup.PATCH("/:id", handler)
		}
		log.Printf("  ✅ PATCH %s/:id (Update)", basePath)
	}

	// Route DELETE /entities/:id (Delete)
	if config.Delete != nil {
		handler := srg.createDeleteHandler(entityName, config.Delete)
		if config.Delete.Security {
			entityGroup.DELETE("/:id", authMiddleware, handler)
		} else {
			entityGroup.DELETE("/:id", handler)
		}
		log.Printf("  ✅ DELETE %s/:id (Delete)", basePath)
	}
}

// Handlers avec documentation automatique

func (srg *SwaggerRouteGenerator) createGetAllHandler(entityName string, operation *entityManagementInterfaces.SwaggerOperation) gin.HandlerFunc {
	return gin.HandlerFunc(func(ctx *gin.Context) {
		// Ajouter des métadonnées Swagger dans les headers pour la documentation
		ctx.Header("X-Swagger-Summary", operation.Summary)
		ctx.Header("X-Swagger-Description", operation.Description)
		ctx.Header("X-Swagger-Tags", strings.Join(operation.Tags, ","))

		// Déléguer au contrôleur générique
		srg.genericController.GetEntities(ctx)
	})
}

func (srg *SwaggerRouteGenerator) createGetOneHandler(entityName string, operation *entityManagementInterfaces.SwaggerOperation) gin.HandlerFunc {
	return gin.HandlerFunc(func(ctx *gin.Context) {
		ctx.Header("X-Swagger-Summary", operation.Summary)
		ctx.Header("X-Swagger-Description", operation.Description)
		ctx.Header("X-Swagger-Tags", strings.Join(operation.Tags, ","))

		srg.genericController.GetEntity(ctx)
	})
}

func (srg *SwaggerRouteGenerator) createCreateHandler(entityName string, operation *entityManagementInterfaces.SwaggerOperation) gin.HandlerFunc {
	return gin.HandlerFunc(func(ctx *gin.Context) {
		ctx.Header("X-Swagger-Summary", operation.Summary)
		ctx.Header("X-Swagger-Description", operation.Description)
		ctx.Header("X-Swagger-Tags", strings.Join(operation.Tags, ","))

		srg.genericController.AddEntity(ctx)
	})
}

func (srg *SwaggerRouteGenerator) createUpdateHandler(entityName string, operation *entityManagementInterfaces.SwaggerOperation) gin.HandlerFunc {
	return gin.HandlerFunc(func(ctx *gin.Context) {
		ctx.Header("X-Swagger-Summary", operation.Summary)
		ctx.Header("X-Swagger-Description", operation.Description)
		ctx.Header("X-Swagger-Tags", strings.Join(operation.Tags, ","))

		srg.genericController.EditEntity(ctx)
	})
}

func (srg *SwaggerRouteGenerator) createDeleteHandler(entityName string, operation *entityManagementInterfaces.SwaggerOperation) gin.HandlerFunc {
	return gin.HandlerFunc(func(ctx *gin.Context) {
		ctx.Header("X-Swagger-Summary", operation.Summary)
		ctx.Header("X-Swagger-Description", operation.Description)
		ctx.Header("X-Swagger-Tags", strings.Join(operation.Tags, ","))

		srg.genericController.DeleteEntity(ctx, true)
	})
}

// DocumentationGenerator génère la documentation Swagger au format OpenAPI
type DocumentationGenerator struct{}

func NewDocumentationGenerator() *DocumentationGenerator {
	return &DocumentationGenerator{}
}

// GenerateOpenAPISpec génère la spécification OpenAPI pour toutes les entités documentées
func (dg *DocumentationGenerator) GenerateOpenAPISpec() map[string]interface{} {
	swaggerConfigs := ems.GlobalEntityRegistrationService.GetAllSwaggerConfigs()

	spec := map[string]interface{}{
		"openapi": "3.0.0",
		"info": map[string]interface{}{
			"title":       "OCF API - Auto-generated",
			"version":     "1.0.0",
			"description": "API documentation automatically generated from entity metadata",
		},
		"paths": make(map[string]interface{}),
		"components": map[string]interface{}{
			"schemas": make(map[string]interface{}),
		},
	}

	paths := spec["paths"].(map[string]interface{})

	for entityName, config := range swaggerConfigs {
		basePath := "/" + strings.ToLower(ems.Pluralize(entityName))

		// Générer les paths pour cette entité
		if config.GetAll != nil {
			paths[basePath] = dg.generatePathSpec(config.GetAll, "get")
		}
		if config.Create != nil {
			if paths[basePath] == nil {
				paths[basePath] = make(map[string]interface{})
			}
			paths[basePath].(map[string]interface{})["post"] = dg.generateOperationSpec(config.Create)
		}

		// Path avec ID
		pathWithId := basePath + "/{id}"
		if config.GetOne != nil {
			paths[pathWithId] = dg.generatePathSpec(config.GetOne, "get")
		}
		if config.Update != nil {
			if paths[pathWithId] == nil {
				paths[pathWithId] = make(map[string]interface{})
			}
			paths[pathWithId].(map[string]interface{})["patch"] = dg.generateOperationSpec(config.Update)
		}
		if config.Delete != nil {
			if paths[pathWithId] == nil {
				paths[pathWithId] = make(map[string]interface{})
			}
			paths[pathWithId].(map[string]interface{})["delete"] = dg.generateOperationSpec(config.Delete)
		}
	}

	return spec
}

func (dg *DocumentationGenerator) generatePathSpec(operation *entityManagementInterfaces.SwaggerOperation, method string) map[string]interface{} {
	return map[string]interface{}{
		method: dg.generateOperationSpec(operation),
	}
}

func (dg *DocumentationGenerator) generateOperationSpec(operation *entityManagementInterfaces.SwaggerOperation) map[string]interface{} {
	spec := map[string]interface{}{
		"summary":     operation.Summary,
		"description": operation.Description,
		"tags":        operation.Tags,
	}

	if operation.Security {
		spec["security"] = []map[string]interface{}{
			{"Bearer": []string{}},
		}
	}

	return spec
}
