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
	terminalModels "soli/formations/src/terminalTrainer/models"

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
		&models.OrganizationRolePlan{},
		&organizationModels.Organization{},
		&organizationModels.OrganizationMember{},
		&configModels.Feature{},
		&models.SubscriptionBatch{},
		&groupModels.ClassGroup{},
		&groupModels.GroupMember{},
		&models.BillingAddress{},
		&models.PaymentMethod{},
	); err != nil {
		return err
	}

	// The SubscriptionPlan.Features model field was removed, but the raw
	// `features` column lives on in prod as an orphan (AutoMigrate never drops
	// it) and the group-management backfill still reads it. Re-create that
	// orphan column here so raw-column-seeding tests (backfill, legacy-string
	// guards) mirror prod.
	if !db.Migrator().HasColumn(&models.SubscriptionPlan{}, "features") {
		db.Exec("ALTER TABLE subscription_plans ADD COLUMN features TEXT")
	}

	// Create the partial unique index that enforces "at most one active
	// OrganizationSubscription per org" at the DB level.
	models.MigrateUniqueActiveOrgSubscriptionIndex(db)

	if err := db.AutoMigrate(&terminalModels.UserTerminalKey{}, &terminalModels.Terminal{}); err != nil {
		return err
	}
	if err := relaxTerminalColumnsForRawInserts(db); err != nil {
		return err
	}

	// Webhook events table (WebhookEvent model uses gen_random_uuid() which is PostgreSQL-only)
	// `status` column is part of the planned reservation-status fix (#261):
	// values are "reserved" / "processed" / "failed".
	db.Exec(`CREATE TABLE IF NOT EXISTS webhook_events (
		id TEXT PRIMARY KEY,
		event_id TEXT UNIQUE NOT NULL,
		event_type TEXT NOT NULL DEFAULT '',
		processed_at DATETIME NOT NULL,
		expires_at DATETIME NOT NULL,
		payload TEXT,
		status TEXT NOT NULL DEFAULT 'reserved',
		created_at DATETIME
	)`)

	return nil
}

// relaxTerminalColumnsForRawInserts loosens two columns of the AutoMigrated
// terminals table for the raw `INSERT INTO terminals (...)` statements in
// this package, which bind neither of them. SQLite cannot alter a column in
// place, so each is dropped and re-added:
//
//   - expires_at defaults to a far-future timestamp so an omitted value still
//     satisfies OccupiesSlotScope (`expires_at > NOW()`). Production rows
//     always carry an expires_at; the default is a test-only convenience.
//     See src/terminalTrainer/models/terminal.go::OccupiesSlotScope.
//   - user_terminal_key_id becomes nullable; the model declares it NOT NULL.
//     Its foreign key to user_terminal_keys goes first, since SQLite refuses
//     to drop a column that a constraint still references.
//
// Each drop rebuilds the table and loses every index, so the model's indexes
// are recreated at the end. A second AutoMigrate would not do: it re-tightens
// the two columns.
func relaxTerminalColumnsForRawInserts(db *gorm.DB) error {
	terminal := &terminalModels.Terminal{}
	if err := db.Migrator().DropConstraint(terminal, "fk_user_terminal_keys_terminals"); err != nil {
		return err
	}
	if err := db.Migrator().DropColumn(terminal, "expires_at"); err != nil {
		return err
	}
	if err := db.Exec("ALTER TABLE terminals ADD COLUMN expires_at DATETIME DEFAULT '2099-12-31 23:59:59'").Error; err != nil {
		return err
	}
	if err := db.Migrator().DropColumn(terminal, "user_terminal_key_id"); err != nil {
		return err
	}
	if err := db.Exec("ALTER TABLE terminals ADD COLUMN user_terminal_key_id TEXT").Error; err != nil {
		return err
	}
	for _, field := range []string{"SessionID", "UserID", "OrganizationID", "SubscriptionPlanID", "UserTerminalKeyID", "DeletedAt"} {
		if err := db.Migrator().CreateIndex(terminal, field); err != nil {
			return err
		}
	}
	return nil
}

// freshTestDB returns the shared DB after cleaning all rows.
// Safe because Go tests within a package run sequentially (no t.Parallel).
//
// IMPORTANT: tests in this package MUST NOT call t.Parallel(). The helper
// withoutUniqueActiveOrgSubIndex (in orgSubscriptionAssignment_test.go)
// temporarily drops the partial unique index on organization_subscriptions;
// any parallel test relying on that invariant would flake.
func freshTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	// Delete in dependency order to respect foreign keys
	sharedTestDB.Exec("DELETE FROM group_members")
	sharedTestDB.Exec("DELETE FROM class_groups")
	sharedTestDB.Exec("DELETE FROM subscription_batches")
	sharedTestDB.Exec("DELETE FROM usage_metrics")
	sharedTestDB.Exec("DELETE FROM user_subscriptions")
	sharedTestDB.Exec("DELETE FROM organization_subscriptions")
	sharedTestDB.Exec("DELETE FROM organization_role_plans")
	sharedTestDB.Exec("DELETE FROM organization_members")
	sharedTestDB.Exec("DELETE FROM organizations")
	sharedTestDB.Exec("DELETE FROM subscription_plans")
	sharedTestDB.Exec("DELETE FROM features")
	sharedTestDB.Exec("DELETE FROM billing_addresses")
	sharedTestDB.Exec("DELETE FROM payment_methods")
	sharedTestDB.Exec("DELETE FROM terminals")
	sharedTestDB.Exec("DELETE FROM user_terminal_keys")
	sharedTestDB.Exec("DELETE FROM webhook_events")
	sharedTestDB.Exec("DELETE FROM stripe_syncs")
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
