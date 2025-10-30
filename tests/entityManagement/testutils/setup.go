package testutils

import (
	"testing"

	"soli/formations/src/entityManagement/hooks"
	entityInterfaces "soli/formations/src/entityManagement/interfaces"

	ems "soli/formations/src/entityManagement/entityManagementService"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestContext holds all necessary resources for entity tests
type TestContext struct {
	DB       *gorm.DB
	Entity   any
	Cleanup  func()
}

// SetupEntityTest creates a complete test environment for entity testing.
//
// This helper function:
//   - Creates an in-memory SQLite database
//   - Initializes entity registry
//   - Sets up hook system in test mode
//   - Provides cleanup function
//
// Usage:
//
//	func TestMyEntity(t *testing.T) {
//	    ctx := testutils.SetupEntityTest(t, &MyEntity{})
//	    defer ctx.Cleanup()
//
//	    // Your test code here
//	}
func SetupEntityTest(t *testing.T, entity any) *TestContext {
	t.Helper()

	// Setup in-memory database
	db := SetupTestDB(t)

	// Setup entity registry
	ems.GlobalEntityRegistrationService = ems.NewEntityRegistrationService()

	// Setup hooks in test mode
	hooks.GlobalHookRegistry = hooks.NewHookRegistry()
	hooks.GlobalHookRegistry.SetTestMode(true)

	cleanup := func() {
		CleanupTestDB(db)
	}

	return &TestContext{
		DB:      db,
		Entity:  entity,
		Cleanup: cleanup,
	}
}

// SetupTestDB creates an in-memory SQLite database for testing.
func SetupTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to setup test DB: %v", err)
	}

	return db
}

// CleanupTestDB closes database connections and cleans up resources.
func CleanupTestDB(db *gorm.DB) {
	sqlDB, err := db.DB()
	if err == nil {
		sqlDB.Close()
	}
}

// RegisterTestEntity registers an entity with the global registry.
//
// This is a helper for tests that need to register entities.
//
// Usage:
//
//	RegisterTestEntity(t, MyEntityRegistration{})
func RegisterTestEntity(t *testing.T, registration entityInterfaces.RegistrableInterface) {
	t.Helper()

	ems.GlobalEntityRegistrationService.RegisterEntity(registration)
}

// CreateTestEntity creates and saves a test entity to the database.
//
// Usage:
//
//	entity := CreateTestEntity(t, ctx.DB, &Course{Title: "Test Course"})
func CreateTestEntity(t *testing.T, db *gorm.DB, entity any) any {
	t.Helper()

	result := db.Create(entity)
	if result.Error != nil {
		t.Fatalf("Failed to create test entity: %v", result.Error)
	}

	return entity
}

// MigrateTestEntity runs auto-migration for a test entity.
//
// Usage:
//
//	MigrateTestEntity(t, ctx.DB, &Course{}, &Chapter{})
func MigrateTestEntity(t *testing.T, db *gorm.DB, entities ...any) {
	t.Helper()

	err := db.AutoMigrate(entities...)
	if err != nil {
		t.Fatalf("Failed to migrate test entities: %v", err)
	}
}
