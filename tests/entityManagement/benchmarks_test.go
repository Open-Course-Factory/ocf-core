// tests/entityManagement/benchmarks_test.go
package entityManagement_tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"soli/formations/src/auth/casdoor"
	authInterfaces "soli/formations/src/auth/interfaces"
	authMocks "soli/formations/src/auth/mocks"
	authModels "soli/formations/src/auth/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	entityManagementModels "soli/formations/src/entityManagement/models"
	controller "soli/formations/src/entityManagement/routes"
)

// ============================================================================
// Benchmark Entities
// ============================================================================

type BenchmarkEntity struct {
	entityManagementModels.BaseModel
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Value       int            `json:"value"`
	IsActive    bool           `json:"is_active"`
	Tags        pq.StringArray `gorm:"type:text[]" json:"tags"`
	Data        string         `json:"data"` // Large text field
}

type BenchmarkEntityInput struct {
	Name        string   `json:"name" binding:"required"`
	Description string   `json:"description"`
	Value       int      `json:"value"`
	IsActive    bool     `json:"is_active"`
	Tags        []string `json:"tags"`
	Data        string   `json:"data"`
	OwnerID     string   `json:"owner_id"`
}

type BenchmarkEntityOutput struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Value       int       `json:"value"`
	IsActive    bool      `json:"is_active"`
	Tags        []string  `json:"tags"`
	Data        string    `json:"data"`
	OwnerIDs    []string  `json:"owner_ids"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type BenchmarkEntityRegistration struct {
	entityManagementInterfaces.AbstractRegistrableInterface
}

func (r BenchmarkEntityRegistration) EntityModelToEntityOutput(input any) (any, error) {
	var entity BenchmarkEntity
	switch v := input.(type) {
	case *BenchmarkEntity:
		entity = *v
	case BenchmarkEntity:
		entity = v
	default:
		return nil, fmt.Errorf("invalid input type")
	}

	return &BenchmarkEntityOutput{
		ID:          entity.ID.String(),
		Name:        entity.Name,
		Description: entity.Description,
		Value:       entity.Value,
		IsActive:    entity.IsActive,
		Tags:        entity.Tags,
		Data:        entity.Data,
		OwnerIDs:    entity.OwnerIDs,
		CreatedAt:   entity.CreatedAt,
		UpdatedAt:   entity.UpdatedAt,
	}, nil
}

func (r BenchmarkEntityRegistration) EntityInputDtoToEntityModel(input any) any {
	var dto BenchmarkEntityInput
	switch v := input.(type) {
	case *BenchmarkEntityInput:
		dto = *v
	case BenchmarkEntityInput:
		dto = v
	default:
		return nil
	}

	entity := &BenchmarkEntity{
		Name:        dto.Name,
		Description: dto.Description,
		Value:       dto.Value,
		IsActive:    dto.IsActive,
		Tags:        dto.Tags,
		Data:        dto.Data,
	}
	entity.OwnerIDs = append(entity.OwnerIDs, dto.OwnerID)

	return entity
}

func (r BenchmarkEntityRegistration) GetEntityRegistrationInput() entityManagementInterfaces.EntityRegistrationInput {
	return entityManagementInterfaces.EntityRegistrationInput{
		EntityInterface: BenchmarkEntity{},
		EntityConverters: entityManagementInterfaces.EntityConverters{
			ModelToDto: r.EntityModelToEntityOutput,
			DtoToModel: r.EntityInputDtoToEntityModel,
		},
		EntityDtos: entityManagementInterfaces.EntityDtos{
			InputCreateDto: BenchmarkEntityInput{},
			OutputDto:      BenchmarkEntityOutput{},
			InputEditDto:   map[string]interface{}{},
		},
	}
}

func (r BenchmarkEntityRegistration) GetEntityRoles() entityManagementInterfaces.EntityRoles {
	roleMap := make(map[string]string)
	roleMap[string(authModels.Member)] = "(" + http.MethodGet + "|" + http.MethodPost + ")"
	return entityManagementInterfaces.EntityRoles{Roles: roleMap}
}

// ============================================================================
// Benchmark Suite Setup
// ============================================================================

type BenchmarkSuite struct {
	db               *gorm.DB
	router           *gin.Engine
	mockEnforcer     *authMocks.MockEnforcer
	controller       controller.GenericController
	originalEnforcer authInterfaces.EnforcerInterface
}

func setupBenchmarkSuite(b *testing.B) *BenchmarkSuite {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(b, err)

	err = db.AutoMigrate(&BenchmarkEntity{})
	require.NoError(b, err)

	mockEnforcer := authMocks.NewMockEnforcer()
	mockEnforcer.LoadPolicyFunc = func() error { return nil }
	mockEnforcer.AddPolicyFunc = func(params ...interface{}) (bool, error) { return true, nil }

	suite := &BenchmarkSuite{
		db:               db,
		mockEnforcer:     mockEnforcer,
		originalEnforcer: casdoor.Enforcer,
	}
	casdoor.Enforcer = mockEnforcer

	gin.SetMode(gin.TestMode)
	router := gin.New()
	suite.router = router

	registrationService := ems.NewEntityRegistrationService()
	testRegistration := BenchmarkEntityRegistration{}
	registrationService.RegisterEntity(testRegistration)

	originalGlobal := ems.GlobalEntityRegistrationService
	ems.GlobalEntityRegistrationService = registrationService
	b.Cleanup(func() {
		ems.GlobalEntityRegistrationService = originalGlobal
		casdoor.Enforcer = suite.originalEnforcer
	})

	suite.controller = controller.NewGenericController(db)

	apiGroup := router.Group("/api/v1")
	apiGroup.POST("/benchmark-entities", suite.controller.AddEntity)
	apiGroup.GET("/benchmark-entities", suite.controller.GetEntities)
	apiGroup.GET("/benchmark-entities/:id", suite.controller.GetEntity)
	apiGroup.PATCH("/benchmark-entities/:id", suite.controller.EditEntity)
	apiGroup.DELETE("/benchmark-entities/:id", func(ctx *gin.Context) {
		suite.controller.DeleteEntity(ctx, true)
	})

	return suite
}

// ============================================================================
// CRUD Operation Benchmarks
// ============================================================================

func BenchmarkCreate_Small(b *testing.B) {
	suite := setupBenchmarkSuite(b)
	userID := "bench-user"

	input := BenchmarkEntityInput{
		Name:        "Benchmark Entity",
		Description: "Short description",
		Value:       42,
		IsActive:    true,
		Tags:        []string{"tag1", "tag2"},
		Data:        "Small data",
		OwnerID:     userID,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		input.Name = fmt.Sprintf("Entity %d", i)
		body, _ := json.Marshal(input)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/benchmark-entities", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		ctx, _ := gin.CreateTestContext(w)
		ctx.Request = req
		ctx.Set("userId", userID)

		suite.controller.AddEntity(ctx)
	}

	b.StopTimer()
	b.Logf("Created %d entities", b.N)
	b.Logf("LoadPolicy called %d times", suite.mockEnforcer.GetLoadPolicyCallCount())
}

func BenchmarkCreate_Large(b *testing.B) {
	suite := setupBenchmarkSuite(b)
	userID := "bench-user"

	largeData := make([]byte, 10*1024) // 10KB
	for i := range largeData {
		largeData[i] = byte('a' + (i % 26))
	}

	input := BenchmarkEntityInput{
		Name:        "Large Entity",
		Description: "Entity with large data field",
		Value:       999,
		IsActive:    true,
		Tags:        []string{"large", "data", "test", "benchmark"},
		Data:        string(largeData),
		OwnerID:     userID,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		input.Name = fmt.Sprintf("Large Entity %d", i)
		body, _ := json.Marshal(input)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/benchmark-entities", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		ctx, _ := gin.CreateTestContext(w)
		ctx.Request = req
		ctx.Set("userId", userID)

		suite.controller.AddEntity(ctx)
	}

	b.StopTimer()
	b.Logf("Created %d large entities (10KB each)", b.N)
}

func BenchmarkRead_Single(b *testing.B) {
	suite := setupBenchmarkSuite(b)
	userID := "bench-user"

	// Create one entity
	input := BenchmarkEntityInput{
		Name:    "Test Entity",
		Value:   42,
		OwnerID: userID,
	}
	body, _ := json.Marshal(input)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/benchmark-entities", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = req
	ctx.Set("userId", userID)
	suite.controller.AddEntity(ctx)

	var created BenchmarkEntityOutput
	json.Unmarshal(w.Body.Bytes(), &created)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/benchmark-entities/"+created.ID, nil)
		w := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(w)
		ctx.Request = req
		ctx.Params = gin.Params{{Key: "id", Value: created.ID}}

		suite.controller.GetEntity(ctx)
	}
}

func BenchmarkRead_List_10(b *testing.B)   { benchmarkReadList(b, 10) }
func BenchmarkRead_List_100(b *testing.B)  { benchmarkReadList(b, 100) }
func BenchmarkRead_List_1000(b *testing.B) { benchmarkReadList(b, 1000) }

func benchmarkReadList(b *testing.B, count int) {
	suite := setupBenchmarkSuite(b)
	userID := "bench-user"

	// Create entities
	for i := 0; i < count; i++ {
		entity := BenchmarkEntity{
			Name:     fmt.Sprintf("Entity %d", i),
			Value:    i,
			IsActive: i%2 == 0,
		}
		entity.OwnerIDs = []string{userID}
		suite.db.Create(&entity)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/benchmark-entities?page=1&size=20", nil)
		w := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(w)
		ctx.Request = req

		suite.controller.GetEntities(ctx)
	}

	b.StopTimer()
	b.Logf("Read from %d total entities", count)
}

func BenchmarkUpdate(b *testing.B) {
	suite := setupBenchmarkSuite(b)
	userID := "bench-user"

	// Create entity
	entity := BenchmarkEntity{Name: "Original", Value: 1}
	entity.OwnerIDs = []string{userID}
	suite.db.Create(&entity)

	update := map[string]interface{}{
		"name":  "Updated",
		"value": 999,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		update["value"] = i
		body, _ := json.Marshal(update)
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/benchmark-entities/"+entity.ID.String(), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(w)
		ctx.Request = req
		ctx.Params = gin.Params{{Key: "id", Value: entity.ID.String()}}

		suite.controller.EditEntity(ctx)
	}
}

func BenchmarkDelete(b *testing.B) {
	suite := setupBenchmarkSuite(b)
	userID := "bench-user"

	// Pre-create entities
	entities := make([]BenchmarkEntity, b.N)
	for i := 0; i < b.N; i++ {
		entities[i] = BenchmarkEntity{Name: fmt.Sprintf("ToDelete %d", i), Value: i}
		entities[i].OwnerIDs = []string{userID}
		suite.db.Create(&entities[i])
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/benchmark-entities/"+entities[i].ID.String(), nil)
		w := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(w)
		ctx.Request = req
		ctx.Params = gin.Params{{Key: "id", Value: entities[i].ID.String()}}

		suite.controller.DeleteEntity(ctx, true)
	}

	b.StopTimer()
	b.Logf("LoadPolicy called %d times during delete", suite.mockEnforcer.GetLoadPolicyCallCount())
}

// ============================================================================
// Filtering and Query Benchmarks
// ============================================================================

func BenchmarkFilter_ByName(b *testing.B) {
	suite := setupBenchmarkSuite(b)
	userID := "bench-user"

	// Create test data
	for i := 0; i < 1000; i++ {
		entity := BenchmarkEntity{
			Name:     fmt.Sprintf("Entity %d", i),
			Value:    i,
			IsActive: i%2 == 0,
		}
		entity.OwnerIDs = []string{userID}
		suite.db.Create(&entity)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		searchName := fmt.Sprintf("Entity %d", i%1000)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/benchmark-entities?name="+searchName+"&page=1&size=20", nil)
		w := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(w)
		ctx.Request = req

		suite.controller.GetEntities(ctx)
	}
}

func BenchmarkFilter_ByBoolean(b *testing.B) {
	suite := setupBenchmarkSuite(b)
	userID := "bench-user"

	for i := 0; i < 1000; i++ {
		entity := BenchmarkEntity{
			Name:     fmt.Sprintf("Entity %d", i),
			Value:    i,
			IsActive: i%2 == 0,
		}
		entity.OwnerIDs = []string{userID}
		suite.db.Create(&entity)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/benchmark-entities?isActive=true&page=1&size=20", nil)
		w := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(w)
		ctx.Request = req

		suite.controller.GetEntities(ctx)
	}
}

func BenchmarkPagination_Page1(b *testing.B)   { benchmarkPagination(b, 1) }
func BenchmarkPagination_Page50(b *testing.B)  { benchmarkPagination(b, 50) }
func BenchmarkPagination_Page100(b *testing.B) { benchmarkPagination(b, 100) }

func benchmarkPagination(b *testing.B, page int) {
	suite := setupBenchmarkSuite(b)
	userID := "bench-user"

	// Create 2000 entities
	for i := 0; i < 2000; i++ {
		entity := BenchmarkEntity{
			Name:  fmt.Sprintf("Entity %d", i),
			Value: i,
		}
		entity.OwnerIDs = []string{userID}
		suite.db.Create(&entity)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		url := fmt.Sprintf("/api/v1/benchmark-entities?page=%d&size=20", page)
		req := httptest.NewRequest(http.MethodGet, url, nil)
		w := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(w)
		ctx.Request = req

		suite.controller.GetEntities(ctx)
	}
}

// ============================================================================
// Security/Permission Benchmarks
// ============================================================================

func BenchmarkSecurity_LoadPolicy_OnCreate(b *testing.B) {
	suite := setupBenchmarkSuite(b)
	userID := "bench-user"

	loadPolicyCount := 0
	suite.mockEnforcer.LoadPolicyFunc = func() error {
		loadPolicyCount++
		return nil
	}

	input := BenchmarkEntityInput{
		Name:    "Security Test",
		Value:   42,
		OwnerID: userID,
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		input.Name = fmt.Sprintf("Entity %d", i)
		body, _ := json.Marshal(input)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/benchmark-entities", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(w)
		ctx.Request = req
		ctx.Set("userId", userID)

		suite.controller.AddEntity(ctx)
	}

	b.StopTimer()
	b.Logf("⚠️  LoadPolicy called %d times for %d creates (%.2f per create)",
		loadPolicyCount, b.N, float64(loadPolicyCount)/float64(b.N))
}

func BenchmarkSecurity_AddPolicy_OnCreate(b *testing.B) {
	suite := setupBenchmarkSuite(b)
	userID := "bench-user"

	addPolicyCount := 0
	suite.mockEnforcer.AddPolicyFunc = func(params ...interface{}) (bool, error) {
		addPolicyCount++
		return true, nil
	}

	input := BenchmarkEntityInput{
		Name:    "Security Test",
		Value:   42,
		OwnerID: userID,
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		input.Name = fmt.Sprintf("Entity %d", i)
		body, _ := json.Marshal(input)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/benchmark-entities", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(w)
		ctx.Request = req
		ctx.Set("userId", userID)

		suite.controller.AddEntity(ctx)
	}

	b.StopTimer()
	b.Logf("AddPolicy called %d times for %d creates (%.2f per create)",
		addPolicyCount, b.N, float64(addPolicyCount)/float64(b.N))
}

// ============================================================================
// Reflection and Conversion Benchmarks
// ============================================================================

func BenchmarkConversion_DtoToModel(b *testing.B) {
	registration := BenchmarkEntityRegistration{}

	input := BenchmarkEntityInput{
		Name:        "Test Entity",
		Description: "Test Description",
		Value:       42,
		IsActive:    true,
		Tags:        []string{"tag1", "tag2", "tag3"},
		Data:        "Some data here",
		OwnerID:     "user-123",
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = registration.EntityInputDtoToEntityModel(input)
	}
}

func BenchmarkConversion_ModelToDto(b *testing.B) {
	registration := BenchmarkEntityRegistration{}

	entity := BenchmarkEntity{
		Name:        "Test Entity",
		Description: "Test Description",
		Value:       42,
		IsActive:    true,
		Tags:        []string{"tag1", "tag2", "tag3"},
		Data:        "Some data here",
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = registration.EntityModelToEntityOutput(entity)
	}
}

// ============================================================================
// Memory and Allocation Benchmarks
// ============================================================================

func BenchmarkMemory_CreateWithPreloading(b *testing.B) {
	suite := setupBenchmarkSuite(b)
	userID := "bench-user"

	// Create entity with relationships
	input := BenchmarkEntityInput{
		Name:    "Entity with relationships",
		Value:   42,
		OwnerID: userID,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		input.Name = fmt.Sprintf("Entity %d", i)
		body, _ := json.Marshal(input)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/benchmark-entities", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(w)
		ctx.Request = req
		ctx.Set("userId", userID)

		suite.controller.AddEntity(ctx)
	}
}
