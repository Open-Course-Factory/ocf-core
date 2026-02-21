package services

import (
	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/interfaces"
	authModels "soli/formations/src/auth/models"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/entityManagement/utils"
	appUtils "soli/formations/src/utils"
	"strings"

	"github.com/gertd/go-pluralize"
)

type EntityRegistrationService struct {
	registry            map[string]any
	subEntities         map[string][]any
	swaggerConfigs      map[string]*entityManagementInterfaces.EntitySwaggerConfig
	relationshipFilters map[string][]entityManagementInterfaces.RelationshipFilter
	membershipConfigs   map[string]*entityManagementInterfaces.MembershipConfig
	defaultIncludes     map[string][]string
	typedOps            map[string]entityManagementInterfaces.EntityOperations
}

func NewEntityRegistrationService() *EntityRegistrationService {
	return &EntityRegistrationService{
		registry:            make(map[string]any),
		subEntities:         make(map[string][]any),
		swaggerConfigs:      make(map[string]*entityManagementInterfaces.EntitySwaggerConfig),
		relationshipFilters: make(map[string][]entityManagementInterfaces.RelationshipFilter),
		membershipConfigs:   make(map[string]*entityManagementInterfaces.MembershipConfig),
		defaultIncludes:     make(map[string][]string),
		typedOps:            make(map[string]entityManagementInterfaces.EntityOperations),
	}
}

// Reset clears all registered entities, functions, DTOs, and configurations
// This is primarily used for testing to ensure clean state between test runs
func (s *EntityRegistrationService) Reset() {
	s.registry = make(map[string]any)
	s.subEntities = make(map[string][]any)
	s.swaggerConfigs = make(map[string]*entityManagementInterfaces.EntitySwaggerConfig)
	s.relationshipFilters = make(map[string][]entityManagementInterfaces.RelationshipFilter)
	s.membershipConfigs = make(map[string]*entityManagementInterfaces.MembershipConfig)
	s.defaultIncludes = make(map[string][]string)
	s.typedOps = make(map[string]entityManagementInterfaces.EntityOperations)
}

// UnregisterEntity removes all registrations for a specific entity
// This is primarily used for testing to clean up after individual tests
func (s *EntityRegistrationService) UnregisterEntity(name string) {
	delete(s.registry, name)
	delete(s.subEntities, name)
	delete(s.swaggerConfigs, name)
	delete(s.relationshipFilters, name)
	delete(s.membershipConfigs, name)
	delete(s.defaultIncludes, name)
	delete(s.typedOps, name)
}

func (s *EntityRegistrationService) RegisterEntityInterface(name string, entityType any) {
	s.registry[name] = entityType
}

func (s *EntityRegistrationService) RegisterSubEntites(name string, subEntities []any) {
	s.subEntities[name] = subEntities
}

func (s *EntityRegistrationService) RegisterSwaggerConfig(name string, config *entityManagementInterfaces.EntitySwaggerConfig) {
	s.swaggerConfigs[name] = config
	appUtils.Debug("Swagger config registered for entity: %s (tag: %s)", name, config.Tag)
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
		appUtils.Debug("Membership config registered for entity: %s (table: %s)", name, config.MemberTable)
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
		appUtils.Debug("Default includes registered for entity: %s -> %v", name, includes)
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
		appUtils.Warn("Enforcer is nil, skipping access setup")
		return
	}

	errLoadingPolicy := enforcer.LoadPolicy()
	if errLoadingPolicy != nil {
		appUtils.Error("Failed to load policy for entity %s: %v", entityName, errLoadingPolicy)
		return
	}
	rolesMap := roles.Roles

	resourceName := Pluralize(entityName)
	resourceName = utils.PascalToKebab(resourceName)
	resourceName = strings.ToLower(resourceName)
	apiGroupPath := "/api/v1/" + resourceName + "/*" // Use wildcard for specific resource endpoints
	apiListPath := "/api/v1/" + resourceName         // List endpoint without wildcard

	appUtils.Info("Setting up entity access for %s at %s and %s", entityName, apiListPath, apiGroupPath)

	for ocfRoleName, accessGiven := range rolesMap {
		// Add permission for the list endpoint (without wildcard)
		_, errListPolicy := enforcer.AddPolicy(ocfRoleName, apiListPath, accessGiven)
		if errListPolicy != nil {
			if strings.Contains(errListPolicy.Error(), "UNIQUE") {
				appUtils.Debug("OCF role permission already exists: %s %s %s", ocfRoleName, apiListPath, accessGiven)
			} else {
				appUtils.Error("Error adding OCF role permission: %v", errListPolicy)
			}
		} else {
			appUtils.Debug("Added OCF role permission: %s can %s %s", ocfRoleName, accessGiven, apiListPath)
		}

		// Add permission for specific resource endpoints (with wildcard)
		_, errPolicy := enforcer.AddPolicy(ocfRoleName, apiGroupPath, accessGiven)
		if errPolicy != nil {
			if strings.Contains(errPolicy.Error(), "UNIQUE") {
				appUtils.Debug("OCF role permission already exists: %s %s %s", ocfRoleName, apiGroupPath, accessGiven)
			} else {
				appUtils.Error("Error adding OCF role permission: %v", errPolicy)
			}
		} else {
			appUtils.Debug("Added OCF role permission: %s can %s %s", ocfRoleName, accessGiven, apiGroupPath)
		}

		// Automatically add permissions for corresponding Casdoor roles
		casdoorRoles := authModels.GetCasdoorRolesForOCFRole(authModels.RoleName(ocfRoleName))
		for _, casdoorRole := range casdoorRoles {
			// Add permission for list endpoint
			_, errCasdoorListPolicy := enforcer.AddPolicy(casdoorRole, apiListPath, accessGiven)
			if errCasdoorListPolicy != nil {
				if strings.Contains(errCasdoorListPolicy.Error(), "UNIQUE") {
					appUtils.Debug("Casdoor role permission already exists: %s %s %s", casdoorRole, apiListPath, accessGiven)
				} else {
					appUtils.Error("Error adding Casdoor role permission: %v", errCasdoorListPolicy)
				}
			} else {
				appUtils.Debug("Added Casdoor role permission: %s can %s %s", casdoorRole, accessGiven, apiListPath)
			}

			// Add permission for specific resource endpoints
			_, errCasdoorPolicy := enforcer.AddPolicy(casdoorRole, apiGroupPath, accessGiven)
			if errCasdoorPolicy != nil {
				if strings.Contains(errCasdoorPolicy.Error(), "UNIQUE") {
					appUtils.Debug("Casdoor role permission already exists: %s %s %s", casdoorRole, apiGroupPath, accessGiven)
				} else {
					appUtils.Error("Error adding Casdoor role permission: %v", errCasdoorPolicy)
				}
			} else {
				appUtils.Debug("Added Casdoor role permission: %s can %s %s", casdoorRole, accessGiven, apiGroupPath)
			}
		}
	}

	appUtils.Info("Completed entity access setup for %s", entityName)
}

func Pluralize(entityName string) string {
	client := pluralize.NewClient()
	plural := client.Plural(entityName)
	return plural
}

// GetEntityOps returns the typed operations for the named entity, if registered.
func (s *EntityRegistrationService) GetEntityOps(name string) (entityManagementInterfaces.EntityOperations, bool) {
	ops, ok := s.typedOps[name]
	return ops, ok
}

// RegisterTypedEntity registers an entity using type-safe generics.
func RegisterTypedEntity[M entityManagementInterfaces.EntityModel, C any, E any, O any](
	service *EntityRegistrationService,
	name string,
	reg entityManagementInterfaces.TypedEntityRegistration[M, C, E, O],
) {
	// Create typed operations bridge
	ops := entityManagementInterfaces.NewTypedEntityOps[M, C, E, O](reg.Converters)
	service.typedOps[name] = ops

	// Populate registry with a zero-value model instance
	service.registry[name] = *new(M)

	// Register optional configs
	service.RegisterSubEntites(name, reg.SubEntities)
	service.RegisterRelationshipFilters(name, reg.RelationshipFilters)
	service.RegisterMembershipConfig(name, reg.MembershipConfig)
	service.RegisterDefaultIncludes(name, reg.DefaultIncludes)

	// Swagger config
	if reg.SwaggerConfig != nil {
		if reg.SwaggerConfig.EntityName == "" {
			reg.SwaggerConfig.EntityName = name
		}
		service.RegisterSwaggerConfig(name, reg.SwaggerConfig)
	} else {
		appUtils.Debug("Entity %s registered without Swagger documentation", name)
	}

	// Set up access policies
	service.setDefaultEntityAccesses(name, reg.Roles, casdoor.Enforcer)
}

var GlobalEntityRegistrationService = NewEntityRegistrationService()
