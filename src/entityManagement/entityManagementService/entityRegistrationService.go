package services

import (
	"log"
	"reflect"
	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/interfaces"
	authModels "soli/formations/src/auth/models"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/entityManagement/utils"
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
	membershipConfigs   map[string]*entityManagementInterfaces.MembershipConfig // NEW: Generic membership configs
	defaultIncludes     map[string][]string                                     // NEW: Default relations to preload per entity
}

func NewEntityRegistrationService() *EntityRegistrationService {
	return &EntityRegistrationService{
		registry:            make(map[string]any),
		functions:           make(map[string]map[ConversionPurpose]any),
		dtos:                make(map[string]map[DtoPurpose]any),
		subEntities:         make(map[string][]any),
		swaggerConfigs:      make(map[string]*entityManagementInterfaces.EntitySwaggerConfig),
		relationshipFilters: make(map[string][]entityManagementInterfaces.RelationshipFilter),
		membershipConfigs:   make(map[string]*entityManagementInterfaces.MembershipConfig),
		defaultIncludes:     make(map[string][]string),
	}
}

// Reset clears all registered entities, functions, DTOs, and configurations
// This is primarily used for testing to ensure clean state between test runs
func (s *EntityRegistrationService) Reset() {
	s.registry = make(map[string]any)
	s.functions = make(map[string]map[ConversionPurpose]any)
	s.dtos = make(map[string]map[DtoPurpose]any)
	s.subEntities = make(map[string][]any)
	s.swaggerConfigs = make(map[string]*entityManagementInterfaces.EntitySwaggerConfig)
	s.relationshipFilters = make(map[string][]entityManagementInterfaces.RelationshipFilter)
	s.membershipConfigs = make(map[string]*entityManagementInterfaces.MembershipConfig)
	s.defaultIncludes = make(map[string][]string)
}

// UnregisterEntity removes all registrations for a specific entity
// This is primarily used for testing to clean up after individual tests
func (s *EntityRegistrationService) UnregisterEntity(name string) {
	delete(s.registry, name)
	delete(s.functions, name)
	delete(s.dtos, name)
	delete(s.subEntities, name)
	delete(s.swaggerConfigs, name)
	delete(s.relationshipFilters, name)
	delete(s.membershipConfigs, name)
	delete(s.defaultIncludes, name)
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

// RegisterMembershipConfig registers a membership configuration for an entity
func (s *EntityRegistrationService) RegisterMembershipConfig(name string, config *entityManagementInterfaces.MembershipConfig) {
	if config != nil {
		s.membershipConfigs[name] = config
		log.Printf("🔐 Membership config registered for entity: %s (table: %s)", name, config.MemberTable)
	}
}

// GetMembershipConfig retrieves the membership configuration for an entity
func (s *EntityRegistrationService) GetMembershipConfig(name string) *entityManagementInterfaces.MembershipConfig {
	return s.membershipConfigs[name]
}

// RegisterDefaultIncludes stores the default relations to preload for an entity
func (s *EntityRegistrationService) RegisterDefaultIncludes(name string, includes []string) {
	if includes != nil && len(includes) > 0 {
		s.defaultIncludes[name] = includes
		log.Printf("📦 Default includes registered for entity: %s -> %v", name, includes)
	}
}

// GetDefaultIncludes retrieves the default relations to preload for an entity
func (s *EntityRegistrationService) GetDefaultIncludes(name string) []string {
	return s.defaultIncludes[name]
}

// HasMembershipConfig checks if an entity has a membership configuration
func (s *EntityRegistrationService) HasMembershipConfig(name string) bool {
	return s.membershipConfigs[name] != nil
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
	resourceName = utils.PascalToKebab(resourceName)
	resourceName = strings.ToLower(resourceName)
	apiGroupPath := "/api/v1/" + resourceName + "/*" // Use wildcard for specific resource endpoints
	apiListPath := "/api/v1/" + resourceName         // List endpoint without wildcard

	log.Printf("Setting up entity access for %s at %s and %s", entityName, apiListPath, apiGroupPath)

	for ocfRoleName, accessGiven := range rolesMap {
		// Add permission for the list endpoint (without wildcard)
		_, errListPolicy := enforcer.AddPolicy(ocfRoleName, apiListPath, accessGiven)
		if errListPolicy != nil {
			if strings.Contains(errListPolicy.Error(), "UNIQUE") {
				log.Printf("OCF role permission already exists: %s %s %s", ocfRoleName, apiListPath, accessGiven)
			} else {
				log.Printf("Error adding OCF role permission: %v", errListPolicy)
			}
		} else {
			log.Printf("Added OCF role permission: %s can %s %s", ocfRoleName, accessGiven, apiListPath)
		}

		// Add permission for specific resource endpoints (with wildcard)
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
			// Add permission for list endpoint
			_, errCasdoorListPolicy := enforcer.AddPolicy(casdoorRole, apiListPath, accessGiven)
			if errCasdoorListPolicy != nil {
				if strings.Contains(errCasdoorListPolicy.Error(), "UNIQUE") {
					log.Printf("Casdoor role permission already exists: %s %s %s", casdoorRole, apiListPath, accessGiven)
				} else {
					log.Printf("Error adding Casdoor role permission: %v", errCasdoorListPolicy)
				}
			} else {
				log.Printf("Added Casdoor role permission: %s can %s %s", casdoorRole, accessGiven, apiListPath)
			}

			// Add permission for specific resource endpoints
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
	GlobalEntityRegistrationService.RegisterMembershipConfig(entityName, entityToRegister.MembershipConfig)
	GlobalEntityRegistrationService.RegisterDefaultIncludes(entityName, entityToRegister.DefaultIncludes)

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
