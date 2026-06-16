package terminalTrainer_tests

import (
	"os"
	"testing"

	configModels "soli/formations/src/configuration/models"
	groupModels "soli/formations/src/groups/models"
	orgModels "soli/formations/src/organizations/models"
	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/terminalTrainer/models"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var sharedTestDB *gorm.DB

// terminalTestModels is the full set of tables any terminalTrainer test may
// touch. Kept as a single list so the SQLite TestMain migration and the
// optional PostgreSQL race harness migrate IDENTICAL schemas — a column the
// budget sum reads must exist on both drivers or the race test would diverge
// from production behaviour for the wrong reason.
var terminalTestModels = []any{
	&models.UserTerminalKey{},
	&models.Terminal{},
	&groupModels.ClassGroup{},
	&groupModels.GroupMember{},
	&orgModels.Organization{},
	&orgModels.OrganizationMember{},
	&paymentModels.SubscriptionPlan{},
	&paymentModels.OrganizationSubscription{},
	&paymentModels.OrganizationRolePlan{},
	&paymentModels.UserSubscription{},
	&paymentModels.UsageMetrics{},
	&configModels.Feature{},
}

func TestMain(m *testing.M) {
	// Initialize Casdoor SDK with a dummy config so package-level lookup
	// functions don't panic on a nil client when production code calls
	// casdoorsdk.GetUserByUserId. The HTTP call will fail gracefully and
	// the code falls back to its userID/email fallback chain — which is
	// the expected behaviour under unit tests that don't stand up a real
	// Casdoor server. Tests that need a specific Casdoor record swap the
	// per-feature seam (e.g. services.LookupCasdoorUserForOrgUsage).
	casdoorsdk.InitConfig("http://localhost:0", "dummy-endpoint", "dummy-client", "dummy-secret", "dummy-org", "dummy-app")

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		panic("failed to open shared test DB: " + err.Error())
	}

	// Migrate all tables needed by any terminalTrainer test
	err = db.AutoMigrate(terminalTestModels...)
	if err != nil {
		panic("failed to migrate shared test DB: " + err.Error())
	}

	sharedTestDB = db
	os.Exit(m.Run())
}

// sharedTestPGDB returns a PostgreSQL-backed *gorm.DB for tests that need
// REAL concurrency (SQLite serialises writers, so a race that only
// reproduces under true parallelism is invisible on the in-memory SQLite
// sharedTestDB). It connects only when TEST_PG_DSN is set, AutoMigrates the
// SAME terminalTestModels the SQLite path uses, and TRUNCATEs every table so
// reruns are deterministic. Tests skip when no DSN is configured.
//
// The DSN is a full lib/pq connection string, e.g.
//
//	TEST_PG_DSN="host=127.0.0.1 port=55432 user=postgres password=test dbname=ocf_race_test sslmode=disable"
func sharedTestPGDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := os.Getenv("TEST_PG_DSN")
	if dsn == "" {
		t.Skip("TEST_PG_DSN not set — skipping PostgreSQL concurrency test (SQLite cannot reproduce the race)")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
		// The SQLite test schema (TestMain) does not create cross-table FK
		// constraints; mirror that here so AutoMigrate doesn't fail on the
		// model declaration order (e.g. terminals→user_terminal_keys,
		// organization_members→organizations). The budget sum the race test
		// exercises never relies on referential integrity — it sums denormalised
		// columns under OccupiesSlotScope.
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		t.Skipf("PostgreSQL not reachable via TEST_PG_DSN: %v", err)
	}

	if err := db.AutoMigrate(terminalTestModels...); err != nil {
		t.Fatalf("failed to migrate PostgreSQL test schema: %v", err)
	}

	truncateAllPGTables(t, db)
	return db
}

// truncateAllPGTables resets every table the budget sum or composer touches
// to a clean state. TRUNCATE ... CASCADE is used so reruns within a single
// `go test` invocation never see rows from a prior test.
func truncateAllPGTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.Exec(`TRUNCATE TABLE
		terminals,
		user_terminal_keys,
		group_members,
		class_groups,
		organization_members,
		organizations,
		organization_subscriptions,
		organization_role_plans,
		user_subscriptions,
		usage_metrics,
		features
		RESTART IDENTITY CASCADE`).Error; err != nil {
		t.Fatalf("failed to truncate PostgreSQL test tables: %v", err)
	}
}

// freshTestDB returns the shared DB after cleaning all rows.
// Safe because Go tests within a package run sequentially (no t.Parallel).
func freshTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	// Delete in dependency order to respect foreign keys
	sharedTestDB.Exec("DELETE FROM terminals")
	sharedTestDB.Exec("DELETE FROM user_terminal_keys")
	sharedTestDB.Exec("DELETE FROM group_members")
	sharedTestDB.Exec("DELETE FROM class_groups")
	sharedTestDB.Exec("DELETE FROM organization_members")
	sharedTestDB.Exec("DELETE FROM organization_subscriptions")
	sharedTestDB.Exec("DELETE FROM organization_role_plans")
	sharedTestDB.Exec("DELETE FROM user_subscriptions")
	sharedTestDB.Exec("DELETE FROM usage_metrics")
	sharedTestDB.Exec("DELETE FROM organizations")
	sharedTestDB.Exec("DELETE FROM subscription_plans")
	sharedTestDB.Exec("DELETE FROM features")
	return sharedTestDB
}
