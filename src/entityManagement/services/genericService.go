package services

import (
	"context"
	"log"
	"reflect"
	authInterfaces "soli/formations/src/auth/interfaces"
	entityErrors "soli/formations/src/entityManagement/errors"
	"soli/formations/src/entityManagement/hooks"
	"soli/formations/src/entityManagement/repositories"
	"soli/formations/src/entityManagement/utils"
	"time"

	ems "soli/formations/src/entityManagement/entityManagementService"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/mitchellh/mapstructure"
	"gorm.io/gorm"
)

// stringToUUIDHookFunc returns a decode hook that converts strings to uuid.UUID
func stringToUUIDHookFunc() mapstructure.DecodeHookFunc {
	return func(
		f reflect.Type,
		t reflect.Type,
		data any,
	) (any, error) {
		// Target must be uuid.UUID
		if t != reflect.TypeOf(uuid.UUID{}) {
			return data, nil
		}

		// If source is already uuid.UUID, pass it through
		if f == reflect.TypeOf(uuid.UUID{}) {
			return data, nil
		}

		// If source is string, parse it
		if f.Kind() == reflect.String {
			return uuid.Parse(data.(string))
		}

		// For any other type, return as-is (will fail later if incompatible)
		return data, nil
	}
}

type GenericService interface {
	CreateEntity(inputDto any, entityName string) (any, error)
	CreateEntityWithUser(inputDto any, entityName string, userID string) (any, error)
	SaveEntity(entity any) (any, error)
	GetEntity(id uuid.UUID, data any, entityName string, includes []string) (any, error)
	GetEntities(data any, page int, pageSize int, filters map[string]any, includes []string) ([]any, int64, error)
	GetEntitiesCursor(data any, cursor string, limit int, filters map[string]any, includes []string) ([]any, string, bool, int64, error)
	DeleteEntity(id uuid.UUID, entity any, scoped bool) error
	EditEntity(id uuid.UUID, entityName string, entity any, data any) error
	GetEntityModelInterface(entityName string) any
	AddOwnerIDs(entity any, userId string) (any, error)
	ExtractUuidFromReflectEntity(entity any) uuid.UUID
	GetDtoArrayFromEntitiesPages(allEntitiesPages []any, entityModelInterface any, entityName string) ([]any, bool)
	GetEntityFromResult(entityName string, item any) (any, bool)
	AddDefaultAccessesForEntity(resourceName string, entity any, userId string) error
	DecodeInputDtoForEntityCreation(entityName string, ctx *gin.Context) (any, error)
}

type genericService struct {
	genericRepository repositories.GenericRepository
	enforcer          authInterfaces.EnforcerInterface
}

// NewGenericService creates a new generic service with the given database and enforcer.
// The enforcer parameter can be nil for testing purposes.
func NewGenericService(db *gorm.DB, enforcer authInterfaces.EnforcerInterface) GenericService {
	return &genericService{
		genericRepository: repositories.NewGenericRepository(db),
		enforcer:          enforcer,
	}
}

func (g *genericService) CreateEntity(inputDto any, entityName string) (any, error) {
	return g.CreateEntityWithUser(inputDto, entityName, "")
}

func (g *genericService) CreateEntityWithUser(inputDto any, entityName string, userID string) (any, error) {
	// Convert DTO to model entity before calling BeforeCreate hook
	ops, ok := ems.GlobalEntityRegistrationService.GetEntityOps(entityName)
	if !ok {
		return nil, entityErrors.NewConversionError(entityName, "no typed operations registered")
	}

	entityModel, err := ops.ConvertDtoToModel(inputDto)
	if err != nil {
		return nil, entityErrors.NewConversionError(entityName, err.Error())
	}

	// Call BeforeCreate hook with the converted model
	beforeCtx := &hooks.HookContext{
		EntityName: entityName,
		HookType:   hooks.BeforeCreate,
		NewEntity:  entityModel,
		UserID:     userID,
		Context:    context.Background(),
	}

	if err := hooks.GlobalHookRegistry.ExecuteHooks(beforeCtx); err != nil {
		return nil, entityErrors.WrapHookError("BeforeCreate", entityName, err)
	}

	// Create the entity in the database (the repository will use entityModel instead of inputDto)
	entity, createEntityError := g.genericRepository.CreateEntityFromModel(entityModel)
	if createEntityError != nil {
		return nil, createEntityError
	}

	afterCtx := &hooks.HookContext{
		EntityName: entityName,
		HookType:   hooks.AfterCreate,
		NewEntity:  entity,
		EntityID:   g.extractEntityID(entityName, entity),
		UserID:     userID,
		Context:    context.Background(),
	}

	// Exécuter les hooks après création (synchrone en test mode, async sinon)
	if hooks.GlobalHookRegistry.IsTestMode() {
		if err := hooks.GlobalHookRegistry.ExecuteHooks(afterCtx); err != nil {
			log.Printf("❌ after_create hooks failed for %s: %v", entityName, err)
		}
	} else {
		go func() {
			if err := hooks.GlobalHookRegistry.ExecuteHooks(afterCtx); err != nil {
				log.Printf("❌ after_create hooks failed for %s: %v", entityName, err)
			}
		}()
	}

	return entity, nil
}

func (g *genericService) SaveEntity(entity any) (any, error) {

	entity, saveEntityError := g.genericRepository.SaveEntity(entity)
	if saveEntityError != nil {
		return nil, saveEntityError
	}

	return entity, nil
}

func (g *genericService) GetEntity(id uuid.UUID, data any, entityName string, includes []string) (any, error) {
	entity, err := g.genericRepository.GetEntity(id, data, entityName, includes)

	if err != nil {
		return nil, err
	}

	if g.extractEntityID(entityName, entity) == uuid.Nil {
		return nil, entityErrors.NewEntityNotFound(entityName, id)
	}

	return entity, nil

}

// should return an array of dtoEntityOutput
func (g *genericService) GetEntities(data any, page int, pageSize int, filters map[string]any, includes []string) ([]any, int64, error) {

	allPages, total, err := g.genericRepository.GetAllEntities(data, page, pageSize, filters, includes)

	if err != nil {
		return nil, 0, err
	}

	return allPages, total, nil
}

// GetEntitiesCursor retrieves entities using cursor-based pagination.
// This method delegates to the repository layer for efficient cursor-based traversal.
func (g *genericService) GetEntitiesCursor(data any, cursor string, limit int, filters map[string]any, includes []string) ([]any, string, bool, int64, error) {

	allPages, nextCursor, hasMore, total, err := g.genericRepository.GetAllEntitiesCursor(data, cursor, limit, filters, includes)

	if err != nil {
		return nil, "", false, 0, err
	}

	return allPages, nextCursor, hasMore, total, nil
}

func (g *genericService) DeleteEntity(id uuid.UUID, entity any, scoped bool) error {
	typeOfEntity := reflect.TypeOf(entity)
	entityName := typeOfEntity.Name()

	if entityName == "" {
		entityName = typeOfEntity.Elem().Name()
	}

	entityModelInterface := g.GetEntityModelInterface(entityName)

	if entityName == "" {
		return entityErrors.NewInvalidInputError("entityName", "", "could not find entity type")
	}

	if entityModelInterface == nil {
		return entityErrors.NewEntityNotRegistered(entityName)
	}

	// Récupérer l'entité avant suppression
	existingEntity, err := g.GetEntity(id, entityModelInterface, entityName, nil)
	if err != nil {
		return err
	}

	// 🎯 Hook BEFORE_DELETE
	beforeCtx := &hooks.HookContext{
		EntityName: entityName,
		HookType:   hooks.BeforeDelete,
		EntityID:   id,
		NewEntity:  existingEntity,
		Context:    context.Background(),
	}

	if err := hooks.GlobalHookRegistry.ExecuteHooks(beforeCtx); err != nil {
		return entityErrors.WrapHookError("BeforeDelete", entityName, err)
	}

	errorDelete := g.genericRepository.DeleteEntity(id, entity, scoped)
	if errorDelete != nil {
		return errorDelete
	}

	afterCtx := &hooks.HookContext{
		EntityName: entityName,
		HookType:   hooks.AfterDelete,
		EntityID:   id,
		NewEntity:  existingEntity,
		Context:    context.Background(),
	}

	// Exécuter les hooks après suppression (synchrone en test mode, async sinon)
	if hooks.GlobalHookRegistry.IsTestMode() {
		if err := hooks.GlobalHookRegistry.ExecuteHooks(afterCtx); err != nil {
			log.Printf("❌ after_delete hooks failed for %s: %v", entityName, err)
		}
	} else {
		go func() {
			if err := hooks.GlobalHookRegistry.ExecuteHooks(afterCtx); err != nil {
				log.Printf("❌ after_delete hooks failed for %s: %v", entityName, err)
			}
		}()
	}

	return nil
}

func (g *genericService) EditEntity(id uuid.UUID, entityName string, entity any, data any) error {
	// Récupérer l'entité existante pour les hooks
	oldEntity, err := g.GetEntity(id, entity, entityName, nil)
	if err != nil {
		return err
	}

	beforeCtx := &hooks.HookContext{
		EntityName: entityName,
		HookType:   hooks.BeforeUpdate,
		EntityID:   id,
		OldEntity:  oldEntity,
		NewEntity:  data,
		Context:    context.Background(),
	}

	if err := hooks.GlobalHookRegistry.ExecuteHooks(beforeCtx); err != nil {
		return entityErrors.WrapHookError("BeforeUpdate", entityName, err)
	}

	errorPatch := g.genericRepository.EditEntity(id, entityName, entity, data)
	if errorPatch != nil {
		return errorPatch
	}

	updatedEntity, err := g.GetEntity(id, entity, entityName, nil)
	if err != nil {
		log.Printf("Warning: could not retrieve updated entity for hooks: %v", err)
		updatedEntity = data // Fallback
	}

	afterCtx := &hooks.HookContext{
		EntityName: entityName,
		HookType:   hooks.AfterUpdate,
		EntityID:   id,
		OldEntity:  oldEntity,
		NewEntity:  updatedEntity,
		Context:    context.Background(),
	}

	// Exécuter les hooks après mise à jour (synchrone en test mode, async sinon)
	if hooks.GlobalHookRegistry.IsTestMode() {
		if err := hooks.GlobalHookRegistry.ExecuteHooks(afterCtx); err != nil {
			log.Printf("❌ after_update hooks failed for %s: %v", entityName, err)
		}
	} else {
		go func() {
			if err := hooks.GlobalHookRegistry.ExecuteHooks(afterCtx); err != nil {
				log.Printf("❌ after_update hooks failed for %s: %v", entityName, err)
			}
		}()
	}
	return nil
}

func (g *genericService) GetEntityModelInterface(entityName string) any {
	var result any
	result, _ = ems.GlobalEntityRegistrationService.GetEntityInterface(entityName)
	return result
}

func (g *genericService) AddOwnerIDs(entity any, userId string) (any, error) {
	// Add owner ID to entity (modifies in-place)
	if err := utils.AddOwnerIDToEntity(entity, userId); err != nil {
		return nil, err
	}

	// Save entity with updated OwnerIDs
	entityWithOwnerIds, entitySavingError := g.SaveEntity(entity)
	if entitySavingError != nil {
		return nil, entitySavingError
	}

	return entityWithOwnerIds, nil
}

func (g *genericService) ExtractUuidFromReflectEntity(entity any) uuid.UUID {
	// Legacy reflect path (kept for backward compatibility)
	entityReflectValue := reflect.ValueOf(entity).Elem()
	field := entityReflectValue.FieldByName("ID")

	var entityUUID uuid.UUID

	result, ok := field.Interface().(string)
	if ok {
		entityUUID, _ = uuid.Parse(result)
	} else {
		entityUUID = uuid.UUID(field.Bytes())
	}

	return entityUUID
}

// extractEntityID tries the typed EntityOperations first, falls back to reflect.
func (g *genericService) extractEntityID(entityName string, entity any) uuid.UUID {
	if ops, ok := ems.GlobalEntityRegistrationService.GetEntityOps(entityName); ok {
		id, err := ops.ExtractID(entity)
		if err == nil {
			return id
		}
	}
	return g.ExtractUuidFromReflectEntity(entity)
}

func (g *genericService) GetDtoArrayFromEntitiesPages(allEntitiesPages []any, entityModelInterface any, entityName string) ([]any, bool) {
	// Fast path: use typed operations (no reflect)
	if ops, ok := ems.GlobalEntityRegistrationService.GetEntityOps(entityName); ok {
		allDtos := make([]any, 0, len(allEntitiesPages)*10)
		for _, page := range allEntitiesPages {
			dtos, err := ops.ConvertSliceToDto(page)
			if err != nil {
				return nil, true
			}
			allDtos = append(allDtos, dtos...)
		}
		return allDtos, false
	}

	// Legacy reflect path
	estimatedCapacity := len(allEntitiesPages) * 10
	entitiesDto := make([]any, 0, estimatedCapacity)

	for _, page := range allEntitiesPages {

		entityModel := reflect.SliceOf(reflect.TypeOf(entityModelInterface))

		pageValue := reflect.ValueOf(page)

		if pageValue.Type().ConvertibleTo(entityModel) {
			convertedPage := pageValue.Convert(entityModel)

			for i := 0; i < convertedPage.Len(); i++ {

				item := convertedPage.Index(i).Interface()

				var shouldReturn bool
				entitiesDto, shouldReturn = g.appendEntityFromResult(entityName, item, entitiesDto)
				if shouldReturn {
					return nil, true
				}
			}
		} else {
			return nil, true
		}

	}
	return entitiesDto, false
}

// used in get
func (g *genericService) appendEntityFromResult(entityName string, item any, entitiesDto []any) ([]any, bool) {
	result, ko := g.GetEntityFromResult(entityName, item)
	if !ko {
		entitiesDto = append(entitiesDto, result)
		return entitiesDto, false
	}

	return nil, true
}

// used in post and get
func (g *genericService) GetEntityFromResult(entityName string, item any) (any, bool) {
	ops, ok := ems.GlobalEntityRegistrationService.GetEntityOps(entityName)
	if !ok {
		return nil, true
	}
	result, err := ops.ConvertModelToDto(item)
	if err != nil {
		return nil, true
	}
	return result, false
}

func (g *genericService) AddDefaultAccessesForEntity(resourceName string, entity any, userId string) error {
	// Skip enforcer setup if not initialized (e.g., in tests)
	if g.enforcer == nil {
		return nil
	}

	errPolicyLoading := g.enforcer.LoadPolicy()
	if errPolicyLoading != nil {
		return errPolicyLoading
	}

	entityUuid := g.ExtractUuidFromReflectEntity(entity)

	_, errAddingPolicy := g.enforcer.AddPolicy(userId, "/api/v1/"+resourceName+"/"+entityUuid.String(), "(GET|DELETE|PATCH|PUT)")
	if errAddingPolicy != nil {
		return errAddingPolicy
	}

	return nil
}

func (g *genericService) DecodeInputDtoForEntityCreation(entityName string, ctx *gin.Context) (any, error) {
	ops, ok := ems.GlobalEntityRegistrationService.GetEntityOps(entityName)
	if !ok {
		return nil, entityErrors.NewEntityNotRegistered(entityName)
	}

	entityCreateDtoInput := ops.NewCreateDto()
	decodedData := ops.NewCreateDto()

	bindError := ctx.BindJSON(&entityCreateDtoInput)
	if bindError != nil {
		log.Printf("❌ BindJSON error for %s: %v", entityName, bindError)
		return nil, bindError
	}
	log.Printf("✅ BindJSON success for %s: %+v", entityName, entityCreateDtoInput)

	config := &mapstructure.DecoderConfig{
		WeaklyTypedInput: true,
		Result:           &decodedData,
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			stringToUUIDHookFunc(),                          // Handle UUID strings
			mapstructure.StringToTimeHookFunc(time.RFC3339), // Handle ISO8601 time strings
			mapstructure.StringToTimeDurationHookFunc(),     // Handle duration strings
		),
	}

	decoder, err := mapstructure.NewDecoder(config)
	if err != nil {
		panic(err)
	}

	log.Printf("🔄 Decoding %s with mapstructure...", entityName)
	errDecode := decoder.Decode(entityCreateDtoInput)
	if errDecode != nil {
		log.Printf("❌ Mapstructure decode error for %s: %v", entityName, errDecode)
		return nil, errDecode
	}
	log.Printf("✅ Mapstructure decode success for %s", entityName)

	return decodedData, nil
}
