// src/entityManagement/swagger/additionalSchemas.go
package swagger

import (
	"reflect"
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/errors"
	"strings"
)

// AdditionalSchemasService gère les schémas qui ne font pas partie du système d'entités
type AdditionalSchemasService struct {
	schemas map[string]interface{}
}

func NewAdditionalSchemasService() *AdditionalSchemasService {
	service := &AdditionalSchemasService{
		schemas: make(map[string]interface{}),
	}

	// Enregistrer les schémas par défaut
	service.registerDefaultSchemas()

	return service
}

// registerDefaultSchemas enregistre tous les schémas nécessaires qui ne sont pas dans les entités
func (ass *AdditionalSchemasService) registerDefaultSchemas() {
	// Schémas d'authentification
	ass.RegisterSchema("dto.LoginInput", dto.LoginInput{})
	ass.RegisterSchema("dto.LoginOutput", dto.LoginOutput{})

	// Schémas d'erreurs
	ass.RegisterSchema("errors.APIError", errors.APIError{})

	// Schémas de groupes
	// ass.RegisterSchema("dto.CreateGroupInput", dto.CreateGroupInput{})
	// ass.RegisterSchema("dto.CreateGroupOutput", dto.CreateGroupOutput{})
	// ass.RegisterSchema("dto.ModifyUsersInGroupInput", dto.ModifyUsersInGroupInput{})

	// // Schémas d'utilisateurs
	// ass.RegisterSchema("dto.CreateUserInput", dto.CreateUserInput{})
	// ass.RegisterSchema("dto.CreateUserOutput", dto.CreateUserOutput{})
	// ass.RegisterSchema("dto.UserOutput", dto.UserOutput{})
	// ass.RegisterSchema("dto.DeleteUserInput", dto.DeleteUserInput{})

	// // Schémas d'accès
	// ass.RegisterSchema("dto.CreateEntityAccessInput", dto.CreateEntityAccessInput{})
	// ass.RegisterSchema("dto.DeleteEntityAccessInput", dto.DeleteEntityAccessInput{})

	// Schémas SSH Keys
	// ass.RegisterSchema("dto.SshkeyOutput", dto.SshkeyOutput{})
	// ass.RegisterSchema("dto.CreateSshkeyInput", dto.CreateSshkeyInput{})
	// ass.RegisterSchema("dto.CreateSshkeyOutput", dto.CreateSshkeyOutput{})
	// ass.RegisterSchema("dto.EditSshkeyInput", dto.EditSshkeyInput{})
	// ass.RegisterSchema("dto.DeleteSshkeyInput", dto.DeleteSshkeyInput{})

	// Schémas génériques pour les réponses
	ass.addGenericResponseSchemas()
}

// addGenericResponseSchemas ajoute des schémas de réponse génériques
func (ass *AdditionalSchemasService) addGenericResponseSchemas() {
	// Schéma de réponse générique pour les succès
	ass.schemas["GenericSuccessResponse"] = map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"message": map[string]interface{}{
				"type":        "string",
				"description": "Message de succès",
				"example":     "Operation completed successfully",
			},
			"data": map[string]interface{}{
				"type":        "object",
				"description": "Données de la réponse",
			},
		},
		"required": []string{"message"},
	}

	// Schéma pour les réponses de suppression
	ass.schemas["DeleteResponse"] = map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"message": map[string]interface{}{
				"type":    "string",
				"example": "Entity deleted successfully",
			},
		},
	}

	// Schéma pour les listes paginées
	ass.schemas["PaginatedResponse"] = map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"data": map[string]interface{}{
				"type":        "array",
				"description": "Liste des éléments",
				"items": map[string]interface{}{
					"type": "object",
				},
			},
			"pagination": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"page": map[string]interface{}{
						"type":    "integer",
						"example": 1,
					},
					"limit": map[string]interface{}{
						"type":    "integer",
						"example": 20,
					},
					"total": map[string]interface{}{
						"type":    "integer",
						"example": 100,
					},
				},
			},
		},
		"required": []string{"data"},
	}

	// Schéma pour les paramètres de pagination
	ass.schemas["PaginationParams"] = map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"page": map[string]interface{}{
				"type":        "integer",
				"minimum":     1,
				"default":     1,
				"description": "Numéro de page",
			},
			"limit": map[string]interface{}{
				"type":        "integer",
				"minimum":     1,
				"maximum":     100,
				"default":     20,
				"description": "Nombre d'éléments par page",
			},
		},
	}
}

// RegisterSchema enregistre un schéma à partir d'un struct Go
func (ass *AdditionalSchemasService) RegisterSchema(name string, structInstance interface{}) {
	schema := ass.generateSchemaFromStruct(structInstance)
	ass.schemas[name] = schema
}

// RegisterCustomSchema enregistre un schéma personnalisé
func (ass *AdditionalSchemasService) RegisterCustomSchema(name string, schema map[string]interface{}) {
	ass.schemas[name] = schema
}

// GetAllSchemas retourne tous les schémas enregistrés
func (ass *AdditionalSchemasService) GetAllSchemas() map[string]interface{} {
	result := make(map[string]interface{})
	for name, schema := range ass.schemas {
		result[name] = schema
	}
	return result
}

// GetSchema retourne un schéma spécifique
func (ass *AdditionalSchemasService) GetSchema(name string) (map[string]interface{}, bool) {
	schema, exists := ass.schemas[name]
	if !exists {
		return nil, false
	}

	if schemaMap, ok := schema.(map[string]interface{}); ok {
		return schemaMap, true
	}

	return nil, false
}

// generateSchemaFromStruct génère un schéma OpenAPI à partir d'un struct Go
func (ass *AdditionalSchemasService) generateSchemaFromStruct(structInstance interface{}) map[string]interface{} {
	schema := map[string]interface{}{
		"type":       "object",
		"properties": make(map[string]interface{}),
	}

	structType := reflect.TypeOf(structInstance)
	if structType.Kind() == reflect.Ptr {
		structType = structType.Elem()
	}

	properties := schema["properties"].(map[string]interface{})
	var required []string

	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)

		// Ignorer les champs non exportés
		if !field.IsExported() {
			continue
		}

		// Récupérer le nom JSON
		jsonTag := field.Tag.Get("json")
		fieldName := field.Name
		if jsonTag != "" && jsonTag != "-" {
			if strings.Contains(jsonTag, ",") {
				parts := strings.Split(jsonTag, ",")
				fieldName = parts[0]

				// Vérifier si le champ est omitempty
				if len(parts) > 1 && strings.Contains(parts[1], "omitempty") {
					// Ne pas ajouter aux required si omitempty
				} else if !strings.Contains(parts[1], "omitempty") {
					// Ajouter aux required si pas omitempty et pas de binding
					bindingTag := field.Tag.Get("binding")
					if bindingTag == "" {
						// Pas de binding tag, utiliser la présence/absence d'omitempty
					}
				}
			} else {
				fieldName = jsonTag
			}
		} else {
			// Convertir en snake_case
			fieldName = strings.ToLower(fieldName)
		}

		// Vérifier si le champ est requis via le tag binding
		bindingTag := field.Tag.Get("binding")
		if strings.Contains(bindingTag, "required") {
			required = append(required, fieldName)
		}

		// Générer le type du champ
		fieldSchema := ass.getSwaggerTypeFromGoType(field.Type)

		// Ajouter la description si disponible
		if description := field.Tag.Get("description"); description != "" {
			fieldSchema["description"] = description
		}

		// Ajouter l'exemple si disponible
		if example := field.Tag.Get("example"); example != "" {
			fieldSchema["example"] = example
		}

		// Ajouter des validations depuis les tags
		if bindingTag != "" {
			ass.addValidationFromBinding(fieldSchema, bindingTag)
		}

		properties[fieldName] = fieldSchema
	}

	if len(required) > 0 {
		schema["required"] = required
	}

	return schema
}

// addValidationFromBinding ajoute des validations basées sur le tag binding
func (ass *AdditionalSchemasService) addValidationFromBinding(fieldSchema map[string]interface{}, bindingTag string) {
	// Parser les validations du tag binding
	validations := strings.Split(bindingTag, ",")

	for _, validation := range validations {
		validation = strings.TrimSpace(validation)

		if strings.HasPrefix(validation, "min=") {
			// Minimum value
			if fieldSchema["type"] == "integer" || fieldSchema["type"] == "number" {
				fieldSchema["minimum"] = validation[4:]
			} else if fieldSchema["type"] == "string" {
				fieldSchema["minLength"] = validation[4:]
			}
		} else if strings.HasPrefix(validation, "max=") {
			// Maximum value
			if fieldSchema["type"] == "integer" || fieldSchema["type"] == "number" {
				fieldSchema["maximum"] = validation[4:]
			} else if fieldSchema["type"] == "string" {
				fieldSchema["maxLength"] = validation[4:]
			}
		} else if validation == "email" {
			// Email format
			fieldSchema["format"] = "email"
		} else if validation == "url" {
			// URL format
			fieldSchema["format"] = "uri"
		} else if validation == "uuid" {
			// UUID format
			fieldSchema["format"] = "uuid"
		}
	}
}

// getSwaggerTypeFromGoType convertit un type Go vers un type Swagger/OpenAPI
func (ass *AdditionalSchemasService) getSwaggerTypeFromGoType(goType reflect.Type) map[string]interface{} {
	// Gérer les pointeurs
	if goType.Kind() == reflect.Ptr {
		goType = goType.Elem()
	}

	switch goType.Kind() {
	case reflect.String:
		schema := map[string]interface{}{
			"type": "string",
		}

		// Cas spéciaux pour les types nommés
		typeName := goType.String()
		if strings.Contains(typeName, "uuid.UUID") || goType.Name() == "UUID" {
			schema["format"] = "uuid"
			schema["example"] = "123e4567-e89b-12d3-a456-426614174000"
		}

		return schema

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32:
		return map[string]interface{}{
			"type":    "integer",
			"format":  "int32",
			"example": 42,
		}

	case reflect.Int64:
		return map[string]interface{}{
			"type":    "integer",
			"format":  "int64",
			"example": 1234567890,
		}

	case reflect.Float32:
		return map[string]interface{}{
			"type":    "number",
			"format":  "float",
			"example": 3.14,
		}

	case reflect.Float64:
		return map[string]interface{}{
			"type":    "number",
			"format":  "double",
			"example": 3.141592653589793,
		}

	case reflect.Bool:
		return map[string]interface{}{
			"type":    "boolean",
			"example": true,
		}

	case reflect.Slice:
		return map[string]interface{}{
			"type":  "array",
			"items": ass.getSwaggerTypeFromGoType(goType.Elem()),
		}

	case reflect.Map:
		return map[string]interface{}{
			"type":                 "object",
			"additionalProperties": ass.getSwaggerTypeFromGoType(goType.Elem()),
		}

	case reflect.Struct:
		typeName := goType.String()

		if typeName == "time.Time" {
			return map[string]interface{}{
				"type":    "string",
				"format":  "date-time",
				"example": "2023-12-07T10:30:00Z",
			}
		}

		// Pour les structs personnalisés, essayer de deviner le type
		if strings.Contains(typeName, "Action") {
			return map[string]interface{}{
				"type":        "integer",
				"enum":        []int{0, 1},
				"example":     0,
				"description": "0: ADD, 1: REMOVE",
			}
		}

		// Struct générique
		return map[string]interface{}{
			"type":        "object",
			"description": "Complex object of type " + typeName,
		}

	case reflect.Interface:
		return map[string]interface{}{
			"type":        "object",
			"description": "Generic interface type",
		}

	default:
		return map[string]interface{}{
			"type":        "string",
			"description": "Unknown type: " + goType.String(),
		}
	}
}

// GetSchemaCount retourne le nombre de schémas enregistrés
func (ass *AdditionalSchemasService) GetSchemaCount() int {
	return len(ass.schemas)
}

// ListSchemaNames retourne la liste des noms de schémas
func (ass *AdditionalSchemasService) ListSchemaNames() []string {
	names := make([]string, 0, len(ass.schemas))
	for name := range ass.schemas {
		names = append(names, name)
	}
	return names
}

// Instance globale du service (optionnelle, pour usage simple)
var GlobalAdditionalSchemasService = NewAdditionalSchemasService()
