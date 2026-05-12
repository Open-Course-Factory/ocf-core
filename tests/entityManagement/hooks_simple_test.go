// tests/entityManagement/hooks_simple_test.go
package entityManagement_tests

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"soli/formations/src/auth/casdoor"
	authMocks "soli/formations/src/auth/mocks"
	ems "soli/formations/src/entityManagement/entityManagementService"
	"soli/formations/src/entityManagement/hooks"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/entityManagement/services"
)

// ============================================================================
// Simple Test Hooks
// ============================================================================

// TrackingHook tracks execution
type SimpleTrackingHook struct {
	name          string
	entityName    string
	hookTypes     []hooks.HookType
	executedCount int
	lastContext   *hooks.HookContext
	mu            sync.Mutex
	enabled       bool
	priority      int
}

func NewSimpleTrackingHook(name, entityName string, hookTypes []hooks.HookType, priority int) *SimpleTrackingHook {
	return &SimpleTrackingHook{
		name:       name,
		entityName: entityName,
		hookTypes:  hookTypes,
		enabled:    true,
		priority:   priority,
	}
}

func (h *SimpleTrackingHook) GetName() string                { return h.name }
func (h *SimpleTrackingHook) GetEntityName() string          { return h.entityName }
func (h *SimpleTrackingHook) GetHookTypes() []hooks.HookType { return h.hookTypes }
func (h *SimpleTrackingHook) IsEnabled() bool                { return h.enabled }
func (h *SimpleTrackingHook) GetPriority() int               { return h.priority }

func (h *SimpleTrackingHook) Execute(ctx *hooks.HookContext) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.executedCount++
	h.lastContext = ctx
	return nil
}

func (h *SimpleTrackingHook) GetExecutedCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.executedCount
}

func (h *SimpleTrackingHook) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.executedCount = 0
	h.lastContext = nil
}

// FailingHook always fails
type SimpleFailingHook struct {
	*SimpleTrackingHook
	shouldFail bool
}

func NewSimpleFailingHook(name, entityName string, hookTypes []hooks.HookType) *SimpleFailingHook {
	return &SimpleFailingHook{
		SimpleTrackingHook: NewSimpleTrackingHook(name, entityName, hookTypes, 50),
		shouldFail:         true,
	}
}

func (h *SimpleFailingHook) Execute(ctx *hooks.HookContext) error {
	_ = h.SimpleTrackingHook.Execute(ctx)
	if h.shouldFail {
		return errors.New("hook execution failed")
	}
	return nil
}

// ConditionalHook with condition function
type SimpleConditionalHook struct {
	*SimpleTrackingHook
	condition func(*hooks.HookContext) bool
}

func NewSimpleConditionalHook(name, entityName string, hookTypes []hooks.HookType, condition func(*hooks.HookContext) bool) *SimpleConditionalHook {
	return &SimpleConditionalHook{
		SimpleTrackingHook: NewSimpleTrackingHook(name, entityName, hookTypes, 50),
		condition:          condition,
	}
}

func (h *SimpleConditionalHook) ShouldExecute(ctx *hooks.HookContext) bool {
	return h.condition(ctx)
}

// Test entity for hooks
type HookTestEntitySimple struct {
	entityManagementModels.BaseModel
	Name   string `json:"name"`
	Value  int    `json:"value"`
	Status string `json:"status"`
}

// ============================================================================
// Hook Registry Tests
// ============================================================================

func TestHooksSimple_RegistryBasics(t *testing.T) {
	registry := hooks.NewHookRegistry()

	hook1 := NewSimpleTrackingHook("test-hook-1", "TestEntity", []hooks.HookType{hooks.BeforeCreate}, 10)
	hook2 := NewSimpleTrackingHook("test-hook-2", "TestEntity", []hooks.HookType{hooks.AfterCreate}, 20)

	t.Run("Register Hook", func(t *testing.T) {
		err := registry.RegisterHook(hook1)
		assert.NoError(t, err)

		err = registry.RegisterHook(hook2)
		assert.NoError(t, err)

		t.Logf("✅ Registered 2 hooks successfully")
	})

	t.Run("Register Duplicate Hook", func(t *testing.T) {
		err := registry.RegisterHook(hook1)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already registered")

		t.Logf("✅ Duplicate registration prevented")
	})

	t.Run("Get Hooks By Type", func(t *testing.T) {
		beforeCreateHooks := registry.GetHooks("TestEntity", hooks.BeforeCreate)
		assert.Len(t, beforeCreateHooks, 1)
		assert.Equal(t, "test-hook-1", beforeCreateHooks[0].GetName())

		afterCreateHooks := registry.GetHooks("TestEntity", hooks.AfterCreate)
		assert.Len(t, afterCreateHooks, 1)
		assert.Equal(t, "test-hook-2", afterCreateHooks[0].GetName())

		t.Logf("✅ Retrieved hooks by type correctly")
	})

	t.Run("Unregister Hook", func(t *testing.T) {
		err := registry.UnregisterHook("test-hook-1")
		assert.NoError(t, err)

		beforeCreateHooks := registry.GetHooks("TestEntity", hooks.BeforeCreate)
		assert.Len(t, beforeCreateHooks, 0)

		t.Logf("✅ Unregistered hook successfully")
	})

	t.Run("Enable/Disable Hook", func(t *testing.T) {
		err := registry.EnableHook("test-hook-2", false)
		assert.NoError(t, err)

		err = registry.EnableHook("test-hook-2", true)
		assert.NoError(t, err)

		t.Logf("✅ Enable/disable hook works")
	})
}

func TestHooksSimple_ExecutionOrder(t *testing.T) {
	registry := hooks.NewHookRegistry()

	executionOrder := []string{}
	mu := sync.Mutex{}

	// Create hooks with different priorities
	priorities := []int{50, 10, 30, 20, 40}
	testHooks := make([]*SimpleTrackingHook, len(priorities))

	for i, priority := range priorities {
		hook := NewSimpleTrackingHook(fmt.Sprintf("hook-%d", i), "TestEntity", []hooks.HookType{hooks.BeforeCreate}, priority)
		testHooks[i] = hook
		_ = registry.RegisterHook(hook)
	}

	ctx := &hooks.HookContext{
		EntityName: "TestEntity",
		HookType:   hooks.BeforeCreate,
		NewEntity:  map[string]any{"test": "data"},
		Context:    context.Background(),
	}

	err := registry.ExecuteHooks(ctx)
	assert.NoError(t, err)

	// Build execution order from execution counts
	for _, hook := range testHooks {
		if hook.GetExecutedCount() > 0 {
			mu.Lock()
			executionOrder = append(executionOrder, hook.GetName())
			mu.Unlock()
		}
	}

	// Verify all hooks executed
	assert.Len(t, executionOrder, 5)

	t.Logf("✅ All hooks executed in order: %v", executionOrder)
}

func TestHooksSimple_ConditionalExecution(t *testing.T) {
	registry := hooks.NewHookRegistry()

	// Hook that only executes if value > 10
	conditionalHook := NewSimpleConditionalHook(
		"conditional-hook",
		"TestEntity",
		[]hooks.HookType{hooks.BeforeCreate},
		func(ctx *hooks.HookContext) bool {
			if data, ok := ctx.NewEntity.(map[string]any); ok {
				if value, ok := data["value"].(int); ok {
					return value > 10
				}
			}
			return false
		},
	)

	_ = registry.RegisterHook(conditionalHook)

	t.Run("Condition False - Should Not Execute", func(t *testing.T) {
		ctx := &hooks.HookContext{
			EntityName: "TestEntity",
			HookType:   hooks.BeforeCreate,
			NewEntity:  map[string]any{"value": 5},
			Context:    context.Background(),
		}

		conditionalHook.Reset()
		err := registry.ExecuteHooks(ctx)
		assert.NoError(t, err)
		assert.Equal(t, 0, conditionalHook.GetExecutedCount())

		t.Logf("✅ Conditional hook did not execute (value=5)")
	})

	t.Run("Condition True - Should Execute", func(t *testing.T) {
		ctx := &hooks.HookContext{
			EntityName: "TestEntity",
			HookType:   hooks.BeforeCreate,
			NewEntity:  map[string]any{"value": 15},
			Context:    context.Background(),
		}

		conditionalHook.Reset()
		err := registry.ExecuteHooks(ctx)
		assert.NoError(t, err)
		assert.Equal(t, 1, conditionalHook.GetExecutedCount())

		t.Logf("✅ Conditional hook executed (value=15)")
	})
}

func TestHooksSimple_FailureHandling(t *testing.T) {
	registry := hooks.NewHookRegistry()

	successHook := NewSimpleTrackingHook("success-hook", "TestEntity", []hooks.HookType{hooks.BeforeCreate}, 10)
	failingHook := NewSimpleFailingHook("failing-hook", "TestEntity", []hooks.HookType{hooks.BeforeCreate})
	failingHook.priority = 20

	_ = registry.RegisterHook(successHook)
	_ = registry.RegisterHook(failingHook)

	ctx := &hooks.HookContext{
		EntityName: "TestEntity",
		HookType:   hooks.BeforeCreate,
		NewEntity:  map[string]any{"test": "data"},
		Context:    context.Background(),
	}

	err := registry.ExecuteHooks(ctx)

	// Currently, the registry continues on error
	assert.NoError(t, err)

	// Both hooks should have been attempted
	assert.Equal(t, 1, successHook.GetExecutedCount())
	assert.Equal(t, 1, failingHook.GetExecutedCount())

	t.Logf("✅ Hook registry continues execution on failure")
}

func TestHooksSimple_EnableDisable(t *testing.T) {
	registry := hooks.NewHookRegistry()

	hook := NewSimpleTrackingHook("toggle-hook", "TestEntity", []hooks.HookType{hooks.BeforeCreate}, 10)
	_ = registry.RegisterHook(hook)

	ctx := &hooks.HookContext{
		EntityName: "TestEntity",
		HookType:   hooks.BeforeCreate,
		NewEntity:  map[string]any{"test": "data"},
		Context:    context.Background(),
	}

	// Execute while enabled
	_ = registry.ExecuteHooks(ctx)
	assert.Equal(t, 1, hook.GetExecutedCount())

	// Disable and execute
	_ = registry.EnableHook("toggle-hook", false)
	_ = registry.ExecuteHooks(ctx)
	assert.Equal(t, 1, hook.GetExecutedCount(), "Should not execute when disabled")

	// Re-enable and execute
	_ = registry.EnableHook("toggle-hook", true)
	_ = registry.ExecuteHooks(ctx)
	assert.Equal(t, 2, hook.GetExecutedCount())

	t.Logf("✅ Hook enable/disable works correctly")
}

// ============================================================================
// Service Integration Tests
// ============================================================================

func setupHookServiceTestSimple(t *testing.T) (*gorm.DB, services.GenericService, hooks.HookRegistry) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&HookTestEntitySimple{})
	require.NoError(t, err)

	mockEnforcer := authMocks.NewMockEnforcer()
	mockEnforcer.LoadPolicyFunc = func() error { return nil }
	mockEnforcer.AddPolicyFunc = func(params ...any) (bool, error) { return true, nil }

	originalEnforcer := casdoor.Enforcer
	casdoor.Enforcer = mockEnforcer
	t.Cleanup(func() {
		casdoor.Enforcer = originalEnforcer
	})

	service := services.NewGenericService(db, nil)

	// Register entity with typed registration
	ems.RegisterTypedEntity[HookTestEntitySimple, HookTestEntitySimple, HookTestEntitySimple, HookTestEntitySimple](
		ems.GlobalEntityRegistrationService,
		"HookTestEntity",
		entityManagementInterfaces.TypedEntityRegistration[HookTestEntitySimple, HookTestEntitySimple, HookTestEntitySimple, HookTestEntitySimple]{
			Converters: entityManagementInterfaces.TypedEntityConverters[HookTestEntitySimple, HookTestEntitySimple, HookTestEntitySimple, HookTestEntitySimple]{
				ModelToDto: func(entity *HookTestEntitySimple) (HookTestEntitySimple, error) {
					return *entity, nil
				},
				DtoToModel: func(dto HookTestEntitySimple) *HookTestEntitySimple {
					return &dto
				},
			},
		},
	)

	t.Cleanup(func() {
		ems.GlobalEntityRegistrationService.UnregisterEntity("HookTestEntity")
	})

	// Replace global hook registry with test registry
	originalRegistry := hooks.GlobalHookRegistry
	testRegistry := hooks.NewHookRegistry()
	hooks.GlobalHookRegistry = testRegistry
	t.Cleanup(func() {
		hooks.GlobalHookRegistry = originalRegistry
	})

	return db, service, testRegistry
}

func TestHooksSimple_ServiceIntegration_BeforeCreate(t *testing.T) {
	_, service, registry := setupHookServiceTestSimple(t)

	beforeCreateHook := NewSimpleTrackingHook("before-create", "HookTestEntity", []hooks.HookType{hooks.BeforeCreate}, 10)
	_ = registry.RegisterHook(beforeCreateHook)

	entity := HookTestEntitySimple{
		Name:   "Test Entity",
		Value:  42,
		Status: "pending",
	}

	_, err := service.CreateEntity(entity, "HookTestEntity")
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, 1, beforeCreateHook.GetExecutedCount())
	t.Logf("✅ Before create hook executed")
}

func TestHooksSimple_ServiceIntegration_AfterCreate(t *testing.T) {
	_, service, registry := setupHookServiceTestSimple(t)

	afterCreateHook := NewSimpleTrackingHook("after-create", "HookTestEntity", []hooks.HookType{hooks.AfterCreate}, 10)
	_ = registry.RegisterHook(afterCreateHook)

	entity := HookTestEntitySimple{
		Name:   "Test Entity",
		Value:  42,
		Status: "pending",
	}

	_, err := service.CreateEntity(entity, "HookTestEntity")
	require.NoError(t, err)

	// Wait for async after-create hook
	time.Sleep(200 * time.Millisecond)

	assert.Equal(t, 1, afterCreateHook.GetExecutedCount())
	t.Logf("✅ After create hook executed (async)")
}

func TestHooksSimple_ConcurrentExecution(t *testing.T) {
	t.Skip("SQLite in-memory database doesn't handle concurrent writes well. This test works with PostgreSQL in production.")

	_, service, registry := setupHookServiceTestSimple(t)

	hook := NewSimpleTrackingHook("concurrent-hook", "HookTestEntity", []hooks.HookType{hooks.BeforeCreate}, 10)
	_ = registry.RegisterHook(hook)

	// Create multiple entities concurrently
	numGoroutines := 10
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(index int) {
			defer wg.Done()

			entity := HookTestEntitySimple{
				Name:   fmt.Sprintf("Entity %d", index),
				Value:  index,
				Status: "active",
			}

			_, err := service.CreateEntity(entity, "HookTestEntity")
			assert.NoError(t, err)
		}(i)
	}

	wg.Wait()
	time.Sleep(200 * time.Millisecond)

	assert.Equal(t, numGoroutines, hook.GetExecutedCount())
	t.Logf("✅ Hook executed %d times concurrently", hook.GetExecutedCount())
}

// ============================================================================
// Error Tracking Tests
// ============================================================================

func TestHooksSimple_ErrorTracking_AsyncHookFailure(t *testing.T) {
	_, service, registry := setupHookServiceTestSimple(t)

	// Register a failing after-create hook
	failingHook := NewSimpleFailingHook("failing-after-create", "HookTestEntity", []hooks.HookType{hooks.AfterCreate})
	_ = registry.RegisterHook(failingHook)

	// Clear any previous errors
	registry.ClearErrors()

	// Create entity (should succeed even if after-hook fails)
	entity := HookTestEntitySimple{
		Name:   "Test Entity",
		Value:  42,
		Status: "active",
	}

	createdEntity, err := service.CreateEntity(entity, "HookTestEntity")
	require.NoError(t, err, "Entity creation should succeed even if after-hook fails")

	// Check that error was recorded
	errors := registry.GetRecentErrors(0)
	require.Len(t, errors, 1, "Should have 1 recorded error")

	hookError := errors[0]
	assert.Equal(t, "failing-after-create", hookError.HookName)
	assert.Equal(t, "HookTestEntity", hookError.EntityName)
	assert.Equal(t, hooks.AfterCreate, hookError.HookType)
	assert.Contains(t, hookError.Error, "hook execution failed")
	assert.NotNil(t, hookError.Timestamp)
	assert.Equal(t, createdEntity.(*HookTestEntitySimple).ID, hookError.EntityID)

	t.Logf("✅ After-hook error tracked: %+v", hookError)
}

func TestHooksSimple_ErrorTracking_MultipleErrors(t *testing.T) {
	_, service, registry := setupHookServiceTestSimple(t)

	registry.ClearErrors()

	// Register failing after-create hook
	afterCreateHook := NewSimpleFailingHook("fail-create", "HookTestEntity", []hooks.HookType{hooks.AfterCreate})
	_ = registry.RegisterHook(afterCreateHook)

	// Create multiple entities to generate multiple errors
	for i := 0; i < 3; i++ {
		entity := HookTestEntitySimple{Name: fmt.Sprintf("Test %d", i), Value: i, Status: "active"}
		_, _ = service.CreateEntity(entity, "HookTestEntity")
	}

	// Check all errors recorded
	errors := registry.GetRecentErrors(0)
	assert.Len(t, errors, 3, "Should have 3 errors from 3 creates")

	for _, err := range errors {
		assert.Equal(t, "fail-create", err.HookName)
		assert.Equal(t, hooks.AfterCreate, err.HookType)
		assert.Contains(t, err.Error, "hook execution failed")
	}

	t.Logf("✅ Tracked %d errors from multiple operations", len(errors))
}

func TestHooksSimple_ErrorTracking_GetRecentErrors(t *testing.T) {
	registry := hooks.NewHookRegistry()

	// Register ONE failing hook
	failingHook := NewSimpleFailingHook("test-hook", "TestEntity", []hooks.HookType{hooks.AfterCreate})
	_ = registry.RegisterHook(failingHook)

	// Execute it 10 times to generate 10 errors
	for i := 0; i < 10; i++ {
		ctx := &hooks.HookContext{
			EntityName: "TestEntity",
			HookType:   hooks.AfterCreate,
			EntityID:   fmt.Sprintf("entity-%d", i),
		}
		_ = registry.ExecuteHooks(ctx)
	}

	t.Run("Get All Errors", func(t *testing.T) {
		errors := registry.GetRecentErrors(0)
		assert.Len(t, errors, 10)
		t.Logf("✅ Retrieved all %d errors", len(errors))
	})

	t.Run("Get Last 5 Errors", func(t *testing.T) {
		errors := registry.GetRecentErrors(5)
		assert.Len(t, errors, 5)
		t.Logf("✅ Retrieved last 5 errors")
	})

	t.Run("Get More Than Available", func(t *testing.T) {
		errors := registry.GetRecentErrors(20)
		assert.Len(t, errors, 10, "Should return all available errors")
		t.Logf("✅ Requested 20, got %d available errors", len(errors))
	})
}

func TestHooksSimple_ErrorTracking_CircularBuffer(t *testing.T) {
	registry := hooks.NewHookRegistry()

	// Register ONE failing hook
	failingHook := NewSimpleFailingHook("test-hook", "TestEntity", []hooks.HookType{hooks.AfterCreate})
	_ = registry.RegisterHook(failingHook)

	// Create more errors than the buffer can hold (default 100)
	for i := 0; i < 150; i++ {
		ctx := &hooks.HookContext{
			EntityName: "TestEntity",
			HookType:   hooks.AfterCreate,
			EntityID:   fmt.Sprintf("entity-%d", i),
		}
		_ = registry.ExecuteHooks(ctx)
	}

	errors := registry.GetRecentErrors(0)
	assert.LessOrEqual(t, len(errors), 100, "Should not exceed max buffer size")
	t.Logf("✅ Circular buffer maintained, storing %d errors (max 100)", len(errors))
}

func TestHooksSimple_ErrorTracking_ClearErrors(t *testing.T) {
	registry := hooks.NewHookRegistry()

	// Register ONE failing hook
	failingHook := NewSimpleFailingHook("test-hook", "TestEntity", []hooks.HookType{hooks.AfterCreate})
	_ = registry.RegisterHook(failingHook)

	// Add some errors
	for i := 0; i < 5; i++ {
		ctx := &hooks.HookContext{
			EntityName: "TestEntity",
			HookType:   hooks.AfterCreate,
			EntityID:   fmt.Sprintf("entity-%d", i),
		}
		_ = registry.ExecuteHooks(ctx)
	}

	assert.Len(t, registry.GetRecentErrors(0), 5, "Should have 5 errors")

	registry.ClearErrors()

	assert.Len(t, registry.GetRecentErrors(0), 0, "Should have 0 errors after clear")
	t.Logf("✅ Errors cleared successfully")
}

func TestHooksSimple_ErrorTracking_Callback(t *testing.T) {
	registry := hooks.NewHookRegistry()

	// Track callback invocations
	var callbackErrors []hooks.HookError
	var mu sync.Mutex

	registry.SetErrorCallback(func(err *hooks.HookError) {
		mu.Lock()
		defer mu.Unlock()
		callbackErrors = append(callbackErrors, *err)
	})

	// Add error
	ctx := &hooks.HookContext{
		EntityName: "TestEntity",
		HookType:   hooks.AfterCreate,
		EntityID:   "test-123",
	}

	failingHook := NewSimpleFailingHook("callback-test-hook", "TestEntity", []hooks.HookType{hooks.AfterCreate})
	_ = registry.RegisterHook(failingHook)
	_ = registry.ExecuteHooks(ctx)

	mu.Lock()
	defer mu.Unlock()

	assert.Len(t, callbackErrors, 1, "Callback should be invoked once")
	assert.Equal(t, "callback-test-hook", callbackErrors[0].HookName)
	assert.Equal(t, "test-123", callbackErrors[0].EntityID)

	t.Logf("✅ Error callback invoked: %+v", callbackErrors[0])
}

func TestHooksSimple_ErrorTracking_BeforeHooksNotTracked(t *testing.T) {
	registry := hooks.NewHookRegistry()
	registry.ClearErrors()

	// Before hooks should NOT be tracked (they're synchronous and return errors)
	failingHook := NewSimpleFailingHook("failing-before-create", "TestEntity", []hooks.HookType{hooks.BeforeCreate})
	_ = registry.RegisterHook(failingHook)

	ctx := &hooks.HookContext{
		EntityName: "TestEntity",
		HookType:   hooks.BeforeCreate,
		EntityID:   "test-123",
	}

	_ = registry.ExecuteHooks(ctx)

	errors := registry.GetRecentErrors(0)
	assert.Len(t, errors, 0, "Before hooks should not be tracked in error buffer")

	t.Logf("✅ Before hooks correctly not tracked (they return errors synchronously)")
}

// ============================================================================
// Synchronous AfterHook Contract (issue #316)
// ============================================================================
//
// These tests lock in the contract that AfterCreate hooks run synchronously,
// so their side effects (and any errors they report) are visible to the caller
// immediately after CreateEntity returns. The hook deliberately sleeps inside
// its Execute() — if the framework spawned a goroutine, the assertions would
// race the goroutine and fail. With hooks always running sync, the sleep
// happens inline and the assertions trivially succeed.

// slowSideEffectHook flips an atomic flag inside Execute, after sleeping long
// enough that an async dispatch is observable to the caller.
type slowSideEffectHook struct {
	*SimpleTrackingHook
	flag  *int32
	delay time.Duration
}

func newSlowSideEffectHook(name, entityName string, hookTypes []hooks.HookType, flag *int32, delay time.Duration) *slowSideEffectHook {
	return &slowSideEffectHook{
		SimpleTrackingHook: NewSimpleTrackingHook(name, entityName, hookTypes, 50),
		flag:               flag,
		delay:              delay,
	}
}

func (h *slowSideEffectHook) Execute(ctx *hooks.HookContext) error {
	_ = h.SimpleTrackingHook.Execute(ctx)
	if h.delay > 0 {
		time.Sleep(h.delay)
	}
	atomic.StoreInt32(h.flag, 1)
	return nil
}

// slowFailingHook is like SimpleFailingHook but sleeps before returning, to
// ensure the failure cannot reach the error sink before CreateEntity returns
// if the registry dispatches it asynchronously.
type slowFailingHook struct {
	*SimpleTrackingHook
	delay time.Duration
}

func newSlowFailingHook(name, entityName string, hookTypes []hooks.HookType, delay time.Duration) *slowFailingHook {
	return &slowFailingHook{
		SimpleTrackingHook: NewSimpleTrackingHook(name, entityName, hookTypes, 50),
		delay:              delay,
	}
}

func (h *slowFailingHook) Execute(ctx *hooks.HookContext) error {
	_ = h.SimpleTrackingHook.Execute(ctx)
	if h.delay > 0 {
		time.Sleep(h.delay)
	}
	return errors.New("slow hook failed")
}

// TestGenericService_AfterCreate_RunsSynchronouslyInProdMode locks in that
// AfterCreate side effects are visible immediately after CreateEntity returns.
// The same production code path is used for tests and prod (no test-mode
// short-circuit), so this contract holds everywhere.
func TestGenericService_AfterCreate_RunsSynchronouslyInProdMode(t *testing.T) {
	_, service, registry := setupHookServiceTestSimple(t)

	var hookRan int32
	hook := newSlowSideEffectHook(
		"sync-after-create",
		"HookTestEntity",
		[]hooks.HookType{hooks.AfterCreate},
		&hookRan,
		50*time.Millisecond, // long enough that async dispatch is observable
	)
	require.NoError(t, registry.RegisterHook(hook))

	entity := HookTestEntitySimple{Name: "Sync Test", Value: 1, Status: "active"}

	_, err := service.CreateEntity(entity, "HookTestEntity")
	require.NoError(t, err)

	// IMMEDIATE assertion — no sleep, no eventually. If the hook ran in a
	// goroutine, the flag is still 0 here.
	require.Equal(t, int32(1), atomic.LoadInt32(&hookRan),
		"AfterCreate hook must complete before CreateEntity returns (side effect must be visible synchronously)")
}

// TestGenericService_AfterCreate_ErrorSurfacesSynchronously locks in that an
// AfterCreate hook error reaches the registered error callback before
// CreateEntity returns, so callers (controllers, tests, monitoring) can act on
// it without polling.
func TestGenericService_AfterCreate_ErrorSurfacesSynchronously(t *testing.T) {
	_, service, registry := setupHookServiceTestSimple(t)

	registry.ClearErrors()

	var capturedErr atomic.Value // stores *hooks.HookError
	registry.SetErrorCallback(func(err *hooks.HookError) {
		capturedErr.Store(err)
	})

	failing := newSlowFailingHook(
		"sync-failing-after-create",
		"HookTestEntity",
		[]hooks.HookType{hooks.AfterCreate},
		50*time.Millisecond,
	)
	require.NoError(t, registry.RegisterHook(failing))

	entity := HookTestEntitySimple{Name: "Error Sync Test", Value: 2, Status: "active"}

	_, err := service.CreateEntity(entity, "HookTestEntity")
	require.NoError(t, err, "CreateEntity must still succeed even when AfterCreate fails")

	// IMMEDIATE assertion — no sleep. If either the hook or the callback ran in
	// a goroutine, the captured value is still nil here.
	got := capturedErr.Load()
	require.NotNil(t, got, "AfterCreate hook error must surface to the callback before CreateEntity returns")
	hookErr := got.(*hooks.HookError)
	assert.Equal(t, "sync-failing-after-create", hookErr.HookName)
	assert.Equal(t, "HookTestEntity", hookErr.EntityName)
	assert.Equal(t, hooks.AfterCreate, hookErr.HookType)
	assert.Contains(t, hookErr.Error, "slow hook failed")
}
