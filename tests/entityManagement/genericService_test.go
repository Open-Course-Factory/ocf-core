// tests/entityManagement/genericService_test.go
package entityManagement_tests

import (
	"fmt"
	"net/http/httptest"
	ems "soli/formations/src/entityManagement/entityManagementService"
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

func (m *MockGenericRepository) SaveEntity(entity any) (any, error) {
	args := m.Called(entity)
	return args.Get(0), args.Error(1)
}

func (m *MockGenericRepository) GetEntity(id uuid.UUID, data any, entityName string) (any, error) {
	args := m.Called(id, data, entityName)
	return args.Get(0), args.Error(1)
}

func (m *MockGenericRepository) GetAllEntities(data any, pageSize int) ([]any, error) {
	args := m.Called(data, pageSize)
	return args.Get(0).([]any), args.Error(1)
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
func (g *mockGenericService) CreateEntity(inputDto interface{}, entityName string) (interface{}, error) {
	return g.repository.CreateEntity(inputDto, entityName)
}

func (g *mockGenericService) SaveEntity(entity interface{}) (interface{}, error) {
	return g.repository.SaveEntity(entity)
}

func (g *mockGenericService) GetEntity(id uuid.UUID, data interface{}, entityName string) (interface{}, error) {
	return g.repository.GetEntity(id, data, entityName)
}

func (g *mockGenericService) GetEntities(data interface{}, page int, pageSize int) ([]interface{}, int64, error) {
	return g.repository.GetAllEntities(data, page, pageSize)
}

func (g *mockGenericService) DeleteEntity(id uuid.UUID, entity interface{}, scoped bool) error {
	return g.repository.DeleteEntity(id, entity, scoped)
}

func (g *mockGenericService) EditEntity(id uuid.UUID, entityName string, entity interface{}, data interface{}) error {
	return g.repository.EditEntity(id, entityName, entity, data)
}

// Implémentation des autres méthodes nécessaires (similaires à l'original mais testables)
func (g *mockGenericService) GetEntityModelInterface(entityName string) interface{} {
	result, _ := ems.GlobalEntityRegistrationService.GetEntityInterface(entityName)
	return result
}

func (g *mockGenericService) AddOwnerIDs(entity interface{}, userId string) (interface{}, error) {
	// Implémentation simplifiée pour les tests
	return entity, nil
}

func (g *mockGenericService) ExtractUuidFromReflectEntity(entity interface{}) uuid.UUID {
	// Implémentation simplifiée pour les tests
	return uuid.New()
}

func (g *mockGenericService) GetDtoArrayFromEntitiesPages(allEntitiesPages []interface{}, entityModelInterface interface{}, entityName string) ([]interface{}, bool) {
	// Implémentation simplifiée pour les tests
	return []interface{}{}, false
}

func (g *mockGenericService) GetEntityFromResult(entityName string, item interface{}) (interface{}, bool) {
	// Implémentation simplifiée pour les tests
	return item, false
}

func (g *mockGenericService) AddDefaultAccessesForEntity(resourceName string, entity interface{}, userId string) error {
	return nil
}

func (g *mockGenericService) DecodeInputDtoForEntityCreation(entityName string, ctx *gin.Context) (interface{}, error) {
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

	// Enregistrer l'entité de test
	ems.GlobalEntityRegistrationService.RegisterEntityInterface("TestEntity", TestEntityWithBaseModel{})

	converters := entityManagementInterfaces.EntityConverters{
		ModelToDto: modelToDto,
		DtoToModel: dtoToModel,
	}
	ems.GlobalEntityRegistrationService.RegisterEntityConversionFunctions("TestEntity", converters)

	dtos := map[ems.DtoPurpose]any{
		ems.InputCreateDto: TestEntityInputDto{},
		ems.OutputDto:      TestEntityOutputDto{},
		ems.InputEditDto:   TestEntityInputDto{},
	}
	ems.GlobalEntityRegistrationService.RegisterEntityDtos("TestEntity", dtos)
}

func TestGenericService_CreateEntity_Success(t *testing.T) {
	// Setup
	mockRepo := &MockGenericRepository{}
	service := newMockGenericService(mockRepo)
	setupTestEntityRegistration()

	inputDto := TestEntityInputDto{Name: "Test Name", Description: "Test Description"}
	expectedEntity := &TestEntityWithBaseModel{Name: "Test Name", Description: "Test Description"}

	// Mock expectations
	mockRepo.On("CreateEntity", inputDto, "TestEntity").Return(expectedEntity, nil)

	// Execute
	result, err := service.CreateEntity(inputDto, "TestEntity")

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
	mockRepo.On("GetEntity", entityID, entityData, "TestEntity").Return(expectedEntity, nil)

	// Execute
	result, err := service.GetEntity(entityID, entityData, "TestEntity")

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
	mockRepo.On("GetEntity", entityID, entityData, "TestEntity").Return(nil, gorm.ErrRecordNotFound)

	// Execute
	result, err := service.GetEntity(entityID, entityData, "TestEntity")

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
	mockRepo.On("GetAllEntities", entityData, 20).Return(expectedEntities, nil)

	// Execute
	result, err := service.GetEntities(entityData)

	// Assert
	assert.NoError(t, err)
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

	// Execute
	result := service.GetEntityModelInterface("TestEntity")

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
	service := services.NewGenericService(db)
	setupTestEntityRegistration()

	// Create
	inputDto := TestEntityInputDto{Name: "Integration Test", Description: "Testing with real DB"}

	createdEntity, err := service.CreateEntity(inputDto, "TestEntity")
	assert.NoError(t, err)
	assert.NotNil(t, createdEntity)

	// Extract ID for retrieval
	entityWithBaseModel, ok := createdEntity.(*TestEntityWithBaseModel)
	assert.True(t, ok)
	assert.NotEqual(t, uuid.Nil, entityWithBaseModel.ID)

	// Retrieve
	retrievedEntity, err := service.GetEntity(entityWithBaseModel.ID, TestEntityWithBaseModel{}, "TestEntity")
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
	service := services.NewGenericService(db)
	setupTestEntityRegistration()

	// CREATE
	inputDto := TestEntityInputDto{Name: "CRUD Test", Description: "Full CRUD workflow"}
	createdEntity, err := service.CreateEntity(inputDto, "TestEntity")
	assert.NoError(t, err)

	entityWithBaseModel := createdEntity.(*TestEntityWithBaseModel)
	entityID := entityWithBaseModel.ID

	// READ
	retrievedEntity, err := service.GetEntity(entityID, TestEntityWithBaseModel{}, "TestEntity")
	assert.NoError(t, err)
	assert.Equal(t, "CRUD Test", retrievedEntity.(*TestEntityWithBaseModel).Name)

	// UPDATE
	updateData := map[string]any{"name": "Updated CRUD Test"}
	err = service.EditEntity(entityID, "TestEntity", TestEntityWithBaseModel{}, updateData)
	assert.NoError(t, err)

	// Verify update
	updatedEntity, err := service.GetEntity(entityID, TestEntityWithBaseModel{}, "TestEntity")
	assert.NoError(t, err)
	assert.Equal(t, "Updated CRUD Test", updatedEntity.(*TestEntityWithBaseModel).Name)

	// DELETE
	err = service.DeleteEntity(entityID, TestEntityWithBaseModel{}, true)
	assert.NoError(t, err)

	// Verify deletion
	entity, err := service.GetEntity(entityID, TestEntityWithBaseModel{}, "TestEntity")
	assert.Error(t, err)

	fmt.Println(entity)
}

// Test de performance pour identifier les goulots d'étranglement
func BenchmarkGenericService_CreateEntity(b *testing.B) {
	db := setupTestDB(&testing.T{})
	service := services.NewGenericService(db)
	setupTestEntityRegistration()

	inputDto := TestEntityInputDto{Name: "Benchmark Test", Description: "Performance testing"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		service.CreateEntity(inputDto, "TestEntity")
	}
}

// Test de validation des données d'entrée
func TestGenericService_DecodeInputDtoForEntityCreation(t *testing.T) {
	// Setup
	service := services.NewGenericService(setupTestDB(t))
	setupTestEntityRegistration()

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
	service := services.NewGenericService(setupTestDB(t))

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
