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

	ems "soli/formations/src/entityManagement/entityManagementService"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/mitchellh/mapstructure"
	"gorm.io/gorm"
)

type GenericService interface {
	CreateEntity(inputDto interface{}, entityName string) (interface{}, error)
	SaveEntity(entity interface{}) (interface{}, error)
	GetEntity(id uuid.UUID, data interface{}, entityName string, includes []string) (interface{}, error)
	GetEntities(data interface{}, page int, pageSize int, filters map[string]interface{}, includes []string) ([]interface{}, int64, error)
	GetEntitiesCursor(data interface{}, cursor string, limit int, filters map[string]interface{}, includes []string) ([]interface{}, string, bool, int64, error)
	DeleteEntity(id uuid.UUID, entity interface{}, scoped bool) error
	EditEntity(id uuid.UUID, entityName string, entity interface{}, data interface{}) error
	GetEntityModelInterface(entityName string) interface{}
	AddOwnerIDs(entity interface{}, userId string) (interface{}, error)
	ExtractUuidFromReflectEntity(entity interface{}) uuid.UUID
	GetDtoArrayFromEntitiesPages(allEntitiesPages []interface{}, entityModelInterface interface{}, entityName string) ([]interface{}, bool)
	GetEntityFromResult(entityName string, item interface{}) (interface{}, bool)
	AddDefaultAccessesForEntity(resourceName string, entity interface{}, userId string) error
	DecodeInputDtoForEntityCreation(entityName string, ctx *gin.Context) (interface{}, error)
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

func (g *genericService) CreateEntity(inputDto interface{}, entityName string) (interface{}, error) {

	beforeCtx := &hooks.HookContext{
		EntityName: entityName,
		HookType:   hooks.BeforeCreate,
		NewEntity:  inputDto,
		Context:    context.Background(),
	}

	if err := hooks.GlobalHookRegistry.ExecuteHooks(beforeCtx); err != nil {
		return nil, entityErrors.WrapHookError("BeforeCreate", entityName, err)
	}

	entity, createEntityError := g.genericRepository.CreateEntity(inputDto, entityName)
	if createEntityError != nil {
		return nil, createEntityError
	}

	afterCtx := &hooks.HookContext{
		EntityName: entityName,
		HookType:   hooks.AfterCreate,
		NewEntity:  entity,
		EntityID:   g.ExtractUuidFromReflectEntity(entity),
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

func (g *genericService) SaveEntity(entity interface{}) (interface{}, error) {

	entity, saveEntityError := g.genericRepository.SaveEntity(entity)
	if saveEntityError != nil {
		return nil, saveEntityError
	}

	return entity, nil
}

func (g *genericService) GetEntity(id uuid.UUID, data interface{}, entityName string, includes []string) (interface{}, error) {
	entity, err := g.genericRepository.GetEntity(id, data, entityName, includes)

	if err != nil {
		return nil, err
	}

	if g.ExtractUuidFromReflectEntity(entity) == uuid.Nil {
		return nil, entityErrors.NewEntityNotFound(entityName, id)
	}

	return entity, nil

}

// should return an array of dtoEntityOutput
func (g *genericService) GetEntities(data interface{}, page int, pageSize int, filters map[string]interface{}, includes []string) ([]interface{}, int64, error) {

	allPages, total, err := g.genericRepository.GetAllEntities(data, page, pageSize, filters, includes)

	if err != nil {
		return nil, 0, err
	}

	return allPages, total, nil
}

// GetEntitiesCursor retrieves entities using cursor-based pagination.
// This method delegates to the repository layer for efficient cursor-based traversal.
func (g *genericService) GetEntitiesCursor(data interface{}, cursor string, limit int, filters map[string]interface{}, includes []string) ([]interface{}, string, bool, int64, error) {

	allPages, nextCursor, hasMore, total, err := g.genericRepository.GetAllEntitiesCursor(data, cursor, limit, filters, includes)

	if err != nil {
		return nil, "", false, 0, err
	}

	return allPages, nextCursor, hasMore, total, nil
}

func (g *genericService) DeleteEntity(id uuid.UUID, entity interface{}, scoped bool) error {
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

func (g *genericService) EditEntity(id uuid.UUID, entityName string, entity interface{}, data interface{}) error {
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

func (g *genericService) GetEntityModelInterface(entityName string) interface{} {
	var result interface{}
	result, _ = ems.GlobalEntityRegistrationService.GetEntityInterface(entityName)
	return result
}

func (g *genericService) AddOwnerIDs(entity interface{}, userId string) (interface{}, error) {
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

func (g *genericService) ExtractUuidFromReflectEntity(entity interface{}) uuid.UUID {
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

func (g *genericService) GetDtoArrayFromEntitiesPages(allEntitiesPages []interface{}, entityModelInterface interface{}, entityName string) ([]interface{}, bool) {
	var entitiesDto []interface{}
	entitiesDto = []interface{}{}

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
func (g *genericService) appendEntityFromResult(entityName string, item interface{}, entitiesDto []interface{}) ([]interface{}, bool) {
	result, ko := g.GetEntityFromResult(entityName, item)
	if !ko {
		entitiesDto = append(entitiesDto, result)
		return entitiesDto, false
	}

	return nil, true
}

// used in post and get
func (g *genericService) GetEntityFromResult(entityName string, item interface{}) (interface{}, bool) {
	var result interface{}
	if funcRef, ok := ems.GlobalEntityRegistrationService.GetConversionFunction(entityName, ems.OutputModelToDto); ok {
		val := reflect.ValueOf(funcRef)

		if val.IsValid() && val.Kind() == reflect.Func {
			args := []reflect.Value{reflect.ValueOf(item)}
			entityDto := val.Call(args)

			if !entityDto[1].IsNil() {
				return nil, true
			}

			if len(entityDto) == 2 {
				result = entityDto[0].Interface()
			}

		} else {
			return nil, true
		}
	} else {
		return nil, true
	}
	return result, false
}

func (g *genericService) AddDefaultAccessesForEntity(resourceName string, entity interface{}, userId string) error {
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

func (g *genericService) DecodeInputDtoForEntityCreation(entityName string, ctx *gin.Context) (interface{}, error) {
	entityCreateDtoInput := ems.GlobalEntityRegistrationService.GetEntityDtos(entityName, ems.InputCreateDto)
	decodedData := ems.GlobalEntityRegistrationService.GetEntityDtos(entityName, ems.InputCreateDto)

	bindError := ctx.BindJSON(&entityCreateDtoInput)
	if bindError != nil {
		return nil, bindError
	}

	config := &mapstructure.DecoderConfig{
		WeaklyTypedInput: true,
		Result:           &decodedData,
	}

	decoder, err := mapstructure.NewDecoder(config)
	if err != nil {
		panic(err)
	}

	errDecode := decoder.Decode(entityCreateDtoInput)
	if errDecode != nil {
		return nil, errDecode
	}

	return decodedData, nil
}
