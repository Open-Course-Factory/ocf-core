package scenarios_test

import (
	"os"
	"testing"

	groupModels "soli/formations/src/groups/models"
	orgModels "soli/formations/src/organizations/models"
	"soli/formations/src/scenarios/models"
	terminalModels "soli/formations/src/terminalTrainer/models"

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

	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)

	err = db.AutoMigrate(
		&models.ProjectFile{},
		&models.Scenario{},
		&models.ScenarioStep{},
		&models.ScenarioStepHint{},
		&models.ScenarioSession{},
		&models.ScenarioStepProgress{},
		&models.ScenarioFlag{},
		&models.ScenarioAssignment{},
		&groupModels.ClassGroup{},
		&groupModels.GroupMember{},
		&terminalModels.Terminal{},
		&terminalModels.UserTerminalKey{},
		&orgModels.Organization{},
		&orgModels.OrganizationMember{},
	)
	if err != nil {
		panic("failed to migrate shared test DB: " + err.Error())
	}

	sharedTestDB = db
	os.Exit(m.Run())
}

func freshTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	// Delete in reverse-dependency order to avoid FK issues
	sharedTestDB.Exec("DELETE FROM scenario_step_progress")
	sharedTestDB.Exec("DELETE FROM scenario_flags")
	sharedTestDB.Exec("DELETE FROM scenario_sessions")
	sharedTestDB.Exec("DELETE FROM scenario_assignments")
	sharedTestDB.Exec("DELETE FROM scenario_step_hints")
	sharedTestDB.Exec("DELETE FROM scenario_steps")
	sharedTestDB.Exec("DELETE FROM scenarios")
	sharedTestDB.Exec("DELETE FROM project_files")
	sharedTestDB.Exec("DELETE FROM group_members")
	sharedTestDB.Exec("DELETE FROM class_groups")
	sharedTestDB.Exec("DELETE FROM organization_members")
	sharedTestDB.Exec("DELETE FROM organizations")
	sharedTestDB.Exec("DELETE FROM terminals")
	sharedTestDB.Exec("DELETE FROM user_terminal_keys")
	return sharedTestDB
}
