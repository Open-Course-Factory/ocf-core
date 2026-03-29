// tests/payment/main_test.go
// Shared test infrastructure for payment tests.
// Migrates tables ONCE and reuses the DB across all tests,
// cleaning rows between tests for isolation.
package payment_tests

import (
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
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		panic("failed to open shared test DB: " + err.Error())
	}

	// Migrate all tables needed by any payment test
	err = db.AutoMigrate(
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
	)
	if err != nil {
		panic("failed to migrate shared test DB: " + err.Error())
	}

	// Create tables with PostgreSQL-specific defaults using raw SQL for SQLite compatibility
	// Terminal table (has no PostgreSQL-specific defaults but isn't in the payment models package)
	db.Exec(`CREATE TABLE IF NOT EXISTS terminals (
		id TEXT PRIMARY KEY,
		created_at DATETIME,
		updated_at DATETIME,
		deleted_at DATETIME,
		session_id TEXT UNIQUE,
		user_id TEXT NOT NULL,
		name TEXT,
		status TEXT DEFAULT 'active',
		expires_at DATETIME,
		user_terminal_key_id TEXT,
		owner_ids TEXT
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

	sharedTestDB = db
	os.Exit(m.Run())
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
	sharedTestDB.Exec("DELETE FROM webhook_events")
	return sharedTestDB
}
