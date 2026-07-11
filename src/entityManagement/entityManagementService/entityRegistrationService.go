package services

import (
	"fmt"
	access "soli/formations/src/auth/access"
	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/interfaces"
	authModels "soli/formations/src/auth/models"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/entityManagement/utils"
	appUtils "soli/formations/src/utils"
	"strings"

	"github.com/gertd/go-pluralize"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// DtoRedactor is invoked by the generic GET handlers AFTER the model has been
// converted to its output DTO, but BEFORE the response is written. It lets a
// module strip sensitive fields from the DTO based on the requesting user's
// authorization (typically read from the gin.Context — userId, userRoles) and
// authorization queries against the supplied database handle.
//
// The DTO is passed by pointer to a typed value (e.g. *dto.ScenarioOutput);
// implementations should do a type assertion. Mutating the DTO in place is
// the expected pattern. Returning an error aborts the request with 500.
//
// The db parameter is the controller's *gorm.DB, passed explicitly so the
// redactor does not depend on a stringly-typed gin.Context key.
type DtoRedactor func(c *gin.Context, dto any, db *gorm.DB) error

type EntityRegistrationService struct {
	registry            map[string]any
	subEntities         map[string][]any
	swaggerConfigs      map[string]*entityManagementInterfaces.EntitySwaggerConfig
	relationshipFilters map[string][]entityManagementInterfaces.RelationshipFilter
	membershipConfigs   map[string]*entityManagementInterfaces.MembershipConfig
	ownershipConfigs    map[string]*access.OwnershipConfig
	defaultIncludes     map[string][]string
	typedOps            map[string]entityManagementInterfaces.EntityOperations
	entityRoles         map[string]entityManagementInterfaces.EntityRoles
	dtoRedactors        map[string]DtoRedactor
	entityActions       map[string][]entityManagementInterfaces.ActionConfig
}

func NewEntityRegistrationService() *EntityRegistrationService {
	return &EntityRegistrationService{
		registry:            make(map[string]any),
		subEntities:         make(map[string][]any),
		swaggerConfigs:      make(map[string]*entityManagementInterfaces.EntitySwaggerConfig),
		relationshipFilters: make(map[string][]entityManagementInterfaces.RelationshipFilter),
		membershipConfigs:   make(map[string]*entityManagementInterfaces.MembershipConfig),
		ownershipConfigs:    make(map[string]*access.OwnershipConfig),
		defaultIncludes:     make(map[string][]string),
		typedOps:            make(map[string]entityManagementInterfaces.EntityOperations),
		entityRoles:         make(map[string]entityManagementInterfaces.EntityRoles),
		dtoRedactors:        make(map[string]DtoRedactor),
		entityActions:       make(map[string][]entityManagementInterfaces.ActionConfig),
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
	s.ownershipConfigs = make(map[string]*access.OwnershipConfig)
	s.defaultIncludes = make(map[string][]string)
	s.typedOps = make(map[string]entityManagementInterfaces.EntityOperations)
	s.entityRoles = make(map[string]entityManagementInterfaces.EntityRoles)
	s.dtoRedactors = make(map[string]DtoRedactor)
	s.entityActions = make(map[string][]entityManagementInterfaces.ActionConfig)
}

// UnregisterEntity removes all registrations for a specific entity
// This is primarily used for testing to clean up after individual tests
func (s *EntityRegistrationService) UnregisterEntity(name string) {
	delete(s.registry, name)
	delete(s.subEntities, name)
	delete(s.swaggerConfigs, name)
	delete(s.relationshipFilters, name)
	delete(s.membershipConfigs, name)
	delete(s.ownershipConfigs, name)
	delete(s.defaultIncludes, name)
	delete(s.typedOps, name)
	delete(s.entityRoles, name)
	delete(s.dtoRedactors, name)
	delete(s.entityActions, name)
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

// RegisterOwnershipConfig stores an entity's ownership configuration so the
// generic read handlers can consult it (e.g. to scope list/get to the owner).
// The write-side ownership hooks are wired separately from this registry.
func (s *EntityRegistrationService) RegisterOwnershipConfig(name string, config *access.OwnershipConfig) {
	if config != nil {
		s.ownershipConfigs[name] = config
		appUtils.Debug("Ownership config registered for entity: %s (field: %s)", name, config.OwnerField)
	}
}

// GetOwnershipConfig retrieves the ownership configuration for an entity, or nil.
func (s *EntityRegistrationService) GetOwnershipConfig(name string) *access.OwnershipConfig {
	return s.ownershipConfigs[name]
}

// GetAllOwnershipConfigs returns a copy of the entityName→OwnershipConfig map for
// every entity that declared one. RegisterOwnershipHooks walks this at startup to
// wire the write-side ownership hooks from the declarative configs.
func (s *EntityRegistrationService) GetAllOwnershipConfigs() map[string]*access.OwnershipConfig {
	result := make(map[string]*access.OwnershipConfig, len(s.ownershipConfigs))
	for name, config := range s.ownershipConfigs {
		result[name] = config
	}
	return result
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

	basePath := "/api/v1" + ResourceBasePath(entityName)
	apiGroupPath := basePath + "/:id" // Match single resource by ID only (not sub-paths)
	apiListPath := basePath           // List endpoint without wildcard

	appUtils.Info("Setting up entity access for %s at %s and %s", entityName, apiListPath, apiGroupPath)

	for ocfRoleName, accessGiven := range rolesMap {
		// Reconcile OCF role policies: only update if DB differs from code
		reconcileEntityPolicy(enforcer, ocfRoleName, apiListPath, accessGiven)
		reconcileEntityPolicy(enforcer, ocfRoleName, apiGroupPath, accessGiven)

		// Reconcile corresponding Casdoor role policies
		casdoorRoles := authModels.GetCasdoorRolesForOCFRole(authModels.RoleName(ocfRoleName))
		for _, casdoorRole := range casdoorRoles {
			reconcileEntityPolicy(enforcer, casdoorRole, apiListPath, accessGiven)
			reconcileEntityPolicy(enforcer, casdoorRole, apiGroupPath, accessGiven)
		}
	}

	appUtils.Info("Completed entity access setup for %s", entityName)
}

// reconcileEntityPolicy idempotently registers the entity CRUD policy for the
// exact (role, path, method) triple. Delegates to access.ReconcilePolicy — the
// single reconciler — so the entity path shares the #297 wipe-safe behavior
// (never removes sibling methods; a code-removed method lingers as a harmless
// over-grant, detectable via ValidatePermissionSetup).
func reconcileEntityPolicy(enforcer interfaces.EnforcerInterface, role, path, method string) {
	access.ReconcilePolicy(enforcer, role, path, method)
}

func Pluralize(entityName string) string {
	client := pluralize.NewClient()
	plural := client.Plural(entityName)
	return plural
}

// ResourceBasePath derives an entity's REST base path (leading slash, no
// /api/v1 prefix) from its registration name, e.g. "EmailTemplate" →
// "/email-templates". This is the single source of truth shared by the Casbin
// policy setup (setDefaultEntityAccesses) and the route generator, so the two
// can never derive divergent paths for the same entity.
func ResourceBasePath(entityName string) string {
	return "/" + utils.PascalToKebab(Pluralize(entityName))
}

// ActionRelativePath returns an action's path relative to its entity base path.
// Item-scoped actions hang off a single instance (/:id/<name>); collection-scoped
// actions hang off the collection (/<name>).
func ActionRelativePath(a entityManagementInterfaces.ActionConfig) string {
	if a.Scope == entityManagementInterfaces.ActionScopeItem {
		return "/:id/" + a.Name
	}
	return "/" + a.Name
}

// ValidateActionConfig returns a descriptive error when a declared action is
// missing a required field (Name, Method, Role, Access.Type, or Handler). It is
// called at registration so a misdeclared action fails fast rather than booting
// silently as an unauthorized or unmountable route; the error names the entity
// and the offending field to keep the startup log actionable.
func ValidateActionConfig(entityName string, a entityManagementInterfaces.ActionConfig) error {
	switch {
	case a.Name == "":
		return fmt.Errorf("entity %q: action config is missing Name", entityName)
	case a.Method == "":
		return fmt.Errorf("entity %q: action %q is missing Method", entityName, a.Name)
	case a.Role == "":
		return fmt.Errorf("entity %q: action %q is missing Role", entityName, a.Name)
	case a.Access.Type == "":
		return fmt.Errorf("entity %q: action %q is missing Access.Type", entityName, a.Name)
	case a.Handler == nil:
		return fmt.Errorf("entity %q: action %q has a nil Handler", entityName, a.Name)
	}
	return nil
}

// normalizeActionConfig canonicalizes an action's HTTP method (trim + uppercase).
// It is the single source of truth for method normalization, shared by both
// registration entry points so the Casbin triple, the RouteRegistry key, and the
// gin mount can never derive divergent methods for the same action.
func normalizeActionConfig(a entityManagementInterfaces.ActionConfig) entityManagementInterfaces.ActionConfig {
	a.Method = strings.ToUpper(strings.TrimSpace(a.Method))
	return a
}

// RegisterEntityActions normalizes, validates, and stores the custom actions
// declared for an entity, returning the normalized slice so callers can wire the
// same values into access setup without a second copy of the normalization rule.
// A misdeclared action panics (startup fail-fast).
func (s *EntityRegistrationService) RegisterEntityActions(name string, actions []entityManagementInterfaces.ActionConfig) []entityManagementInterfaces.ActionConfig {
	if len(actions) == 0 {
		return nil
	}
	normalized := make([]entityManagementInterfaces.ActionConfig, len(actions))
	for i, a := range actions {
		a = normalizeActionConfig(a)
		if err := ValidateActionConfig(name, a); err != nil {
			panic(err)
		}
		normalized[i] = a
	}
	s.entityActions[name] = normalized
	return normalized
}

// GetActions returns the custom actions declared for an entity, or nil if none.
func (s *EntityRegistrationService) GetActions(name string) []entityManagementInterfaces.ActionConfig {
	return s.entityActions[name]
}

// GetAllActions returns a copy of the entityName→actions map for every entity
// that declared at least one action.
func (s *EntityRegistrationService) GetAllActions() map[string][]entityManagementInterfaces.ActionConfig {
	result := make(map[string][]entityManagementInterfaces.ActionConfig, len(s.entityActions))
	for name, actions := range s.entityActions {
		result[name] = actions
	}
	return result
}

// SetEntityActionAccesses registers, for each declared action, the Layer 1
// Casbin policy and Layer 2 RoutePermission (keyed under the entity name as its
// category) so custom actions are authorized exactly like CRUD routes.
func (s *EntityRegistrationService) SetEntityActionAccesses(entityName string, actions []entityManagementInterfaces.ActionConfig, enforcer interfaces.EnforcerInterface) {
	// Normalize + validate up front, ahead of the early-returns below: this entry
	// point is independently callable, and a misdeclared action must fail fast
	// (panic) even when the enforcer is nil — otherwise the nil-enforcer return
	// would let a bad declaration boot silently.
	normalized := make([]entityManagementInterfaces.ActionConfig, 0, len(actions))
	for _, a := range actions {
		a = normalizeActionConfig(a)
		if err := ValidateActionConfig(entityName, a); err != nil {
			panic(err)
		}
		normalized = append(normalized, a)
	}

	if len(normalized) == 0 {
		return
	}
	if enforcer == nil {
		appUtils.Warn("Enforcer is nil, skipping action access setup for %s", entityName)
		return
	}

	basePath := "/api/v1" + ResourceBasePath(entityName)
	perms := make([]access.RoutePermission, 0, len(normalized))
	for _, a := range normalized {
		perms = append(perms, access.RoutePermission{
			Path:        basePath + ActionRelativePath(a),
			Method:      a.Method,
			Role:        a.Role,
			Access:      a.Access,
			Description: a.Description,
		})
	}
	access.RegisterEnforced(enforcer, entityName, perms...)
}

// GetAllEntityRoles returns the roles configuration for all registered entities
func (s *EntityRegistrationService) GetAllEntityRoles() map[string]entityManagementInterfaces.EntityRoles {
	result := make(map[string]entityManagementInterfaces.EntityRoles)
	for k, v := range s.entityRoles {
		result[k] = v
	}
	return result
}

// GetEntityOps returns the typed operations for the named entity, if registered.
func (s *EntityRegistrationService) GetEntityOps(name string) (entityManagementInterfaces.EntityOperations, bool) {
	ops, ok := s.typedOps[name]
	return ops, ok
}

// RegisterDtoRedactor registers a redactor invoked by the generic GET handlers
// after model→DTO conversion. Use this to strip sensitive fields from the DTO
// based on the requesting user's authorization (e.g. hide step content from
// non-managers in scenarios).
func (s *EntityRegistrationService) RegisterDtoRedactor(name string, r DtoRedactor) {
	if r == nil {
		return
	}
	s.dtoRedactors[name] = r
	appUtils.Debug("DTO redactor registered for entity: %s", name)
}

// GetDtoRedactor returns the redactor for the named entity, if any.
func (s *EntityRegistrationService) GetDtoRedactor(name string) (DtoRedactor, bool) {
	r, ok := s.dtoRedactors[name]
	return r, ok
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
	service.RegisterOwnershipConfig(name, reg.OwnershipConfig)
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

	// Store entity roles for security admin panel
	service.entityRoles[name] = reg.Roles

	// Set up access policies
	service.setDefaultEntityAccesses(name, reg.Roles, casdoor.Enforcer)

	// Register CRUD permissions in RouteRegistry for the permission reference page
	access.RouteRegistry.RegisterEntity(access.EntityCRUDPermissions{
		Entity: name,
		Create: deriveAccessRule(reg.Roles, "POST", name, reg.OwnershipConfig),
		Read:   deriveAccessRule(reg.Roles, "GET", name, reg.OwnershipConfig),
		Update: deriveAccessRule(reg.Roles, "PATCH", name, reg.OwnershipConfig),
		Delete: deriveAccessRule(reg.Roles, "DELETE", name, reg.OwnershipConfig),
	})

	// Store custom actions and set up their Layer 1 / Layer 2 access policies.
	// Normalization/validation happens once in RegisterEntityActions; feed the
	// normalized slice into access setup so both see the same uppercase method.
	normalizedActions := service.RegisterEntityActions(name, reg.Actions)
	service.SetEntityActionAccesses(name, normalizedActions, casdoor.Enforcer)
}

// deriveAccessRule determines the Layer 2 access rule for an entity CRUD operation
// based on RBAC role config and optional ownership config.
func deriveAccessRule(roles entityManagementInterfaces.EntityRoles, method string, entityName string, ownershipConfig *access.OwnershipConfig) access.AccessRule {
	memberMethods := roles.Roles["member"]

	// Check if member role includes this HTTP method
	memberHasAccess := strings.Contains(memberMethods, method)

	if !memberHasAccess {
		return access.AccessRule{Type: access.AdminOnly}
	}

	// Member has access — check if an ownership hook protects this operation
	if ownershipConfig != nil {
		opMap := map[string]string{"GET": "read", "POST": "create", "PATCH": "update", "DELETE": "delete"}
		if op, ok := opMap[method]; ok {
			for _, configOp := range ownershipConfig.Operations {
				if configOp == op {
					if op == "create" {
						return access.AccessRule{Type: access.SelfScoped}
					}
					return access.AccessRule{
						Type:   access.EntityOwner,
						Entity: entityName,
						Field:  ownershipConfig.OwnerField,
					}
				}
			}
		}
	}

	return access.AccessRule{Type: access.Public}
}

var GlobalEntityRegistrationService = NewEntityRegistrationService()
