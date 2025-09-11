// src/entityManagement/swagger/routeGenerator.go
package swagger

import (
	"fmt"
	"log"
	"reflect"
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
		handler := srg.createGetAllHandler(config.GetAll)
		if config.GetAll.Security {
			entityGroup.GET("", authMiddleware, handler)
		} else {
			entityGroup.GET("", handler)
		}
		log.Printf("  ✅ GET %s (GetAll)", basePath)
	}

	// Route GET /entities/:id (Get One)
	if config.GetOne != nil {
		handler := srg.createGetOneHandler(config.GetOne)
		if config.GetOne.Security {
			entityGroup.GET("/:id", authMiddleware, handler)
		} else {
			entityGroup.GET("/:id", handler)
		}
		log.Printf("  ✅ GET %s/:id (GetOne)", basePath)
	}

	// Route POST /entities (Create)
	if config.Create != nil {
		handler := srg.createCreateHandler(config.Create)
		if config.Create.Security {
			entityGroup.POST("", authMiddleware, handler)
		} else {
			entityGroup.POST("", handler)
		}
		log.Printf("  ✅ POST %s (Create)", basePath)
	}

	// Route PATCH /entities/:id (Update)
	if config.Update != nil {
		handler := srg.createUpdateHandler(config.Update)
		if config.Update.Security {
			entityGroup.PATCH("/:id", authMiddleware, handler)
		} else {
			entityGroup.PATCH("/:id", handler)
		}
		log.Printf("  ✅ PATCH %s/:id (Update)", basePath)
	}

	// Route DELETE /entities/:id (Delete)
	if config.Delete != nil {
		handler := srg.createDeleteHandler(config.Delete)
		if config.Delete.Security {
			entityGroup.DELETE("/:id", authMiddleware, handler)
		} else {
			entityGroup.DELETE("/:id", handler)
		}
		log.Printf("  ✅ DELETE %s/:id (Delete)", basePath)
	}
}

// Handlers avec documentation automatique

func (srg *SwaggerRouteGenerator) createGetAllHandler(operation *entityManagementInterfaces.SwaggerOperation) gin.HandlerFunc {
	return gin.HandlerFunc(func(ctx *gin.Context) {
		// Ajouter des métadonnées Swagger dans les headers pour la documentation
		ctx.Header("X-Swagger-Summary", operation.Summary)
		ctx.Header("X-Swagger-Description", operation.Description)
		ctx.Header("X-Swagger-Tags", strings.Join(operation.Tags, ","))

		// Déléguer au contrôleur générique
		srg.genericController.GetEntities(ctx)
	})
}

func (srg *SwaggerRouteGenerator) createGetOneHandler(operation *entityManagementInterfaces.SwaggerOperation) gin.HandlerFunc {
	return gin.HandlerFunc(func(ctx *gin.Context) {
		ctx.Header("X-Swagger-Summary", operation.Summary)
		ctx.Header("X-Swagger-Description", operation.Description)
		ctx.Header("X-Swagger-Tags", strings.Join(operation.Tags, ","))

		srg.genericController.GetEntity(ctx)
	})
}

func (srg *SwaggerRouteGenerator) createCreateHandler(operation *entityManagementInterfaces.SwaggerOperation) gin.HandlerFunc {
	return gin.HandlerFunc(func(ctx *gin.Context) {
		ctx.Header("X-Swagger-Summary", operation.Summary)
		ctx.Header("X-Swagger-Description", operation.Description)
		ctx.Header("X-Swagger-Tags", strings.Join(operation.Tags, ","))

		srg.genericController.AddEntity(ctx)
	})
}

func (srg *SwaggerRouteGenerator) createUpdateHandler(operation *entityManagementInterfaces.SwaggerOperation) gin.HandlerFunc {
	return gin.HandlerFunc(func(ctx *gin.Context) {
		ctx.Header("X-Swagger-Summary", operation.Summary)
		ctx.Header("X-Swagger-Description", operation.Description)
		ctx.Header("X-Swagger-Tags", strings.Join(operation.Tags, ","))

		srg.genericController.EditEntity(ctx)
	})
}

func (srg *SwaggerRouteGenerator) createDeleteHandler(operation *entityManagementInterfaces.SwaggerOperation) gin.HandlerFunc {
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
			"securitySchemes": map[string]interface{}{
				"Bearer": map[string]interface{}{
					"type":         "http",
					"scheme":       "bearer",
					"bearerFormat": "JWT",
					"description":  "JWT token for authentication",
				},
			},
		},
	}

	paths := spec["paths"].(map[string]interface{})
	schemas := spec["components"].(map[string]interface{})["schemas"].(map[string]interface{})

	for entityName, config := range swaggerConfigs {
		basePath := "/" + strings.ToLower(ems.Pluralize(entityName))

		// Générer les schémas DTOs
		dg.generateSchemasForEntity(schemas, entityName)

		// Générer les paths pour cette entité
		if config.GetAll != nil {
			paths[basePath] = dg.generateGetAllPathSpec(config.GetAll, entityName)
		}
		if config.Create != nil {
			if paths[basePath] == nil {
				paths[basePath] = make(map[string]interface{})
			}
			paths[basePath].(map[string]interface{})["post"] = dg.generateCreateOperationSpec(config.Create, entityName)
		}

		// Path avec ID
		pathWithId := basePath + "/{id}"
		if config.GetOne != nil {
			paths[pathWithId] = dg.generateGetOnePathSpec(config.GetOne, entityName)
		}
		if config.Update != nil {
			if paths[pathWithId] == nil {
				paths[pathWithId] = make(map[string]interface{})
			}
			paths[pathWithId].(map[string]interface{})["patch"] = dg.generateUpdateOperationSpec(config.Update, entityName)
		}
		if config.Delete != nil {
			if paths[pathWithId] == nil {
				paths[pathWithId] = make(map[string]interface{})
			}
			paths[pathWithId].(map[string]interface{})["delete"] = dg.generateDeleteOperationSpec(config.Delete, entityName)
		}
	}

	return spec
}

// Génération des schémas
func (dg *DocumentationGenerator) generateSchemasForEntity(schemas map[string]interface{}, entityName string) {
	// Récupérer les DTOs depuis le service d'enregistrement
	inputCreateDto := ems.GlobalEntityRegistrationService.GetEntityDtos(entityName, ems.InputCreateDto)
	outputDto := ems.GlobalEntityRegistrationService.GetEntityDtos(entityName, ems.OutputDto)
	inputEditDto := ems.GlobalEntityRegistrationService.GetEntityDtos(entityName, ems.InputEditDto)

	// Générer le schéma pour le DTO de création
	if inputCreateDto != nil {
		schemaName := fmt.Sprintf("%sCreateInput", entityName)
		schemas[schemaName] = dg.generateSchemaFromStruct(inputCreateDto)
	}

	// Générer le schéma pour le DTO de sortie
	if outputDto != nil {
		schemaName := fmt.Sprintf("%sOutput", entityName)
		schemas[schemaName] = dg.generateSchemaFromStruct(outputDto)
	}

	// Générer le schéma pour le DTO d'édition
	if inputEditDto != nil {
		schemaName := fmt.Sprintf("%sEditInput", entityName)
		schemas[schemaName] = dg.generateSchemaFromStruct(inputEditDto)
	}
}

// Génération de schéma à partir de struct
func (dg *DocumentationGenerator) generateSchemaFromStruct(dto interface{}) map[string]interface{} {
	schema := map[string]interface{}{
		"type":       "object",
		"properties": make(map[string]interface{}),
	}

	dtoType := reflect.TypeOf(dto)
	if dtoType.Kind() == reflect.Ptr {
		dtoType = dtoType.Elem()
	}

	properties := schema["properties"].(map[string]interface{})
	required := []string{}

	for i := 0; i < dtoType.NumField(); i++ {
		field := dtoType.Field(i)

		// Ignorer les champs non exportés
		if !field.IsExported() {
			continue
		}

		// Récupérer le nom JSON
		jsonTag := field.Tag.Get("json")
		fieldName := field.Name
		if jsonTag != "" && jsonTag != "-" {
			if strings.Contains(jsonTag, ",") {
				fieldName = strings.Split(jsonTag, ",")[0]
			} else {
				fieldName = jsonTag
			}
		} else {
			// Convertir en snake_case
			fieldName = strings.ToLower(fieldName)
		}

		// Vérifier si le champ est requis
		bindingTag := field.Tag.Get("binding")
		if strings.Contains(bindingTag, "required") {
			required = append(required, fieldName)
		}

		// Déterminer le type du champ
		fieldType := dg.getSwaggerTypeFromGoType(field.Type)
		properties[fieldName] = fieldType
	}

	if len(required) > 0 {
		schema["required"] = required
	}

	return schema
}

// Conversion de types Go vers Swagger
func (dg *DocumentationGenerator) getSwaggerTypeFromGoType(goType reflect.Type) map[string]interface{} {
	// Gérer les pointeurs
	if goType.Kind() == reflect.Ptr {
		goType = goType.Elem()
	}

	switch goType.Kind() {
	case reflect.String:
		if goType.Name() == "UUID" || strings.Contains(goType.String(), "uuid.UUID") {
			return map[string]interface{}{
				"type":   "string",
				"format": "uuid",
			}
		}
		return map[string]interface{}{"type": "string"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32:
		return map[string]interface{}{"type": "integer", "format": "int32"}
	case reflect.Int64:
		return map[string]interface{}{"type": "integer", "format": "int64"}
	case reflect.Float32:
		return map[string]interface{}{"type": "number", "format": "float"}
	case reflect.Float64:
		return map[string]interface{}{"type": "number", "format": "double"}
	case reflect.Bool:
		return map[string]interface{}{"type": "boolean"}
	case reflect.Slice:
		return map[string]interface{}{
			"type":  "array",
			"items": dg.getSwaggerTypeFromGoType(goType.Elem()),
		}
	case reflect.Struct:
		if goType.String() == "time.Time" {
			return map[string]interface{}{
				"type":   "string",
				"format": "date-time",
			}
		}
		// Pour les structs complexes, retourner un objet générique
		return map[string]interface{}{"type": "object"}
	default:
		return map[string]interface{}{"type": "string"}
	}
}

// Génération des opérations avec requestBody

func (dg *DocumentationGenerator) generateGetAllPathSpec(operation *entityManagementInterfaces.SwaggerOperation, entityName string) map[string]interface{} {
	return map[string]interface{}{
		"get": dg.generateGetAllOperationSpec(operation, entityName),
	}
}

func (dg *DocumentationGenerator) generateGetOnePathSpec(operation *entityManagementInterfaces.SwaggerOperation, entityName string) map[string]interface{} {
	return map[string]interface{}{
		"get": dg.generateGetOneOperationSpec(operation, entityName),
	}
}

func (dg *DocumentationGenerator) generateGetAllOperationSpec(operation *entityManagementInterfaces.SwaggerOperation, entityName string) map[string]interface{} {
	spec := map[string]interface{}{
		"summary":     operation.Summary,
		"description": operation.Description,
		"tags":        operation.Tags,
		"responses": map[string]interface{}{
			"200": map[string]interface{}{
				"description": "Successful response",
				"content": map[string]interface{}{
					"application/json": map[string]interface{}{
						"schema": map[string]interface{}{
							"type": "array",
							"items": map[string]interface{}{
								"$ref": fmt.Sprintf("#/components/schemas/%sOutput", entityName),
							},
						},
					},
				},
			},
			"500": map[string]interface{}{
				"description": "Internal server error",
			},
		},
	}

	if operation.Security {
		spec["security"] = []map[string]interface{}{
			{"Bearer": []string{}},
		}
	}

	return spec
}

func (dg *DocumentationGenerator) generateGetOneOperationSpec(operation *entityManagementInterfaces.SwaggerOperation, entityName string) map[string]interface{} {
	spec := map[string]interface{}{
		"summary":     operation.Summary,
		"description": operation.Description,
		"tags":        operation.Tags,
		"parameters": []map[string]interface{}{
			{
				"name":        "id",
				"in":          "path",
				"required":    true,
				"description": fmt.Sprintf("ID of the %s", entityName),
				"schema": map[string]interface{}{
					"type":   "string",
					"format": "uuid",
				},
			},
		},
		"responses": map[string]interface{}{
			"200": map[string]interface{}{
				"description": "Successful response",
				"content": map[string]interface{}{
					"application/json": map[string]interface{}{
						"schema": map[string]interface{}{
							"$ref": fmt.Sprintf("#/components/schemas/%sOutput", entityName),
						},
					},
				},
			},
			"404": map[string]interface{}{
				"description": "Entity not found",
			},
		},
	}

	if operation.Security {
		spec["security"] = []map[string]interface{}{
			{"Bearer": []string{}},
		}
	}

	return spec
}

func (dg *DocumentationGenerator) generateCreateOperationSpec(operation *entityManagementInterfaces.SwaggerOperation, entityName string) map[string]interface{} {
	spec := map[string]interface{}{
		"summary":     operation.Summary,
		"description": operation.Description,
		"tags":        operation.Tags,
		"requestBody": map[string]interface{}{
			"required": true,
			"content": map[string]interface{}{
				"application/json": map[string]interface{}{
					"schema": map[string]interface{}{
						"$ref": fmt.Sprintf("#/components/schemas/%sCreateInput", entityName),
					},
				},
			},
		},
		"responses": map[string]interface{}{
			"201": map[string]interface{}{
				"description": "Created successfully",
				"content": map[string]interface{}{
					"application/json": map[string]interface{}{
						"schema": map[string]interface{}{
							"$ref": fmt.Sprintf("#/components/schemas/%sOutput", entityName),
						},
					},
				},
			},
			"400": map[string]interface{}{
				"description": "Bad request",
			},
		},
	}

	if operation.Security {
		spec["security"] = []map[string]interface{}{
			{"Bearer": []string{}},
		}
	}

	return spec
}

func (dg *DocumentationGenerator) generateUpdateOperationSpec(operation *entityManagementInterfaces.SwaggerOperation, entityName string) map[string]interface{} {
	spec := map[string]interface{}{
		"summary":     operation.Summary,
		"description": operation.Description,
		"tags":        operation.Tags,
		"parameters": []map[string]interface{}{
			{
				"name":        "id",
				"in":          "path",
				"required":    true,
				"description": fmt.Sprintf("ID of the %s", entityName),
				"schema": map[string]interface{}{
					"type":   "string",
					"format": "uuid",
				},
			},
		},
		"requestBody": map[string]interface{}{
			"required": true,
			"content": map[string]interface{}{
				"application/json": map[string]interface{}{
					"schema": map[string]interface{}{
						"$ref": fmt.Sprintf("#/components/schemas/%sEditInput", entityName),
					},
				},
			},
		},
		"responses": map[string]interface{}{
			"200": map[string]interface{}{
				"description": "Updated successfully",
			},
			"400": map[string]interface{}{
				"description": "Bad request",
			},
			"404": map[string]interface{}{
				"description": "Entity not found",
			},
		},
	}

	if operation.Security {
		spec["security"] = []map[string]interface{}{
			{"Bearer": []string{}},
		}
	}

	return spec
}

func (dg *DocumentationGenerator) generateDeleteOperationSpec(operation *entityManagementInterfaces.SwaggerOperation, entityName string) map[string]interface{} {
	spec := map[string]interface{}{
		"summary":     operation.Summary,
		"description": operation.Description,
		"tags":        operation.Tags,
		"parameters": []map[string]interface{}{
			{
				"name":        "id",
				"in":          "path",
				"required":    true,
				"description": fmt.Sprintf("ID of the %s", entityName),
				"schema": map[string]interface{}{
					"type":   "string",
					"format": "uuid",
				},
			},
		},
		"responses": map[string]interface{}{
			"204": map[string]interface{}{
				"description": "Deleted successfully",
			},
			"404": map[string]interface{}{
				"description": "Entity not found",
			},
		},
	}

	if operation.Security {
		spec["security"] = []map[string]interface{}{
			{"Bearer": []string{}},
		}
	}

	return spec
}
