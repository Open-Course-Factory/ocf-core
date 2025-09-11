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

// GenerateOpenAPISpec avec génération complète de schémas
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
			"schemas": dg.generateSchemas(swaggerConfigs),
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

	for entityName, config := range swaggerConfigs {
		basePath := "/" + strings.ToLower(ems.Pluralize(entityName))

		// GET /entities (Get All)
		if config.GetAll != nil {
			if paths[basePath] == nil {
				paths[basePath] = make(map[string]interface{})
			}
			paths[basePath].(map[string]interface{})["get"] = dg.generateGetAllOperationSpec(config.GetAll, entityName)
		}

		// POST /entities (Create) avec requestBody
		if config.Create != nil {
			if paths[basePath] == nil {
				paths[basePath] = make(map[string]interface{})
			}
			paths[basePath].(map[string]interface{})["post"] = dg.generateCreateOperationSpec(config.Create, entityName)
		}

		// GET /entities/{id} (Get One)
		pathWithId := basePath + "/{id}"
		if config.GetOne != nil {
			if paths[pathWithId] == nil {
				paths[pathWithId] = make(map[string]interface{})
			}
			paths[pathWithId].(map[string]interface{})["get"] = dg.generateGetOneOperationSpec(config.GetOne, entityName)
		}

		// PATCH /entities/{id} (Update) avec requestBody
		if config.Update != nil {
			if paths[pathWithId] == nil {
				paths[pathWithId] = make(map[string]interface{})
			}
			paths[pathWithId].(map[string]interface{})["patch"] = dg.generateUpdateOperationSpec(config.Update, entityName)
		}

		// DELETE /entities/{id} (Delete)
		if config.Delete != nil {
			if paths[pathWithId] == nil {
				paths[pathWithId] = make(map[string]interface{})
			}
			paths[pathWithId].(map[string]interface{})["delete"] = dg.generateDeleteOperationSpec(config.Delete, entityName)
		}
	}

	return spec
}

// Générer les schémas à partir des DTOs enregistrés
func (dg *DocumentationGenerator) generateSchemas(configs map[string]*entityManagementInterfaces.EntitySwaggerConfig) map[string]interface{} {
	schemas := make(map[string]interface{})

	for entityName := range configs {
		log.Printf("🧩 Generating schemas for entity: %s", entityName)

		// Récupérer les DTOs depuis le système d'enregistrement
		inputCreateDto := ems.GlobalEntityRegistrationService.GetEntityDtos(entityName, ems.InputCreateDto)
		outputDto := ems.GlobalEntityRegistrationService.GetEntityDtos(entityName, ems.OutputDto)
		inputEditDto := ems.GlobalEntityRegistrationService.GetEntityDtos(entityName, ems.InputEditDto)

		// Générer les schémas à partir des types Go
		if inputCreateDto != nil {
			schemaName := entityName + "CreateInput"
			schemas[schemaName] = dg.generateSchemaFromStruct(inputCreateDto)
			log.Printf("  📝 Generated schema: %s", schemaName)
		}

		if outputDto != nil {
			schemaName := entityName + "Output"
			schemas[schemaName] = dg.generateSchemaFromStruct(outputDto)
			log.Printf("  📝 Generated schema: %s", schemaName)
		}

		if inputEditDto != nil {
			schemaName := entityName + "UpdateInput"
			schemas[schemaName] = dg.generateSchemaFromStruct(inputEditDto)
			log.Printf("  📝 Generated schema: %s", schemaName)
		}
	}

	log.Printf("🧩 Total schemas generated: %d", len(schemas))
	return schemas
}

// Générer un schéma OpenAPI à partir d'une structure Go via réflexion
func (dg *DocumentationGenerator) generateSchemaFromStruct(structInstance interface{}) map[string]interface{} {
	// Utiliser la réflexion pour analyser la structure
	t := reflect.TypeOf(structInstance)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	properties := make(map[string]interface{})
	required := []string{}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Ignorer les champs non exportés
		if !field.IsExported() {
			continue
		}

		jsonTag := field.Tag.Get("json")
		bindingTag := field.Tag.Get("binding")

		// Ignorer les champs avec json:"-"
		if jsonTag == "-" {
			continue
		}

		fieldName := field.Name
		if jsonTag != "" {
			// Extraire le nom du tag json (avant la virgule)
			if parts := strings.Split(jsonTag, ","); len(parts) > 0 && parts[0] != "" {
				fieldName = parts[0]
			}
		}

		// Déterminer le type OpenAPI
		fieldType := dg.getOpenAPIType(field.Type)

		// Ajouter une description si disponible via le tag
		if description := field.Tag.Get("description"); description != "" {
			fieldType["description"] = description
		}

		properties[fieldName] = fieldType

		// Vérifier si le champ est requis via le tag binding
		if strings.Contains(bindingTag, "required") {
			required = append(required, fieldName)
		}
	}

	schema := map[string]interface{}{
		"type":       "object",
		"properties": properties,
	}

	if len(required) > 0 {
		schema["required"] = required
	}

	return schema
}

// Convertir un type Go vers un type OpenAPI
func (dg *DocumentationGenerator) getOpenAPIType(t reflect.Type) map[string]interface{} {
	switch t.Kind() {
	case reflect.String:
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
		elemType := dg.getOpenAPIType(t.Elem())
		return map[string]interface{}{
			"type":  "array",
			"items": elemType,
		}
	case reflect.Ptr:
		// Pour les pointeurs, analyser le type sous-jacent
		return dg.getOpenAPIType(t.Elem())
	case reflect.Struct:
		// Pour les types comme time.Time, uuid.UUID, etc.
		switch t.String() {
		case "time.Time":
			return map[string]interface{}{
				"type":   "string",
				"format": "date-time",
			}
		case "uuid.UUID":
			return map[string]interface{}{
				"type":   "string",
				"format": "uuid",
			}
		default:
			return map[string]interface{}{"type": "object"}
		}
	default:
		return map[string]interface{}{"type": "string"}
	}
}

// Opérations spécialisées avec requestBody et réponses complètes

func (dg *DocumentationGenerator) generateCreateOperationSpec(operation *entityManagementInterfaces.SwaggerOperation, entityName string) map[string]interface{} {
	spec := dg.generateBaseOperationSpec(operation)

	// Ajouter le requestBody pour POST
	spec["requestBody"] = map[string]interface{}{
		"required": true,
		"content": map[string]interface{}{
			"application/json": map[string]interface{}{
				"schema": map[string]interface{}{
					"$ref": fmt.Sprintf("#/components/schemas/%sCreateInput", entityName),
				},
			},
		},
	}

	// Ajouter les réponses détaillées
	spec["responses"] = map[string]interface{}{
		"201": map[string]interface{}{
			"description": fmt.Sprintf("%s created successfully", entityName),
			"content": map[string]interface{}{
				"application/json": map[string]interface{}{
					"schema": map[string]interface{}{
						"$ref": fmt.Sprintf("#/components/schemas/%sOutput", entityName),
					},
				},
			},
		},
		"400": dg.generateErrorResponse("Bad request"),
		"401": dg.generateErrorResponse("Unauthorized"),
		"403": dg.generateErrorResponse("Forbidden"),
	}

	return spec
}

func (dg *DocumentationGenerator) generateUpdateOperationSpec(operation *entityManagementInterfaces.SwaggerOperation, entityName string) map[string]interface{} {
	spec := dg.generateBaseOperationSpec(operation)

	// Ajouter le paramètre id
	spec["parameters"] = []map[string]interface{}{
		{
			"name":        "id",
			"in":          "path",
			"required":    true,
			"description": fmt.Sprintf("%s ID", entityName),
			"schema": map[string]interface{}{
				"type":   "string",
				"format": "uuid",
			},
		},
	}

	// Ajouter le requestBody pour PATCH
	spec["requestBody"] = map[string]interface{}{
		"required": true,
		"content": map[string]interface{}{
			"application/json": map[string]interface{}{
				"schema": map[string]interface{}{
					"$ref": fmt.Sprintf("#/components/schemas/%sUpdateInput", entityName),
				},
			},
		},
	}

	spec["responses"] = map[string]interface{}{
		"204": map[string]interface{}{
			"description": fmt.Sprintf("%s updated successfully", entityName),
		},
		"400": dg.generateErrorResponse("Bad request"),
		"401": dg.generateErrorResponse("Unauthorized"),
		"403": dg.generateErrorResponse("Forbidden"),
		"404": dg.generateErrorResponse("Not found"),
	}

	return spec
}

func (dg *DocumentationGenerator) generateGetAllOperationSpec(operation *entityManagementInterfaces.SwaggerOperation, entityName string) map[string]interface{} {
	spec := dg.generateBaseOperationSpec(operation)

	spec["responses"] = map[string]interface{}{
		"200": map[string]interface{}{
			"description": fmt.Sprintf("List of %s", strings.ToLower(ems.Pluralize(entityName))),
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
		"401": dg.generateErrorResponse("Unauthorized"),
		"403": dg.generateErrorResponse("Forbidden"),
	}

	return spec
}

func (dg *DocumentationGenerator) generateGetOneOperationSpec(operation *entityManagementInterfaces.SwaggerOperation, entityName string) map[string]interface{} {
	spec := dg.generateBaseOperationSpec(operation)

	spec["parameters"] = []map[string]interface{}{
		{
			"name":        "id",
			"in":          "path",
			"required":    true,
			"description": fmt.Sprintf("%s ID", entityName),
			"schema": map[string]interface{}{
				"type":   "string",
				"format": "uuid",
			},
		},
	}

	spec["responses"] = map[string]interface{}{
		"200": map[string]interface{}{
			"description": fmt.Sprintf("%s details", entityName),
			"content": map[string]interface{}{
				"application/json": map[string]interface{}{
					"schema": map[string]interface{}{
						"$ref": fmt.Sprintf("#/components/schemas/%sOutput", entityName),
					},
				},
			},
		},
		"401": dg.generateErrorResponse("Unauthorized"),
		"403": dg.generateErrorResponse("Forbidden"),
		"404": dg.generateErrorResponse("Not found"),
	}

	return spec
}

func (dg *DocumentationGenerator) generateDeleteOperationSpec(operation *entityManagementInterfaces.SwaggerOperation, entityName string) map[string]interface{} {
	spec := dg.generateBaseOperationSpec(operation)

	spec["parameters"] = []map[string]interface{}{
		{
			"name":        "id",
			"in":          "path",
			"required":    true,
			"description": fmt.Sprintf("%s ID", entityName),
			"schema": map[string]interface{}{
				"type":   "string",
				"format": "uuid",
			},
		},
	}

	spec["responses"] = map[string]interface{}{
		"204": map[string]interface{}{
			"description": fmt.Sprintf("%s deleted successfully", entityName),
		},
		"401": dg.generateErrorResponse("Unauthorized"),
		"403": dg.generateErrorResponse("Forbidden"),
		"404": dg.generateErrorResponse("Not found"),
	}

	return spec
}

func (dg *DocumentationGenerator) generateBaseOperationSpec(operation *entityManagementInterfaces.SwaggerOperation) map[string]interface{} {
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

// Génération standardisée des réponses d'erreur
func (dg *DocumentationGenerator) generateErrorResponse(description string) map[string]interface{} {
	return map[string]interface{}{
		"description": description,
		"content": map[string]interface{}{
			"application/json": map[string]interface{}{
				"schema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"error_code": map[string]interface{}{
							"type": "integer",
						},
						"error_message": map[string]interface{}{
							"type": "string",
						},
					},
				},
			},
		},
	}
}
