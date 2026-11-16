package payment_tests

import (
	"bytes"
	"fmt"
	"log"
	"testing"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	courseModels "soli/formations/src/courses/models"
	"soli/formations/src/initialization"
)

// A failed AutoMigrate at startup must be logged with the model it concerns,
// otherwise the API boots on a schema it does not match and nobody knows why.
func TestAutoMigrateAll_ModelMigrationFails_LogsErrorNamingModel(t *testing.T) {
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	// A view squatting the table name makes CREATE TABLE pages fail, and only that.
	require.NoError(t, db.Exec("CREATE VIEW pages AS SELECT 1 AS id").Error)

	// AutoMigrateAll ends with Casdoor syncs; an unreachable endpoint makes them log and return.
	casdoorsdk.InitConfig("http://localhost:0", "dummy", "dummy", "dummy", "dummy", "dummy")

	var logs bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(prev) })

	initialization.AutoMigrateAll(db)

	require.Error(t, db.AutoMigrate(&courseModels.Page{}), "precondition: the Page migration really fails")
	assert.Contains(t, logs.String(), "AutoMigrate *models.Page failed",
		"the failed migration of the Page model must be logged and name the model; logs were:\n%s", logs.String())
}
