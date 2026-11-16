package initialization

import (
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"gorm.io/gorm"

	"soli/formations/docs"
	authController "soli/formations/src/auth"
	"soli/formations/src/auth/access"
	swaggerGenerator "soli/formations/src/entityManagement/swagger"
)

// InitSwagger initializes Swagger documentation
func InitSwagger(r *gin.Engine, db *gorm.DB) {
	env := os.Getenv("ENVIRONMENT")
	if env == "development" || env == "test" {
		docs.SwaggerInfo.Schemes = []string{"http", "https"}
	} else {
		docs.SwaggerInfo.Schemes = []string{"https"}
	}

	docs.SwaggerInfo.Title = "OCF API"
	docs.SwaggerInfo.Description = "This is an API to build and generate courses with labs"
	docs.SwaggerInfo.Version = getVersionFromFile()

	// Setup de la documentation complète (manual + auto-generated)
	SetupCompleteSwaggerSystem(r, db)
}

func getVersionFromFile() string {
	if data, err := os.ReadFile("VERSION"); err == nil {
		return strings.TrimSpace(string(data))
	}
	if version := os.Getenv("OCF_VERSION"); version != "" {
		return version
	}
	return "unknown"
}

// SetupCompleteSwaggerSystem sets up the complete Swagger documentation system
func SetupCompleteSwaggerSystem(r *gin.Engine, db *gorm.DB) {
	log.Println("🚀 Setting up complete Swagger documentation system...")

	// Middleware d'authentification pour les routes documentées
	authMiddleware := authController.NewAuthMiddleware(db)

	// 📋 ÉTAPE 1: Setup des routes auto-documentées
	log.Println("  📋 Setting up auto-documented routes...")
	routeGenerator := swaggerGenerator.NewSwaggerRouteGenerator(db)
	docGroup := r.Group("/api/v1")
	// Layer 2 enforcement: pass-through for generated CRUD routes (not registered
	// by route in the RouteRegistry) but REQUIRED so custom action Access rules are
	// actually enforced. Without it this group skips Layer 2 entirely.
	docGroup.Use(access.Layer2Enforcement())
	routeGenerator.RegisterDocumentedRoutes(docGroup, authMiddleware.AuthManagement(), authMiddleware.IdentifyIfPresent())

	// 🔀 ÉTAPE 2: Setup du merger Swagger
	log.Println("  🔀 Setting up Swagger spec merger...")
	merger := swaggerGenerator.NewSwaggerSpecMerger()

	// 📄 ÉTAPE 3: Endpoints de documentation
	setupSwaggerEndpoints(r, merger)

	log.Println("✅ Complete Swagger system ready!")
	log.Println("📚 Available endpoints:")
	log.Println("  🎨 /swagger/ - Complete documentation (manual + auto)")
	log.Println("  📋 /api/v1/swagger/spec - Merged OpenAPI spec")
	log.Println("  📄 /swagger/index.html - Original swag documentation")
}

func setupSwaggerEndpoints(r *gin.Engine, merger *swaggerGenerator.SwaggerSpecMerger) {
	// Endpoint principal : spec mergée
	r.GET("/api/v1/swagger/spec", func(ctx *gin.Context) {
		mergedSpec := merger.MergeSpecs()

		// Headers CORS pour Swagger UI
		ctx.Header("Access-Control-Allow-Origin", "*")
		ctx.Header("Access-Control-Allow-Methods", "GET")
		ctx.Header("Access-Control-Allow-Headers", "Content-Type")

		ctx.JSON(http.StatusOK, mergedSpec)
	})

	// Interface Swagger UI personnalisée (documentation complète)
	r.GET("/swagger/", func(ctx *gin.Context) {
		ctx.Header("Content-Type", "text/html")
		ctx.String(http.StatusOK, generateCustomSwaggerHTML())
	})

	// Garder l'endpoint original pour compatibilité
	r.GET("/swagger/previous", ginSwagger.WrapHandler(swaggerFiles.Handler))
}

// generateCustomSwaggerHTML génère une page HTML Swagger UI personnalisée
func generateCustomSwaggerHTML() string {
	return `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>OCF API Documentation - Complete</title>
    <link rel="stylesheet" type="text/css" href="https://unpkg.com/swagger-ui-dist@4.15.5/swagger-ui.css" />
    <style>
        html { box-sizing: border-box; overflow: -moz-scrollbars-vertical; overflow-y: scroll; }
        *, *:before, *:after { box-sizing: inherit; }
        body { margin:0; background: #fafafa; }

        .header-banner {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            padding: 15px;
            text-align: center;
            margin-bottom: 20px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }

        .header-banner h1 {
            margin: 0;
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
        }

        .header-banner p {
            margin: 5px 0 0 0;
            opacity: 0.9;
            font-size: 14px;
        }

        .status-badge {
            display: inline-block;
            background: #28a745;
            color: white;
            padding: 4px 8px;
            border-radius: 12px;
            font-size: 12px;
            margin-left: 10px;
        }

        /* Style pour les entités auto-générées */
        .swagger-ui .opblock.opblock-get .opblock-summary-control,
        .swagger-ui .opblock.opblock-post .opblock-summary-control,
        .swagger-ui .opblock.opblock-patch .opblock-summary-control,
        .swagger-ui .opblock.opblock-delete .opblock-summary-control {
            position: relative;
        }
    </style>
</head>
<body>
    <div class="header-banner">
        <h1>🚀 OCF API Documentation</h1>
        <p>Documentation complète : Endpoints manuels + Entités auto-générées <span class="status-badge">🤖 Hybrid</span></p>
    </div>

    <div id="swagger-ui"></div>

    <script src="https://unpkg.com/swagger-ui-dist@4.15.5/swagger-ui-bundle.js"></script>
    <script src="https://unpkg.com/swagger-ui-dist@4.15.5/swagger-ui-standalone-preset.js"></script>
    <script>
        window.onload = function() {
            const ui = SwaggerUIBundle({
                url: window.location.origin + '/api/v1/swagger/spec',
                dom_id: '#swagger-ui',
                deepLinking: true,
                presets: [
                    SwaggerUIBundle.presets.apis,
                    SwaggerUIStandalonePreset
                ],
                plugins: [
                    SwaggerUIBundle.plugins.DownloadUrl
                ],
                layout: "StandaloneLayout",
                defaultModelsExpandDepth: 1,
                defaultModelExpandDepth: 1,
                docExpansion: "list",
                tagsSorter: "alpha",
                operationsSorter: "alpha",
                filter: true,
                validatorUrl: null,
                tryItOutEnabled: true,
                onComplete: function() {
                    console.log('📚 OCF API Documentation chargée');
                    console.log('🔀 Documentation hybride : manuelle + auto-générée');

										// 🔍 DEBUG : Vérifier les serveurs configurés
                    const spec = ui.getSystem().specSelectors.spec().toJS();
                    console.log('🔍 Servers in spec:', spec.servers);

                    // Ajouter un indicateur de statut dans le header
                    setTimeout(() => {
                        const infoSection = document.querySelector('.information-container');
                        if (infoSection) {
                            const statusDiv = document.createElement('div');
                            statusDiv.style.cssText = 'background: #e8f5e8; padding: 10px; border-radius: 5px; margin: 10px 0; border-left: 4px solid #28a745;';
                            statusDiv.innerHTML = '<strong>🔄 Documentation dynamique :</strong> Cette documentation est générée automatiquement et reste toujours synchronisée avec le code.';
                            infoSection.appendChild(statusDiv);
                        }
                    }, 1000);
                }
            });
        };
    </script>
</body>
</html>`
}
