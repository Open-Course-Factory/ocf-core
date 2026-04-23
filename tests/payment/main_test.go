// tests/payment/main_test.go
// Shared test infrastructure for payment tests.
// Migrates tables ONCE and reuses the DB across all tests,
// cleaning rows between tests for isolation.
package payment_tests

import (
	"fmt"
	"os"
	"testing"

	configModels "soli/formations/src/configuration/models"
	groupModels "soli/formations/src/groups/models"
	organizationModels "soli/formations/src/organizations/models"
	"soli/formations/src/payment/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var sharedTestDB *gorm.DB

func TestMain(m *testing.M) {
	// SQLite ":memory:" is per-connection by default: a second connection
	// from the Go sql.DB pool would point at a DIFFERENT in-memory DB that
	// doesn't have our tables. Using file::memory:?cache=shared keeps a
	// single shared in-memory DB across all connections in the process,
	// which is what we need for concurrent webhook tests (goroutines
	// opening new connections from the pool).
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&_busy_timeout=5000"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		panic("failed to open shared test DB: " + err.Error())
	}

	if err := runTestMigrations(db); err != nil {
		panic("failed to migrate shared test DB: " + err.Error())
	}

	sharedTestDB = db
	os.Exit(m.Run())
}

// runTestMigrations applies the full payment-test schema to the given DB.
// Extracted so both the shared DB and per-test isolated DBs (see
// cappedTestDB) can reuse the same migration logic.
func runTestMigrations(db *gorm.DB) error {
	// Migrate all tables needed by any payment test
	if err := db.AutoMigrate(
		&models.SubscriptionPlan{},
		&models.UserSubscription{},
		&models.UsageMetrics{},
		&models.OrganizationSubscription{},
		&organizationModels.Organization{},
		&organizationModels.OrganizationMember{},
		&models.PlanFeature{},
		&configModels.Feature{},
		&models.SubscriptionBatch{},
		&groupModels.ClassGroup{},
		&groupModels.GroupMember{},
		&models.BillingAddress{},
		&models.PaymentMethod{},
	); err != nil {
		return err
	}

	// Create tables with PostgreSQL-specific defaults using raw SQL for SQLite compatibility
	// UserTerminalKey table (referenced by Terminal via foreign key)
	db.Exec(`CREATE TABLE IF NOT EXISTS user_terminal_keys (
		id TEXT PRIMARY KEY,
		created_at DATETIME,
		updated_at DATETIME,
		deleted_at DATETIME,
		owner_ids TEXT,
		user_id TEXT NOT NULL,
		api_key TEXT NOT NULL,
		key_name TEXT NOT NULL,
		is_active BOOLEAN DEFAULT true,
		max_sessions INTEGER DEFAULT 5
	)`)

	// Terminal table (has no PostgreSQL-specific defaults but isn't in the payment models package)
	db.Exec(`CREATE TABLE IF NOT EXISTS terminals (
		id TEXT PRIMARY KEY,
		created_at DATETIME,
		updated_at DATETIME,
		deleted_at DATETIME,
		owner_ids TEXT,
		session_id TEXT UNIQUE,
		user_id TEXT NOT NULL,
		name TEXT,
		status TEXT DEFAULT 'active',
		expires_at DATETIME,
		instance_type TEXT,
		machine_size TEXT,
		backend TEXT DEFAULT '',
		organization_id TEXT,
		subscription_plan_id TEXT,
		user_terminal_key_id TEXT REFERENCES user_terminal_keys(id),
		is_hidden_by_owner BOOLEAN DEFAULT false,
		hidden_by_owner_at DATETIME,
		composed_distribution TEXT,
		composed_size TEXT,
		composed_features TEXT
	)`)

	// Webhook events table (WebhookEvent model uses gen_random_uuid() which is PostgreSQL-only)
	db.Exec(`CREATE TABLE IF NOT EXISTS webhook_events (
		id TEXT PRIMARY KEY,
		event_id TEXT UNIQUE NOT NULL,
		event_type TEXT NOT NULL DEFAULT '',
		processed_at DATETIME NOT NULL,
		expires_at DATETIME NOT NULL,
		payload TEXT,
		created_at DATETIME
	)`)

	return nil
}

// freshTestDB returns the shared DB after cleaning all rows.
// Safe because Go tests within a package run sequentially (no t.Parallel).
func freshTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	// Delete in dependency order to respect foreign keys
	sharedTestDB.Exec("DELETE FROM group_members")
	sharedTestDB.Exec("DELETE FROM class_groups")
	sharedTestDB.Exec("DELETE FROM subscription_batches")
	sharedTestDB.Exec("DELETE FROM usage_metrics")
	sharedTestDB.Exec("DELETE FROM user_subscriptions")
	sharedTestDB.Exec("DELETE FROM organization_subscriptions")
	sharedTestDB.Exec("DELETE FROM organization_members")
	sharedTestDB.Exec("DELETE FROM organizations")
	sharedTestDB.Exec("DELETE FROM subscription_plans")
	sharedTestDB.Exec("DELETE FROM plan_features")
	sharedTestDB.Exec("DELETE FROM features")
	sharedTestDB.Exec("DELETE FROM billing_addresses")
	sharedTestDB.Exec("DELETE FROM payment_methods")
	sharedTestDB.Exec("DELETE FROM terminals")
	sharedTestDB.Exec("DELETE FROM user_terminal_keys")
	sharedTestDB.Exec("DELETE FROM webhook_events")
	return sharedTestDB
}

// cappedTestDB returns an isolated in-memory SQLite DB with MaxOpenConns=1.
// Use this for tests that exercise concurrent writers (e.g. webhook race) —
// the shared sharedTestDB can't use MaxOpenConns=1 globally because some
// tests use internal transactions that would deadlock on a single-connection
// pool (notably TestCheckLimit_UsesContextPlan_SkipsPlanResolution).
//
// Each invocation must use a unique `name` (e.g. t.Name()) — the DSN encodes
// it so every caller gets an isolated in-memory DB and cannot pollute the
// shared schema.
func cappedTestDB(t *testing.T, name string) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_busy_timeout=5000", name)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("cappedTestDB: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("cappedTestDB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := runTestMigrations(db); err != nil {
		t.Fatalf("cappedTestDB migrate: %v", err)
	}
	return db
}
