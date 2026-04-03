package audit_tests

import (
	"os"
	"testing"

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
		panic("failed to open test DB: " + err.Error())
	}

	// Create audit_logs table manually because the AuditLog model uses PostgreSQL-specific
	// defaults (gen_random_uuid()) that SQLite cannot parse via AutoMigrate.
	err = db.Exec(`CREATE TABLE IF NOT EXISTS audit_logs (
		id TEXT PRIMARY KEY,
		event_type TEXT NOT NULL,
		severity TEXT NOT NULL,
		actor_id TEXT,
		actor_email TEXT,
		actor_ip TEXT,
		actor_user_agent TEXT,
		target_id TEXT,
		target_type TEXT,
		target_name TEXT,
		organization_id TEXT,
		action TEXT NOT NULL,
		status TEXT NOT NULL,
		error_message TEXT,
		metadata TEXT,
		amount REAL,
		currency TEXT,
		request_id TEXT,
		session_id TEXT,
		created_at DATETIME NOT NULL,
		expires_at DATETIME NOT NULL
	)`).Error
	if err != nil {
		panic("failed to create audit_logs table: " + err.Error())
	}

	sharedTestDB = db
	os.Exit(m.Run())
}

func freshTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	sharedTestDB.Exec("DELETE FROM audit_logs")
	return sharedTestDB
}
