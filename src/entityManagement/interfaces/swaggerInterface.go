// src/entityManagement/interfaces/swaggerInterface.go
package entityManagementInterfaces

// SwaggerOperation définit les métadonnées pour une opération CRUD
type SwaggerOperation struct {
	Summary     string   `json:"summary"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Security    bool     `json:"security"` // Si l'opération nécessite une authentification
}

// EntitySwaggerConfig configure la documentation Swagger pour une entité
type EntitySwaggerConfig struct {
	Tag        string            `json:"tag"`         // Tag principal pour grouper les opérations
	EntityName string            `json:"entity_name"` // Nom de l'entité (au singulier)
	GetAll     *SwaggerOperation `json:"get_all,omitempty"`
	GetOne     *SwaggerOperation `json:"get_one,omitempty"`
	Create     *SwaggerOperation `json:"create,omitempty"`
	Update     *SwaggerOperation `json:"update,omitempty"`
	Delete     *SwaggerOperation `json:"delete,omitempty"`
}

// SwaggerDocumentedEntity - Interface optionnelle pour les entités qui veulent de la documentation
type SwaggerDocumentedEntity interface {
	GetSwaggerConfig() EntitySwaggerConfig
}

// Helper function pour créer rapidement une opération standard
func NewSwaggerOperation(summary, description string, tags []string, requiresAuth bool) *SwaggerOperation {
	return &SwaggerOperation{
		Summary:     summary,
		Description: description,
		Tags:        tags,
		Security:    requiresAuth,
	}
}

// Helper function pour créer une config Swagger complète rapidement
func NewEntitySwaggerConfig(entityName, tag string) EntitySwaggerConfig {
	tags := []string{tag}

	return EntitySwaggerConfig{
		Tag:        tag,
		EntityName: entityName,
		GetAll: NewSwaggerOperation(
			"Récupérer tous les "+entityName+"s",
			"Retourne la liste de tous les "+entityName+"s disponibles",
			tags,
			true,
		),
		GetOne: NewSwaggerOperation(
			"Récupérer un "+entityName,
			"Retourne les détails d'un "+entityName+" spécifique",
			tags,
			true,
		),
		Create: NewSwaggerOperation(
			"Créer un "+entityName,
			"Crée un nouveau "+entityName,
			tags,
			true,
		),
		Update: NewSwaggerOperation(
			"Mettre à jour un "+entityName,
			"Modifie un "+entityName+" existant",
			tags,
			true,
		),
		Delete: NewSwaggerOperation(
			"Supprimer un "+entityName,
			"Supprime un "+entityName+" existant",
			tags,
			true,
		),
	}
}
