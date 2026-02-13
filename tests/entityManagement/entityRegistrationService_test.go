// tests/entityManagement/entityRegistrationService_test.go
package entityManagement_tests

import (
	"fmt"
	"net/http"
	"testing"

	"soli/formations/src/auth/casdoor"
	authMocks "soli/formations/src/auth/mocks"
	"soli/formations/src/auth/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockEnforcerForTests est un mock simple pour les tests unitaires
type MockEnforcerForTests struct {
	mock.Mock
}

func (m *MockEnforcerForTests) LoadPolicy() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockEnforcerForTests) AddPolicy(params ...any) (bool, error) {
	args := m.Called(params...)
	return args.Bool(0), args.Error(1)
}

func (m *MockEnforcerForTests) Enforce(rvals ...any) (bool, error) {
	args := m.Called(rvals...)
	return args.Bool(0), args.Error(1)
}

func (m *MockEnforcerForTests) RemovePolicy(params ...any) (bool, error) {
	args := m.Called(params...)
	return args.Bool(0), args.Error(1)
}

func (m *MockEnforcerForTests) RemoveFilteredPolicy(fieldIndex int, fieldValues ...string) (bool, error) {
	args := m.Called(fieldIndex, fieldValues)
	return args.Bool(0), args.Error(1)
}

func (m *MockEnforcerForTests) GetRolesForUser(name string) ([]string, error) {
	args := m.Called(name)
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockEnforcerForTests) GetImplicitPermissionsForUser(name string) ([][]string, error) {
	args := m.Called(name)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([][]string), args.Error(1)
}

func (m *MockEnforcerForTests) GetFilteredPolicy(fieldIndex int, fieldValues ...string) ([][]string, error) {
	args := m.Called(fieldIndex, fieldValues)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([][]string), args.Error(1)
}

func (m *MockEnforcerForTests) GetPolicy() ([][]string, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([][]string), args.Error(1)
}

func (m *MockEnforcerForTests) AddGroupingPolicy(params ...any) (bool, error) {
	args := m.Called(params...)
	return args.Bool(0), args.Error(1)
}

func (m *MockEnforcerForTests) RemoveGroupingPolicy(params ...any) (bool, error) {
	args := m.Called(params...)
	return args.Bool(0), args.Error(1)
}

// TestEntity pour les tests
type TestEntity struct {
	ID   string
	Name string
}

// TestEntityInput pour les tests
type TestEntityInput struct {
	Name string
}

// TestEntityOutput pour les tests
type TestEntityOutput struct {
	ID   string
	Name string
}

func TestEntityRegistrationService_NewEntityRegistrationService(t *testing.T) {
	service := ems.NewEntityRegistrationService()

	assert.NotNil(t, service)
	// Vérifier que les maps sont initialisées
	entityType, exists := service.GetEntityInterface("NonExistent")
	assert.Nil(t, entityType)
	assert.False(t, exists)
}

func TestEntityRegistrationService_RegisterEntityInterface(t *testing.T) {
	service := ems.NewEntityRegistrationService()
	testEntity := TestEntity{}

	service.RegisterEntityInterface("TestEntity", testEntity)

	retrievedEntity, exists := service.GetEntityInterface("TestEntity")
	assert.True(t, exists)
	assert.Equal(t, testEntity, retrievedEntity)
}

func TestEntityRegistrationService_RegisterSubEntites(t *testing.T) {
	service := ems.NewEntityRegistrationService()
	subEntities := []any{TestEntity{}}

	service.RegisterSubEntites("ParentEntity", subEntities)

	retrievedSubEntities := service.GetSubEntites("ParentEntity")
	assert.Equal(t, subEntities, retrievedSubEntities)
}

func TestEntityRegistrationService_SetDefaultEntityAccesses(t *testing.T) {
	service := ems.NewEntityRegistrationService()
	mockEnforcer := &MockEnforcerForTests{}

	// Setup des expectations pour le mock
	mockEnforcer.On("LoadPolicy").Return(nil)

	// All Casdoor roles that map to Member OCF role
	casdoorRoles := []string{"user", "member", "student", "premium_student", "teacher", "trainer", "supervisor"}

	for _, role := range casdoorRoles {
		// List endpoint (without wildcard) - note: "TestEntity" becomes "test-entities" via PascalToKebab
		mockEnforcer.On("AddPolicy",
			role,
			"/api/v1/test-entities",
			"("+http.MethodGet+"|"+http.MethodPost+")").Return(true, nil)
		// Resource endpoints (with wildcard)
		mockEnforcer.On("AddPolicy",
			role,
			"/api/v1/test-entities/*",
			"("+http.MethodGet+"|"+http.MethodPost+")").Return(true, nil)
	}

	roles := entityManagementInterfaces.EntityRoles{
		Roles: map[string]string{
			string(models.Member): "(" + http.MethodGet + "|" + http.MethodPost + ")",
		},
	}

	// Exécuter la méthode
	service.SetDefaultEntityAccesses("TestEntity", roles, mockEnforcer)

	// Vérifier que le mock a été appelé correctement
	mockEnforcer.AssertExpectations(t)
}

func TestEntityRegistrationService_SetDefaultEntityAccesses_WithNilEnforcer(t *testing.T) {
	service := ems.NewEntityRegistrationService()

	roles := entityManagementInterfaces.EntityRoles{
		Roles: map[string]string{
			string(models.Member): "(" + http.MethodGet + "|" + http.MethodPost + ")",
		},
	}

	// Cette méthode ne devrait pas paniquer avec un enforcer nil
	assert.NotPanics(t, func() {
		service.SetDefaultEntityAccesses("TestEntity", roles, nil)
	})
}

func TestEntityRegistrationService_Pluralize(t *testing.T) {
	testCases := []struct {
		singular string
		expected string
	}{
		{"Course", "Courses"},
		{"Chapter", "Chapters"},
		{"Section", "Sections"},
		{"Page", "Pages"},
		{"User", "Users"},
	}

	for _, tc := range testCases {
		t.Run(tc.singular, func(t *testing.T) {
			result := ems.Pluralize(tc.singular)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// Test d'intégration pour vérifier que le service global fonctionne
func TestGlobalEntityRegistrationService(t *testing.T) {
	// Sauvegarder l'état global
	originalService := ems.GlobalEntityRegistrationService

	// Créer un nouveau service pour le test
	testService := ems.NewEntityRegistrationService()
	ems.GlobalEntityRegistrationService = testService

	// Restore à la fin du test
	defer func() {
		ems.GlobalEntityRegistrationService = originalService
	}()

	// Test basique d'enregistrement
	testEntity := TestEntity{}
	testService.RegisterEntityInterface("TestEntity", testEntity)

	retrievedEntity, exists := ems.GlobalEntityRegistrationService.GetEntityInterface("TestEntity")
	assert.True(t, exists)
	assert.Equal(t, testEntity, retrievedEntity)
}

func TestEntityRegistrationService_TypedOps_NewOutputDto(t *testing.T) {
	service := ems.NewEntityRegistrationService()

	ems.RegisterTypedEntity[TestEntityWithBaseModel, TestEntityInputDto, TestEntityInputDto, TestEntityOutputDto](
		service,
		"TestEntityWithBaseModel",
		entityManagementInterfaces.TypedEntityRegistration[TestEntityWithBaseModel, TestEntityInputDto, TestEntityInputDto, TestEntityOutputDto]{
			Converters: entityManagementInterfaces.TypedEntityConverters[TestEntityWithBaseModel, TestEntityInputDto, TestEntityInputDto, TestEntityOutputDto]{
				ModelToDto: func(entity *TestEntityWithBaseModel) (TestEntityOutputDto, error) {
					return TestEntityOutputDto{ID: entity.ID.String(), Name: entity.Name}, nil
				},
				DtoToModel: func(dto TestEntityInputDto) *TestEntityWithBaseModel {
					return &TestEntityWithBaseModel{Name: dto.Name}
				},
			},
		},
	)

	ops, ok := service.GetEntityOps("TestEntityWithBaseModel")
	assert.True(t, ok)
	assert.NotNil(t, ops)

	outputDto := ops.NewOutputDto()
	assert.IsType(t, TestEntityOutputDto{}, outputDto)

	createDto := ops.NewCreateDto()
	assert.IsType(t, TestEntityInputDto{}, createDto)

	editDto := ops.NewEditDto()
	assert.IsType(t, TestEntityInputDto{}, editDto)
}

// ============================================================================
// Helper: register entity with erroring ModelToDto converter
// ============================================================================

func registerTestEntityWithErroringConverter(service *ems.EntityRegistrationService) {
	ems.RegisterTypedEntity[TestEntityWithBaseModel, TestEntityInputDto, TestEntityInputDto, TestEntityOutputDto](
		service,
		"ErroringEntity",
		entityManagementInterfaces.TypedEntityRegistration[TestEntityWithBaseModel, TestEntityInputDto, TestEntityInputDto, TestEntityOutputDto]{
			Converters: entityManagementInterfaces.TypedEntityConverters[TestEntityWithBaseModel, TestEntityInputDto, TestEntityInputDto, TestEntityOutputDto]{
				ModelToDto: func(entity *TestEntityWithBaseModel) (TestEntityOutputDto, error) {
					return TestEntityOutputDto{}, fmt.Errorf("conversion error")
				},
				DtoToModel: func(dto TestEntityInputDto) *TestEntityWithBaseModel {
					return &TestEntityWithBaseModel{Name: dto.Name}
				},
			},
		},
	)
}

// ============================================================================
// typedEntityOps.ConvertDtoToModel — type mismatch error
// ============================================================================

func TestTypedOps_ConvertDtoToModel_WrongType(t *testing.T) {
	service := ems.NewEntityRegistrationService()

	mockEnforcer := authMocks.NewMockEnforcer()
	mockEnforcer.LoadPolicyFunc = func() error { return nil }
	mockEnforcer.AddPolicyFunc = func(params ...any) (bool, error) { return true, nil }
	origEnforcer := casdoor.Enforcer
	casdoor.Enforcer = mockEnforcer
	defer func() { casdoor.Enforcer = origEnforcer }()

	ems.RegisterTypedEntity[TestEntityWithBaseModel, TestEntityInputDto, TestEntityInputDto, TestEntityOutputDto](
		service,
		"TestTypedEntity",
		entityManagementInterfaces.TypedEntityRegistration[TestEntityWithBaseModel, TestEntityInputDto, TestEntityInputDto, TestEntityOutputDto]{
			Converters: entityManagementInterfaces.TypedEntityConverters[TestEntityWithBaseModel, TestEntityInputDto, TestEntityInputDto, TestEntityOutputDto]{
				ModelToDto: func(entity *TestEntityWithBaseModel) (TestEntityOutputDto, error) {
					return TestEntityOutputDto{ID: entity.ID.String(), Name: entity.Name}, nil
				},
				DtoToModel: func(dto TestEntityInputDto) *TestEntityWithBaseModel {
					return &TestEntityWithBaseModel{Name: dto.Name}
				},
			},
		},
	)

	ops, ok := service.GetEntityOps("TestTypedEntity")
	assert.True(t, ok)

	// Pass a string instead of TestEntityInputDto
	_, err := ops.ConvertDtoToModel("wrong type")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expected")
	assert.Contains(t, err.Error(), "got")
}

// ============================================================================
// typedEntityOps.ConvertModelToDto — value, value error, wrong type, pointer error
// ============================================================================

func TestTypedOps_ConvertModelToDto_Value(t *testing.T) {
	service := ems.NewEntityRegistrationService()

	mockEnforcer := authMocks.NewMockEnforcer()
	mockEnforcer.LoadPolicyFunc = func() error { return nil }
	mockEnforcer.AddPolicyFunc = func(params ...any) (bool, error) { return true, nil }
	origEnforcer := casdoor.Enforcer
	casdoor.Enforcer = mockEnforcer
	defer func() { casdoor.Enforcer = origEnforcer }()

	ems.RegisterTypedEntity[TestEntityWithBaseModel, TestEntityInputDto, TestEntityInputDto, TestEntityOutputDto](
		service,
		"TestTypedEntity",
		entityManagementInterfaces.TypedEntityRegistration[TestEntityWithBaseModel, TestEntityInputDto, TestEntityInputDto, TestEntityOutputDto]{
			Converters: entityManagementInterfaces.TypedEntityConverters[TestEntityWithBaseModel, TestEntityInputDto, TestEntityInputDto, TestEntityOutputDto]{
				ModelToDto: func(entity *TestEntityWithBaseModel) (TestEntityOutputDto, error) {
					return TestEntityOutputDto{ID: entity.ID.String(), Name: entity.Name}, nil
				},
				DtoToModel: func(dto TestEntityInputDto) *TestEntityWithBaseModel {
					return &TestEntityWithBaseModel{Name: dto.Name}
				},
			},
		},
	)

	ops, ok := service.GetEntityOps("TestTypedEntity")
	assert.True(t, ok)

	// Pass by value (not pointer)
	model := TestEntityWithBaseModel{Name: "test"}
	result, err := ops.ConvertModelToDto(model)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	dto, ok := result.(TestEntityOutputDto)
	assert.True(t, ok)
	assert.Equal(t, "test", dto.Name)
}

func TestTypedOps_ConvertModelToDto_ValueError(t *testing.T) {
	service := ems.NewEntityRegistrationService()

	mockEnforcer := authMocks.NewMockEnforcer()
	mockEnforcer.LoadPolicyFunc = func() error { return nil }
	mockEnforcer.AddPolicyFunc = func(params ...any) (bool, error) { return true, nil }
	origEnforcer := casdoor.Enforcer
	casdoor.Enforcer = mockEnforcer
	defer func() { casdoor.Enforcer = origEnforcer }()

	registerTestEntityWithErroringConverter(service)

	ops, ok := service.GetEntityOps("ErroringEntity")
	assert.True(t, ok)

	// Pass by value — converter returns error
	model := TestEntityWithBaseModel{Name: "test"}
	_, err := ops.ConvertModelToDto(model)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "conversion error")
}

func TestTypedOps_ConvertModelToDto_WrongType(t *testing.T) {
	service := ems.NewEntityRegistrationService()

	mockEnforcer := authMocks.NewMockEnforcer()
	mockEnforcer.LoadPolicyFunc = func() error { return nil }
	mockEnforcer.AddPolicyFunc = func(params ...any) (bool, error) { return true, nil }
	origEnforcer := casdoor.Enforcer
	casdoor.Enforcer = mockEnforcer
	defer func() { casdoor.Enforcer = origEnforcer }()

	ems.RegisterTypedEntity[TestEntityWithBaseModel, TestEntityInputDto, TestEntityInputDto, TestEntityOutputDto](
		service,
		"TestTypedEntity",
		entityManagementInterfaces.TypedEntityRegistration[TestEntityWithBaseModel, TestEntityInputDto, TestEntityInputDto, TestEntityOutputDto]{
			Converters: entityManagementInterfaces.TypedEntityConverters[TestEntityWithBaseModel, TestEntityInputDto, TestEntityInputDto, TestEntityOutputDto]{
				ModelToDto: func(entity *TestEntityWithBaseModel) (TestEntityOutputDto, error) {
					return TestEntityOutputDto{ID: entity.ID.String(), Name: entity.Name}, nil
				},
				DtoToModel: func(dto TestEntityInputDto) *TestEntityWithBaseModel {
					return &TestEntityWithBaseModel{Name: dto.Name}
				},
			},
		},
	)

	ops, ok := service.GetEntityOps("TestTypedEntity")
	assert.True(t, ok)

	// Pass wrong type entirely
	_, err := ops.ConvertModelToDto("wrong type")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expected")
	assert.Contains(t, err.Error(), "got")
}

func TestTypedOps_ConvertModelToDto_PointerError(t *testing.T) {
	service := ems.NewEntityRegistrationService()

	mockEnforcer := authMocks.NewMockEnforcer()
	mockEnforcer.LoadPolicyFunc = func() error { return nil }
	mockEnforcer.AddPolicyFunc = func(params ...any) (bool, error) { return true, nil }
	origEnforcer := casdoor.Enforcer
	casdoor.Enforcer = mockEnforcer
	defer func() { casdoor.Enforcer = origEnforcer }()

	registerTestEntityWithErroringConverter(service)

	ops, ok := service.GetEntityOps("ErroringEntity")
	assert.True(t, ok)

	// Pass by pointer — converter returns error
	model := &TestEntityWithBaseModel{Name: "test"}
	_, err := ops.ConvertModelToDto(model)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "conversion error")
}

// ============================================================================
// typedEntityOps.ConvertEditDtoToMap — type mismatch when dtoToMap is set
// ============================================================================

func TestTypedOps_ConvertEditDtoToMap_WrongType(t *testing.T) {
	service := ems.NewEntityRegistrationService()

	mockEnforcer := authMocks.NewMockEnforcer()
	mockEnforcer.LoadPolicyFunc = func() error { return nil }
	mockEnforcer.AddPolicyFunc = func(params ...any) (bool, error) { return true, nil }
	origEnforcer := casdoor.Enforcer
	casdoor.Enforcer = mockEnforcer
	defer func() { casdoor.Enforcer = origEnforcer }()

	ems.RegisterTypedEntity[TestEntityWithBaseModel, TestEntityInputDto, TestEntityInputDto, TestEntityOutputDto](
		service,
		"TestTypedEntity",
		entityManagementInterfaces.TypedEntityRegistration[TestEntityWithBaseModel, TestEntityInputDto, TestEntityInputDto, TestEntityOutputDto]{
			Converters: entityManagementInterfaces.TypedEntityConverters[TestEntityWithBaseModel, TestEntityInputDto, TestEntityInputDto, TestEntityOutputDto]{
				ModelToDto: func(entity *TestEntityWithBaseModel) (TestEntityOutputDto, error) {
					return TestEntityOutputDto{ID: entity.ID.String(), Name: entity.Name}, nil
				},
				DtoToModel: func(dto TestEntityInputDto) *TestEntityWithBaseModel {
					return &TestEntityWithBaseModel{Name: dto.Name}
				},
				DtoToMap: func(dto TestEntityInputDto) map[string]any {
					return map[string]any{"name": dto.Name}
				},
			},
		},
	)

	ops, ok := service.GetEntityOps("TestTypedEntity")
	assert.True(t, ok)

	// Pass wrong type — should trigger type assertion failure
	_, err := ops.ConvertEditDtoToMap("wrong type")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expected")
	assert.Contains(t, err.Error(), "got")
}

// ============================================================================
// typedEntityOps.ExtractID — value path and wrong type
// ============================================================================

func TestTypedOps_ExtractID_Value(t *testing.T) {
	service := ems.NewEntityRegistrationService()

	mockEnforcer := authMocks.NewMockEnforcer()
	mockEnforcer.LoadPolicyFunc = func() error { return nil }
	mockEnforcer.AddPolicyFunc = func(params ...any) (bool, error) { return true, nil }
	origEnforcer := casdoor.Enforcer
	casdoor.Enforcer = mockEnforcer
	defer func() { casdoor.Enforcer = origEnforcer }()

	ems.RegisterTypedEntity[TestEntityWithBaseModel, TestEntityInputDto, TestEntityInputDto, TestEntityOutputDto](
		service,
		"TestTypedEntity",
		entityManagementInterfaces.TypedEntityRegistration[TestEntityWithBaseModel, TestEntityInputDto, TestEntityInputDto, TestEntityOutputDto]{
			Converters: entityManagementInterfaces.TypedEntityConverters[TestEntityWithBaseModel, TestEntityInputDto, TestEntityInputDto, TestEntityOutputDto]{
				ModelToDto: func(entity *TestEntityWithBaseModel) (TestEntityOutputDto, error) {
					return TestEntityOutputDto{ID: entity.ID.String(), Name: entity.Name}, nil
				},
				DtoToModel: func(dto TestEntityInputDto) *TestEntityWithBaseModel {
					return &TestEntityWithBaseModel{Name: dto.Name}
				},
			},
		},
	)

	ops, ok := service.GetEntityOps("TestTypedEntity")
	assert.True(t, ok)

	// Pass model by value (not pointer)
	testID := uuid.New()
	model := TestEntityWithBaseModel{Name: "test"}
	model.ID = testID

	id, err := ops.ExtractID(model)
	assert.NoError(t, err)
	assert.Equal(t, testID, id)
}

func TestTypedOps_ExtractID_WrongType(t *testing.T) {
	service := ems.NewEntityRegistrationService()

	mockEnforcer := authMocks.NewMockEnforcer()
	mockEnforcer.LoadPolicyFunc = func() error { return nil }
	mockEnforcer.AddPolicyFunc = func(params ...any) (bool, error) { return true, nil }
	origEnforcer := casdoor.Enforcer
	casdoor.Enforcer = mockEnforcer
	defer func() { casdoor.Enforcer = origEnforcer }()

	ems.RegisterTypedEntity[TestEntityWithBaseModel, TestEntityInputDto, TestEntityInputDto, TestEntityOutputDto](
		service,
		"TestTypedEntity",
		entityManagementInterfaces.TypedEntityRegistration[TestEntityWithBaseModel, TestEntityInputDto, TestEntityInputDto, TestEntityOutputDto]{
			Converters: entityManagementInterfaces.TypedEntityConverters[TestEntityWithBaseModel, TestEntityInputDto, TestEntityInputDto, TestEntityOutputDto]{
				ModelToDto: func(entity *TestEntityWithBaseModel) (TestEntityOutputDto, error) {
					return TestEntityOutputDto{ID: entity.ID.String(), Name: entity.Name}, nil
				},
				DtoToModel: func(dto TestEntityInputDto) *TestEntityWithBaseModel {
					return &TestEntityWithBaseModel{Name: dto.Name}
				},
			},
		},
	)

	ops, ok := service.GetEntityOps("TestTypedEntity")
	assert.True(t, ok)

	// Pass wrong type
	id, err := ops.ExtractID("wrong type")
	assert.Error(t, err)
	assert.Equal(t, uuid.Nil, id)
	assert.Contains(t, err.Error(), "expected")
	assert.Contains(t, err.Error(), "got")
}

// ============================================================================
// typedEntityOps.ConvertSliceToDto — typed slice error, non-slice, reflect slice error
// ============================================================================

func TestTypedOps_ConvertSliceToDto_TypedSliceError(t *testing.T) {
	service := ems.NewEntityRegistrationService()

	mockEnforcer := authMocks.NewMockEnforcer()
	mockEnforcer.LoadPolicyFunc = func() error { return nil }
	mockEnforcer.AddPolicyFunc = func(params ...any) (bool, error) { return true, nil }
	origEnforcer := casdoor.Enforcer
	casdoor.Enforcer = mockEnforcer
	defer func() { casdoor.Enforcer = origEnforcer }()

	registerTestEntityWithErroringConverter(service)

	ops, ok := service.GetEntityOps("ErroringEntity")
	assert.True(t, ok)

	// Pass a typed slice — converter will error on each item
	slice := []TestEntityWithBaseModel{{Name: "test"}}
	_, err := ops.ConvertSliceToDto(slice)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "conversion error")
}

func TestTypedOps_ConvertSliceToDto_NonSlice(t *testing.T) {
	service := ems.NewEntityRegistrationService()

	mockEnforcer := authMocks.NewMockEnforcer()
	mockEnforcer.LoadPolicyFunc = func() error { return nil }
	mockEnforcer.AddPolicyFunc = func(params ...any) (bool, error) { return true, nil }
	origEnforcer := casdoor.Enforcer
	casdoor.Enforcer = mockEnforcer
	defer func() { casdoor.Enforcer = origEnforcer }()

	ems.RegisterTypedEntity[TestEntityWithBaseModel, TestEntityInputDto, TestEntityInputDto, TestEntityOutputDto](
		service,
		"TestTypedEntity",
		entityManagementInterfaces.TypedEntityRegistration[TestEntityWithBaseModel, TestEntityInputDto, TestEntityInputDto, TestEntityOutputDto]{
			Converters: entityManagementInterfaces.TypedEntityConverters[TestEntityWithBaseModel, TestEntityInputDto, TestEntityInputDto, TestEntityOutputDto]{
				ModelToDto: func(entity *TestEntityWithBaseModel) (TestEntityOutputDto, error) {
					return TestEntityOutputDto{ID: entity.ID.String(), Name: entity.Name}, nil
				},
				DtoToModel: func(dto TestEntityInputDto) *TestEntityWithBaseModel {
					return &TestEntityWithBaseModel{Name: dto.Name}
				},
			},
		},
	)

	ops, ok := service.GetEntityOps("TestTypedEntity")
	assert.True(t, ok)

	// Pass a single struct instead of a slice
	_, err := ops.ConvertSliceToDto(TestEntityWithBaseModel{Name: "test"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expected slice")
}

func TestTypedOps_ConvertSliceToDto_ReflectSliceSuccess(t *testing.T) {
	service := ems.NewEntityRegistrationService()

	mockEnforcer := authMocks.NewMockEnforcer()
	mockEnforcer.LoadPolicyFunc = func() error { return nil }
	mockEnforcer.AddPolicyFunc = func(params ...any) (bool, error) { return true, nil }
	origEnforcer := casdoor.Enforcer
	casdoor.Enforcer = mockEnforcer
	defer func() { casdoor.Enforcer = origEnforcer }()

	ems.RegisterTypedEntity[TestEntityWithBaseModel, TestEntityInputDto, TestEntityInputDto, TestEntityOutputDto](
		service,
		"TestTypedEntity",
		entityManagementInterfaces.TypedEntityRegistration[TestEntityWithBaseModel, TestEntityInputDto, TestEntityInputDto, TestEntityOutputDto]{
			Converters: entityManagementInterfaces.TypedEntityConverters[TestEntityWithBaseModel, TestEntityInputDto, TestEntityInputDto, TestEntityOutputDto]{
				ModelToDto: func(entity *TestEntityWithBaseModel) (TestEntityOutputDto, error) {
					return TestEntityOutputDto{ID: entity.ID.String(), Name: entity.Name}, nil
				},
				DtoToModel: func(dto TestEntityInputDto) *TestEntityWithBaseModel {
					return &TestEntityWithBaseModel{Name: dto.Name}
				},
			},
		},
	)

	ops, ok := service.GetEntityOps("TestTypedEntity")
	assert.True(t, ok)

	// Pass []any with valid model elements — goes through reflect path successfully
	model := TestEntityWithBaseModel{Name: "test"}
	slice := []any{model}
	result, err := ops.ConvertSliceToDto(slice)
	assert.NoError(t, err)
	assert.Len(t, result, 1)
	dto, ok := result[0].(TestEntityOutputDto)
	assert.True(t, ok)
	assert.Equal(t, "test", dto.Name)
}

func TestTypedOps_ConvertSliceToDto_ReflectSliceError(t *testing.T) {
	service := ems.NewEntityRegistrationService()

	mockEnforcer := authMocks.NewMockEnforcer()
	mockEnforcer.LoadPolicyFunc = func() error { return nil }
	mockEnforcer.AddPolicyFunc = func(params ...any) (bool, error) { return true, nil }
	origEnforcer := casdoor.Enforcer
	casdoor.Enforcer = mockEnforcer
	defer func() { casdoor.Enforcer = origEnforcer }()

	ems.RegisterTypedEntity[TestEntityWithBaseModel, TestEntityInputDto, TestEntityInputDto, TestEntityOutputDto](
		service,
		"TestTypedEntity",
		entityManagementInterfaces.TypedEntityRegistration[TestEntityWithBaseModel, TestEntityInputDto, TestEntityInputDto, TestEntityOutputDto]{
			Converters: entityManagementInterfaces.TypedEntityConverters[TestEntityWithBaseModel, TestEntityInputDto, TestEntityInputDto, TestEntityOutputDto]{
				ModelToDto: func(entity *TestEntityWithBaseModel) (TestEntityOutputDto, error) {
					return TestEntityOutputDto{ID: entity.ID.String(), Name: entity.Name}, nil
				},
				DtoToModel: func(dto TestEntityInputDto) *TestEntityWithBaseModel {
					return &TestEntityWithBaseModel{Name: dto.Name}
				},
			},
		},
	)

	ops, ok := service.GetEntityOps("TestTypedEntity")
	assert.True(t, ok)

	// Pass []any with wrong element type — goes through reflect path, ConvertModelToDto fails
	slice := []any{"bad type"}
	_, err := ops.ConvertSliceToDto(slice)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expected")
	assert.Contains(t, err.Error(), "got")
}

// ============================================================================
// RegisterTypedEntity — SwaggerConfig with empty EntityName
// ============================================================================

func TestRegisterTypedEntity_SwaggerConfigEmptyEntityName(t *testing.T) {
	service := ems.NewEntityRegistrationService()

	mockEnforcer := authMocks.NewMockEnforcer()
	mockEnforcer.LoadPolicyFunc = func() error { return nil }
	mockEnforcer.AddPolicyFunc = func(params ...any) (bool, error) { return true, nil }
	origEnforcer := casdoor.Enforcer
	casdoor.Enforcer = mockEnforcer
	defer func() { casdoor.Enforcer = origEnforcer }()

	ems.RegisterTypedEntity[TestEntityWithBaseModel, TestEntityInputDto, TestEntityInputDto, TestEntityOutputDto](
		service,
		"MyEntityName",
		entityManagementInterfaces.TypedEntityRegistration[TestEntityWithBaseModel, TestEntityInputDto, TestEntityInputDto, TestEntityOutputDto]{
			Converters: entityManagementInterfaces.TypedEntityConverters[TestEntityWithBaseModel, TestEntityInputDto, TestEntityInputDto, TestEntityOutputDto]{
				ModelToDto: func(entity *TestEntityWithBaseModel) (TestEntityOutputDto, error) {
					return TestEntityOutputDto{ID: entity.ID.String(), Name: entity.Name}, nil
				},
				DtoToModel: func(dto TestEntityInputDto) *TestEntityWithBaseModel {
					return &TestEntityWithBaseModel{Name: dto.Name}
				},
			},
			SwaggerConfig: &entityManagementInterfaces.EntitySwaggerConfig{
				Tag:        "my-entities",
				EntityName: "", // Empty — should be populated with "MyEntityName"
			},
		},
	)

	swaggerConfig := service.GetSwaggerConfig("MyEntityName")
	assert.NotNil(t, swaggerConfig)
	assert.Equal(t, "MyEntityName", swaggerConfig.EntityName)
}

// Tests de performance pour identifier les goulots d'étranglement
func BenchmarkEntityRegistrationService_GetEntityInterface(b *testing.B) {
	service := ems.NewEntityRegistrationService()
	service.RegisterEntityInterface("TestEntity", TestEntity{})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		service.GetEntityInterface("TestEntity")
	}
}

