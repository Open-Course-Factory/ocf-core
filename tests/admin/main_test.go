package admin_test

import (
	"os"
	"testing"

	"gorm.io/gorm"

	auditModels "soli/formations/src/audit/models"
	groupModels "soli/formations/src/groups/models"
	orgModels "soli/formations/src/organizations/models"
	terminalModels "soli/formations/src/terminalTrainer/models"
	testTools "soli/formations/tests/testTools"
)

var sharedTestDB *gorm.DB

func TestMain(m *testing.M) {
	sharedTestDB = testTools.MemoryDB(
		&orgModels.Organization{},
		&orgModels.OrganizationMember{},
		&groupModels.ClassGroup{},
		&groupModels.GroupMember{},
		&auditModels.AuditLog{},
		&terminalModels.UserTerminalKey{},
		&terminalModels.Terminal{},
	)
	// Metadata is a jsonb column SQLite cannot write; the provisioning path
	// creates a personal organization with raw GORM, so omit it there too.
	if err := sharedTestDB.Callback().Create().Before("gorm:create").
		Register("admin_omit_metadata_for_sqlite", func(tx *gorm.DB) {
			if tx.Statement == nil || tx.Statement.Schema == nil {
				return
			}
			table := tx.Statement.Schema.Table
			if table == "organizations" || table == "organization_members" {
				tx.Statement.Omits = append(tx.Statement.Omits, "Metadata")
			}
		}); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

func freshTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	testTools.Truncate(sharedTestDB, "group_members", "class_groups", "organization_members", "organizations", "audit_logs", "terminals", "user_terminal_keys")
	return sharedTestDB
}
