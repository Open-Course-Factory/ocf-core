package services

import (
	"log"
	"reflect"
	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/interfaces"
	authModels "soli/formations/src/auth/models"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"strings"

	"github.com/gertd/go-pluralize"
)

type ConversionPurpose int

const (
	CreateInputDtoToModel ConversionPurpose = iota
	OutputModelToDto
	EditInputDtoToMap
)

type DtoPurpose int

const (
	InputCreateDto DtoPurpose = iota
	InputEditDto
	OutputDto
)

type EntityRegistrationService struct {
	registry            map[string]any
	functions           map[string]map[ConversionPurpose]any
	dtos                map[string]map[DtoPurpose]any
	subEntities         map[string][]any
	swaggerConfigs      map[string]*entityManagementInterfaces.EntitySwaggerConfig
	relationshipFilters map[string][]entityManagementInterfaces.RelationshipFilter
}

func NewEntityRegistrationService() *EntityRegistrationService {
	return &EntityRegistrationService{
		registry:            make(map[string]any),
		functions:           make(map[string]map[ConversionPurpose]any),
		dtos:                make(map[string]map[DtoPurpose]any),
		subEntities:         make(map[string][]any),
		swaggerConfigs:      make(map[string]*entityManagementInterfaces.EntitySwaggerConfig),
		relationshipFilters: make(map[string][]entityManagementInterfaces.RelationshipFilter),
	}
}

func (s *EntityRegistrationService) RegisterEntityInterface(name string, entityType any) {
	s.registry[name] = entityType
}

func (s *EntityRegistrationService) RegisterSubEntites(name string, subEntities []any) {
	s.subEntities[name] = subEntities
}

func (s *EntityRegistrationService) RegisterEntityConversionFunctions(name string, converters entityManagementInterfaces.EntityConverters) {
	ways := make(map[ConversionPurpose]any)

	ways[OutputModelToDto] = converters.ModelToDto
	ways[CreateInputDtoToModel] = converters.DtoToModel
	ways[EditInputDtoToMap] = converters.DtoToMap

	s.functions[name] = ways
}

func (s *EntityRegistrationService) RegisterEntityDtos(name string, dtos map[DtoPurpose]any) {
	s.dtos[name] = dtos
}

func (s *EntityRegistrationService) RegisterSwaggerConfig(name string, config *entityManagementInterfaces.EntitySwaggerConfig) {
	s.swaggerConfigs[name] = config
	log.Printf("📚 Swagger config registered for entity: %s (tag: %s)", name, config.Tag)
}

func (s *EntityRegistrationService) GetSwaggerConfig(name string) *entityManagementInterfaces.EntitySwaggerConfig {
	return s.swaggerConfigs[name]
}

func (s *EntityRegistrationService) GetAllSwaggerConfigs() map[string]*entityManagementInterfaces.EntitySwaggerConfig {
	// Retourner une copie pour éviter les modifications externes
	result := make(map[string]*entityManagementInterfaces.EntitySwaggerConfig)
	for k, v := range s.swaggerConfigs {
		result[k] = v
	}
	return result
}

func (s *EntityRegistrationService) GetEntityInterface(name string) (any, bool) {
	entityType, exists := s.registry[name]
	return entityType, exists
}

func (s *EntityRegistrationService) GetEntityDtos(name string, way DtoPurpose) any {
	return s.dtos[name][way]
}

func (s *EntityRegistrationService) GetConversionFunction(name string, way ConversionPurpose) (any, bool) {
	var function any
	var exists bool
	switch way {
	case OutputModelToDto:
		function, exists = s.functions[name][OutputModelToDto]
	case CreateInputDtoToModel:
		function, exists = s.functions[name][CreateInputDtoToModel]
	case EditInputDtoToMap:
		function, exists = s.functions[name][EditInputDtoToMap]
	default:
		function = nil
		exists = false
	}

	return function, exists
}

func (s *EntityRegistrationService) GetSubEntites(entityName string) []any {
	return s.subEntities[entityName]
}

func (s *EntityRegistrationService) RegisterRelationshipFilters(name string, filters []entityManagementInterfaces.RelationshipFilter) {
	s.relationshipFilters[name] = filters
}

func (s *EntityRegistrationService) GetRelationshipFilters(name string) []entityManagementInterfaces.RelationshipFilter {
	return s.relationshipFilters[name]
}

// SetDefaultEntityAccesses est une version publique pour les tests qui accepte un enforcer
func (s *EntityRegistrationService) SetDefaultEntityAccesses(entityName string, roles entityManagementInterfaces.EntityRoles, enforcer interfaces.EnforcerInterface) {
	s.setDefaultEntityAccesses(entityName, roles, enforcer)
}

func (s *EntityRegistrationService) setDefaultEntityAccesses(entityName string, roles entityManagementInterfaces.EntityRoles, enforcer interfaces.EnforcerInterface) {
	if enforcer == nil {
		log.Println("Enforcer is nil, skipping access setup")
		return
	}

	errLoadingPolicy := enforcer.LoadPolicy()
	if errLoadingPolicy != nil {
		log.Fatal(errLoadingPolicy.Error())
	}
	rolesMap := roles.Roles

	resourceName := Pluralize(entityName)
	resourceName = strings.ToLower(resourceName)
	apiGroupPath := "/api/v1/" + resourceName + "/*" // Use wildcard for entire API group

	log.Printf("Setting up entity access for %s at %s", entityName, apiGroupPath)

	for ocfRoleName, accessGiven := range rolesMap {
		// Add permission for the OCF role
		_, errPolicy := enforcer.AddPolicy(ocfRoleName, apiGroupPath, accessGiven)
		if errPolicy != nil {
			if strings.Contains(errPolicy.Error(), "UNIQUE") {
				log.Printf("OCF role permission already exists: %s %s %s", ocfRoleName, apiGroupPath, accessGiven)
			} else {
				log.Printf("Error adding OCF role permission: %v", errPolicy)
			}
		} else {
			log.Printf("Added OCF role permission: %s can %s %s", ocfRoleName, accessGiven, apiGroupPath)
		}

		// Automatically add permissions for corresponding Casdoor roles
		casdoorRoles := authModels.GetCasdoorRolesForOCFRole(authModels.RoleName(ocfRoleName))
		for _, casdoorRole := range casdoorRoles {
			_, errCasdoorPolicy := enforcer.AddPolicy(casdoorRole, apiGroupPath, accessGiven)
			if errCasdoorPolicy != nil {
				if strings.Contains(errCasdoorPolicy.Error(), "UNIQUE") {
					log.Printf("Casdoor role permission already exists: %s %s %s", casdoorRole, apiGroupPath, accessGiven)
				} else {
					log.Printf("Error adding Casdoor role permission: %v", errCasdoorPolicy)
				}
			} else {
				log.Printf("Added Casdoor role permission: %s can %s %s", casdoorRole, accessGiven, apiGroupPath)
			}
		}
	}

	log.Printf("Completed entity access setup for %s", entityName)
}

func Pluralize(entityName string) string {
	client := pluralize.NewClient()
	plural := client.Plural(entityName)
	return plural
}

func (s *EntityRegistrationService) RegisterEntity(input entityManagementInterfaces.RegistrableInterface) {
	entityToRegister := input.GetEntityRegistrationInput()
	entityName := reflect.TypeOf(entityToRegister.EntityInterface).Name()

	GlobalEntityRegistrationService.RegisterEntityInterface(entityName, entityToRegister.EntityInterface)
	GlobalEntityRegistrationService.RegisterEntityConversionFunctions(entityName, entityToRegister.EntityConverters)
	entityDtos := make(map[DtoPurpose]any)
	entityDtos[InputCreateDto] = entityToRegister.EntityDtos.InputCreateDto
	entityDtos[OutputDto] = entityToRegister.EntityDtos.OutputDto
	entityDtos[InputEditDto] = entityToRegister.EntityDtos.InputEditDto
	GlobalEntityRegistrationService.RegisterEntityDtos(entityName, entityDtos)
	GlobalEntityRegistrationService.RegisterSubEntites(entityName, entityToRegister.EntitySubEntities)
	GlobalEntityRegistrationService.RegisterRelationshipFilters(entityName, entityToRegister.RelationshipFilters)

	// Gestion automatique de la configuration Swagger
	if swaggerEntity, ok := input.(entityManagementInterfaces.SwaggerDocumentedEntity); ok {
		swaggerConfig := swaggerEntity.GetSwaggerConfig()
		// S'assurer que le nom de l'entité est défini
		if swaggerConfig.EntityName == "" {
			swaggerConfig.EntityName = entityName
		}
		GlobalEntityRegistrationService.RegisterSwaggerConfig(entityName, &swaggerConfig)
	} else {
		log.Printf("📝 Entity %s registered without Swagger documentation", entityName)
	}

	// Utiliser la variable globale casdoor.Enforcer en production
	s.setDefaultEntityAccesses(entityName, input.GetEntityRoles(), casdoor.Enforcer)
}

var GlobalEntityRegistrationService = NewEntityRegistrationService()
