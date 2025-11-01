// tests/entityManagement/genericService_test.go
package entityManagement_tests

import (
	"fmt"
	"net/http/httptest"
	ems "soli/formations/src/entityManagement/entityManagementService"
	"soli/formations/src/entityManagement/hooks"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/entityManagement/repositories"
	"soli/formations/src/entityManagement/services"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestEntity pour les tests avec BaseModel
type TestEntityWithBaseModel struct {
	entityManagementModels.BaseModel
	Name        string
	Description string
}

// TestEntityInput pour les tests
type TestEntityInputDto struct {
	Name        string
	Description string
}

// TestEntityOutput pour les tests
type TestEntityOutputDto struct {
	ID          string
	Name        string
	Description string
	OwnerIDs    []string
}

// Mock Repository pour isoler les tests du GenericService
type MockGenericRepository struct {
	mock.Mock
}

func (m *MockGenericRepository) CreateEntity(data any, entityName string) (any, error) {
	args := m.Called(data, entityName)
	return args.Get(0), args.Error(1)
}

func (m *MockGenericRepository) CreateEntityFromModel(entityModel any) (any, error) {
	args := m.Called(entityModel)
	return args.Get(0), args.Error(1)
}

func (m *MockGenericRepository) SaveEntity(entity any) (any, error) {
	args := m.Called(entity)
	return args.Get(0), args.Error(1)
}

func (m *MockGenericRepository) GetEntity(id uuid.UUID, data any, entityName string, includes []string) (any, error) {
	args := m.Called(id, data, entityName, includes)
	return args.Get(0), args.Error(1)
}

func (m *MockGenericRepository) GetAllEntities(data any, page int, pageSize int, filters map[string]any, includes []string) ([]any, int64, error) {
	args := m.Called(data, page, pageSize, filters, includes)
	return args.Get(0).([]any), args.Get(1).(int64), args.Error(2)
}

func (m *MockGenericRepository) GetAllEntitiesCursor(data any, cursor string, limit int, filters map[string]any, includes []string) ([]any, string, bool, int64, error) {
	args := m.Called(data, cursor, limit, filters, includes)
	return args.Get(0).([]any), args.Get(1).(string), args.Get(2).(bool), args.Get(3).(int64), args.Error(4)
}

func (m *MockGenericRepository) EditEntity(id uuid.UUID, entityName string, entity any, data any) error {
	args := m.Called(id, entityName, entity, data)
	return args.Error(0)
}

func (m *MockGenericRepository) DeleteEntity(id uuid.UUID, entity any, scoped bool) error {
	args := m.Called(id, entity, scoped)
	return args.Error(0)
}

// Mock GenericService pour pouvoir injecter le mock repository
type mockGenericService struct {
	repository repositories.GenericRepository
}

func newMockGenericService(repo repositories.GenericRepository) services.GenericService {
	return &mockGenericService{
		repository: repo,
	}
}

// Implémentation des méthodes du GenericService
func (g *mockGenericService) CreateEntity(inputDto any, entityName string) (any, error) {
	return g.repository.CreateEntity(inputDto, entityName)
}

func (g *mockGenericService) CreateEntityWithUser(inputDto any, entityName string, userID string) (any, error) {
	return g.repository.CreateEntity(inputDto, entityName)
}

func (g *mockGenericService) SaveEntity(entity any) (any, error) {
	return g.repository.SaveEntity(entity)
}

func (g *mockGenericService) GetEntity(id uuid.UUID, data any, entityName string, includes []string) (any, error) {
	return g.repository.GetEntity(id, data, entityName, includes)
}

func (g *mockGenericService) GetEntities(data any, page int, pageSize int, filters map[string]any, includes []string) ([]any, int64, error) {
	return g.repository.GetAllEntities(data, page, pageSize, filters, includes)
}

func (g *mockGenericService) GetEntitiesCursor(data any, cursor string, limit int, filters map[string]any, includes []string) ([]any, string, bool, int64, error) {
	return g.repository.GetAllEntitiesCursor(data, cursor, limit, filters, includes)
}

func (g *mockGenericService) DeleteEntity(id uuid.UUID, entity any, scoped bool) error {
	return g.repository.DeleteEntity(id, entity, scoped)
}

func (g *mockGenericService) EditEntity(id uuid.UUID, entityName string, entity any, data any) error {
	return g.repository.EditEntity(id, entityName, entity, data)
}

// Implémentation des autres méthodes nécessaires (similaires à l'original mais testables)
func (g *mockGenericService) GetEntityModelInterface(entityName string) any {
	result, _ := ems.GlobalEntityRegistrationService.GetEntityInterface(entityName)
	return result
}

func (g *mockGenericService) AddOwnerIDs(entity any, userId string) (any, error) {
	// Implémentation simplifiée pour les tests
	return entity, nil
}

func (g *mockGenericService) ExtractUuidFromReflectEntity(entity any) uuid.UUID {
	// Implémentation simplifiée pour les tests
	return uuid.New()
}

func (g *mockGenericService) GetDtoArrayFromEntitiesPages(allEntitiesPages []any, entityModelInterface any, entityName string) ([]any, bool) {
	// Implémentation simplifiée pour les tests
	return []any{}, false
}

func (g *mockGenericService) GetEntityFromResult(entityName string, item any) (any, bool) {
	// Implémentation simplifiée pour les tests
	return item, false
}

func (g *mockGenericService) AddDefaultAccessesForEntity(resourceName string, entity any, userId string) error {
	return nil
}

func (g *mockGenericService) DecodeInputDtoForEntityCreation(entityName string, ctx *gin.Context) (any, error) {
	// Implémentation simplifiée pour les tests
	return TestEntityInputDto{}, nil
}

// Setup de test avec une vraie base de données SQLite en mémoire
func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to connect to test database: %v", err)
	}

	// Migrer les modèles de test
	err = db.AutoMigrate(&TestEntityWithBaseModel{})
	if err != nil {
		t.Fatalf("Failed to migrate test models: %v", err)
	}

	return db
}

// Setup du service de registration avec les entités de test
func setupTestEntityRegistration() {
	// Reset global service to ensure clean state
	ems.GlobalEntityRegistrationService = ems.NewEntityRegistrationService()

	// Disable hooks for tests to prevent async issues and timing problems
	hooks.GlobalHookRegistry.DisableAllHooks(true)

	// Mock conversion functions
	modelToDto := func(input any) (any, error) {
		if entity, ok := input.(TestEntityWithBaseModel); ok {
			return TestEntityOutputDto{
				ID:          entity.ID.String(),
				Name:        entity.Name,
				Description: entity.Description,
				OwnerIDs:    entity.OwnerIDs,
			}, nil
		}
		return nil, assert.AnError
	}

	dtoToModel := func(input any) any {
		if dto, ok := input.(TestEntityInputDto); ok {
			return &TestEntityWithBaseModel{
				Name:        dto.Name,
				Description: dto.Description,
			}
		}
		return nil
	}

	// Enregistrer l'entité de test using the struct name (to match the pattern in RegisterEntity)
	entityName := "TestEntityWithBaseModel"
	ems.GlobalEntityRegistrationService.RegisterEntityInterface(entityName, TestEntityWithBaseModel{})

	converters := entityManagementInterfaces.EntityConverters{
		ModelToDto: modelToDto,
		DtoToModel: dtoToModel,
	}
	ems.GlobalEntityRegistrationService.RegisterEntityConversionFunctions(entityName, converters)

	dtos := map[ems.DtoPurpose]any{
		ems.InputCreateDto: TestEntityInputDto{},
		ems.OutputDto:      TestEntityOutputDto{},
		ems.InputEditDto:   TestEntityInputDto{},
	}
	ems.GlobalEntityRegistrationService.RegisterEntityDtos(entityName, dtos)
}

// cleanupTestEntityRegistration removes the test entity registration to prevent state pollution
func cleanupTestEntityRegistration() {
	ems.GlobalEntityRegistrationService.UnregisterEntity("TestEntityWithBaseModel")
}

func TestGenericService_CreateEntity_Success(t *testing.T) {
	// Setup
	mockRepo := &MockGenericRepository{}
	service := newMockGenericService(mockRepo)
	setupTestEntityRegistration()
	defer cleanupTestEntityRegistration()

	inputDto := TestEntityInputDto{Name: "Test Name", Description: "Test Description"}
	expectedEntity := &TestEntityWithBaseModel{Name: "Test Name", Description: "Test Description"}

	// Mock expectations
	mockRepo.On("CreateEntity", inputDto, "TestEntityWithBaseModel").Return(expectedEntity, nil)

	// Execute
	result, err := service.CreateEntity(inputDto, "TestEntityWithBaseModel")

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, expectedEntity, result)
	mockRepo.AssertExpectations(t)
}

func TestGenericService_CreateEntity_RepositoryError(t *testing.T) {
	// Setup
	mockRepo := &MockGenericRepository{}
	service := newMockGenericService(mockRepo)

	inputDto := TestEntityInputDto{Name: "Test Name"}

	// Mock expectations
	mockRepo.On("CreateEntity", inputDto, "TestEntity").Return(nil, assert.AnError)

	// Execute
	result, err := service.CreateEntity(inputDto, "TestEntity")

	// Assert
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, assert.AnError, err)
	mockRepo.AssertExpectations(t)
}

func TestGenericService_SaveEntity_Success(t *testing.T) {
	// Setup
	mockRepo := &MockGenericRepository{}
	service := newMockGenericService(mockRepo)

	entity := &TestEntityWithBaseModel{Name: "Test Name"}
	savedEntity := &TestEntityWithBaseModel{Name: "Test Name"}

	// Mock expectations
	mockRepo.On("SaveEntity", entity).Return(savedEntity, nil)

	// Execute
	result, err := service.SaveEntity(entity)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, savedEntity, result)
	mockRepo.AssertExpectations(t)
}

func TestGenericService_GetEntity_Success(t *testing.T) {
	// Setup
	mockRepo := &MockGenericRepository{}
	service := newMockGenericService(mockRepo)

	entityID := uuid.New()
	entityData := TestEntityWithBaseModel{}
	expectedEntity := &TestEntityWithBaseModel{Name: "Retrieved Entity"}

	// Mock expectations
	mockRepo.On("GetEntity", entityID, entityData, "TestEntity", mock.Anything).Return(expectedEntity, nil)

	// Execute
	result, err := service.GetEntity(entityID, entityData, "TestEntity", nil)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, expectedEntity, result)
	mockRepo.AssertExpectations(t)
}

func TestGenericService_GetEntity_NotFound(t *testing.T) {
	// Setup
	mockRepo := &MockGenericRepository{}
	service := newMockGenericService(mockRepo)

	entityID := uuid.New()
	entityData := TestEntityWithBaseModel{}

	// Mock expectations
	mockRepo.On("GetEntity", entityID, entityData, "TestEntity", mock.Anything).Return(nil, gorm.ErrRecordNotFound)

	// Execute
	result, err := service.GetEntity(entityID, entityData, "TestEntity", nil)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, gorm.ErrRecordNotFound, err)
	mockRepo.AssertExpectations(t)
}

func TestGenericService_GetEntities_Success(t *testing.T) {
	// Setup
	mockRepo := &MockGenericRepository{}
	service := newMockGenericService(mockRepo)

	entityData := TestEntityWithBaseModel{}
	expectedEntities := []any{
		&TestEntityWithBaseModel{Name: "Entity 1"},
		&TestEntityWithBaseModel{Name: "Entity 2"},
	}

	// Mock expectations
	mockRepo.On("GetAllEntities", entityData, 1, 20, map[string]any{}, mock.Anything).Return(expectedEntities, int64(2), nil)

	// Execute
	result, total, err := service.GetEntities(entityData, 1, 20, map[string]any{}, nil)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Equal(t, expectedEntities, result)
	mockRepo.AssertExpectations(t)
}

func TestGenericService_DeleteEntity_Success(t *testing.T) {
	// Setup
	mockRepo := &MockGenericRepository{}
	service := newMockGenericService(mockRepo)

	entityID := uuid.New()
	entity := &TestEntityWithBaseModel{}
	scoped := true

	// Mock expectations
	mockRepo.On("DeleteEntity", entityID, entity, scoped).Return(nil)

	// Execute
	err := service.DeleteEntity(entityID, entity, scoped)

	// Assert
	assert.NoError(t, err)
	mockRepo.AssertExpectations(t)
}

func TestGenericService_EditEntity_Success(t *testing.T) {
	// Setup
	mockRepo := &MockGenericRepository{}
	service := newMockGenericService(mockRepo)

	entityID := uuid.New()
	entity := &TestEntityWithBaseModel{}
	updateData := map[string]any{"name": "Updated Name"}

	// Mock expectations
	mockRepo.On("EditEntity", entityID, "TestEntity", entity, updateData).Return(nil)

	// Execute
	err := service.EditEntity(entityID, "TestEntity", entity, updateData)

	// Assert
	assert.NoError(t, err)
	mockRepo.AssertExpectations(t)
}

func TestGenericService_GetEntityModelInterface(t *testing.T) {
	// Setup
	service := newMockGenericService(&MockGenericRepository{})
	setupTestEntityRegistration()
	defer cleanupTestEntityRegistration()

	// Execute
	result := service.GetEntityModelInterface("TestEntityWithBaseModel")

	// Assert
	assert.Equal(t, TestEntityWithBaseModel{}, result)
}

func TestGenericService_GetEntityModelInterface_NonExistent(t *testing.T) {
	// Setup
	service := newMockGenericService(&MockGenericRepository{})

	// Execute
	result := service.GetEntityModelInterface("NonExistentEntity")

	// Assert
	assert.Nil(t, result)
}

// Test d'intégration avec une vraie base de données
func TestGenericService_Integration_CreateAndRetrieve(t *testing.T) {
	// Setup
	db := setupTestDB(t)
	service := services.NewGenericService(db, nil)
	setupTestEntityRegistration()
	defer cleanupTestEntityRegistration()

	// Create
	inputDto := TestEntityInputDto{Name: "Integration Test", Description: "Testing with real DB"}

	createdEntity, err := service.CreateEntity(inputDto, "TestEntityWithBaseModel")
	assert.NoError(t, err)
	assert.NotNil(t, createdEntity)

	// Extract ID for retrieval
	entityWithBaseModel, ok := createdEntity.(*TestEntityWithBaseModel)
	assert.True(t, ok)
	assert.NotEqual(t, uuid.Nil, entityWithBaseModel.ID)

	// Retrieve
	retrievedEntity, err := service.GetEntity(entityWithBaseModel.ID, TestEntityWithBaseModel{}, "TestEntityWithBaseModel", nil)
	assert.NoError(t, err)
	assert.NotNil(t, retrievedEntity)

	// Verify
	retrievedEntityTyped, ok := retrievedEntity.(*TestEntityWithBaseModel)
	assert.True(t, ok)
	assert.Equal(t, "Integration Test", retrievedEntityTyped.Name)
	assert.Equal(t, "Testing with real DB", retrievedEntityTyped.Description)
}

// Test d'intégration pour le workflow complet CRUD
func TestGenericService_Integration_FullCRUD(t *testing.T) {
	// Setup
	db := setupTestDB(t)
	service := services.NewGenericService(db, nil)
	setupTestEntityRegistration()
	defer cleanupTestEntityRegistration()

	// CREATE
	inputDto := TestEntityInputDto{Name: "CRUD Test", Description: "Full CRUD workflow"}
	createdEntity, err := service.CreateEntity(inputDto, "TestEntityWithBaseModel")
	assert.NoError(t, err)

	entityWithBaseModel := createdEntity.(*TestEntityWithBaseModel)
	entityID := entityWithBaseModel.ID

	// READ
	retrievedEntity, err := service.GetEntity(entityID, TestEntityWithBaseModel{}, "TestEntityWithBaseModel", nil)
	assert.NoError(t, err)
	assert.Equal(t, "CRUD Test", retrievedEntity.(*TestEntityWithBaseModel).Name)

	// UPDATE
	updateData := map[string]any{"name": "Updated CRUD Test"}
	err = service.EditEntity(entityID, "TestEntityWithBaseModel", TestEntityWithBaseModel{}, updateData)
	assert.NoError(t, err)

	// Verify update
	updatedEntity, err := service.GetEntity(entityID, TestEntityWithBaseModel{}, "TestEntityWithBaseModel", nil)
	assert.NoError(t, err)
	assert.Equal(t, "Updated CRUD Test", updatedEntity.(*TestEntityWithBaseModel).Name)

	// DELETE
	entityInterface := service.GetEntityModelInterface("TestEntityWithBaseModel")
	assert.NotNil(t, entityInterface, "Entity interface should not be nil - entity not registered?")
	if entityInterface != nil {
		err = service.DeleteEntity(entityID, entityInterface, true)
		assert.NoError(t, err)
	}

	// Verify deletion
	entity, err := service.GetEntity(entityID, TestEntityWithBaseModel{}, "TestEntityWithBaseModel", nil)
	assert.Error(t, err)

	fmt.Println(entity)
}

// Test de performance pour identifier les goulots d'étranglement
func BenchmarkGenericService_CreateEntity(b *testing.B) {
	db := setupTestDB(&testing.T{})
	service := services.NewGenericService(db, nil)
	setupTestEntityRegistration()
	defer cleanupTestEntityRegistration()

	inputDto := TestEntityInputDto{Name: "Benchmark Test", Description: "Performance testing"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = service.CreateEntity(inputDto, "TestEntityWithBaseModel")
	}
}

// Test de validation des données d'entrée
func TestGenericService_DecodeInputDtoForEntityCreation(t *testing.T) {
	// Setup
	service := services.NewGenericService(setupTestDB(t), nil)
	setupTestEntityRegistration()
	defer cleanupTestEntityRegistration()

	// Create test context with JSON body
	gin.SetMode(gin.TestMode)
	jsonBody := `{"name": "Test Name", "description": "Test Description"}`
	req := httptest.NewRequest("POST", "/test", strings.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = req

	// Execute
	result, err := service.DecodeInputDtoForEntityCreation("TestEntity", ctx)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, result)

	// Note: Le résultat exact dépend de l'implémentation de mapstructure
	// Ce test vérifie principalement que la méthode ne panique pas
}

// Test d'erreur pour DTO invalide
func TestGenericService_DecodeInputDtoForEntityCreation_InvalidJSON(t *testing.T) {
	// Setup
	service := services.NewGenericService(setupTestDB(t), nil)

	// Create test context with invalid JSON
	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest("POST", "/test", strings.NewReader(`{invalid json`))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = req

	// Execute
	result, err := service.DecodeInputDtoForEntityCreation("TestEntity", ctx)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, result)
}
