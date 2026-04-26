package authorization_tests

// These tests cover the GormEntityLoader against a real GORM-backed DB using
// the EXACT (Go-name) entity and field identifiers that production route
// declarations pass in. They reproduce production bug #273 — the loader
// builds raw SQL with Go names instead of translating them through GORM's
// NamingStrategy, so a route declaring `Entity: "Terminal", Field: "UserID"`
// runs `SELECT UserID FROM Terminal WHERE id = ?` against a database where
// the table is `terminals` and the column is `user_id`.
//
// All Layer 2 audit tests stub the loader; nothing else exercises the real
// `GormEntityLoader` against a real schema. Until the loader is fixed, these
// tests must FAIL.

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	access "soli/formations/src/auth/access"
	terminalModels "soli/formations/src/terminalTrainer/models"
)

// setupTerminalLoaderDB creates an in-memory SQLite DB and migrates the real
// Terminal model. GORM applies its default NamingStrategy on AutoMigrate, so
// the resulting table is `terminals` (snake_case + plural) and the resulting
// column is `user_id`.
func setupTerminalLoaderDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	err = db.AutoMigrate(&terminalModels.UserTerminalKey{}, &terminalModels.Terminal{})
	require.NoError(t, err)
	return db
}

// TestGormEntityLoader_ResolvesGoNameToTableName verifies that calling the
// loader with the Go struct name (e.g. "Terminal") and the Go field name
// (e.g. "UserID") — exactly as declared in route permissions — returns the
// stored owner value.
//
// This test currently FAILS because the loader sends raw Go names to SQL,
// producing `SELECT UserID FROM Terminal WHERE id = ?`, which SQLite rejects
// with "no such table: Terminal".
//
// This is the canonical reproduction of the production 403 bug on every
// EntityOwner-protected route (terminals, scenario sessions, subscription
// batches, ...).
func TestGormEntityLoader_ResolvesGoNameToTableName(t *testing.T) {
	db := setupTerminalLoaderDB(t)

	// Seed a UserTerminalKey first (Terminal has a NOT NULL FK to it).
	keyID, _ := uuid.NewV7()
	key := terminalModels.UserTerminalKey{
		UserID:      "user-owner-xyz",
		APIKey:      "api-key-xyz",
		KeyName:     "test-key",
		IsActive:    true,
		MaxSessions: 5,
	}
	key.ID = keyID
	require.NoError(t, db.Create(&key).Error)

	// Seed a Terminal owned by "user-owner-xyz".
	terminalID, _ := uuid.NewV7()
	terminal := terminalModels.Terminal{
		SessionID:         "session-abc",
		UserID:            "user-owner-xyz",
		Name:              "test terminal",
		Status:            "active",
		ExpiresAt:         time.Now().Add(time.Hour),
		UserTerminalKeyID: keyID,
	}
	terminal.ID = terminalID
	require.NoError(t, db.Create(&terminal).Error)

	loader := access.NewGormEntityLoader(db)

	// Call with the EXACT identifiers that
	// `src/terminalTrainer/routes/permissions.go` declares:
	//   AccessRule{Type: EntityOwner, Entity: "Terminal", Field: "UserID"}
	value, err := loader.GetOwnerField("Terminal", terminalID.String(), "UserID")
	assert.NoError(t, err,
		"loader must translate Go names to DB names via NamingStrategy "+
			"(this is the production bug — see gorm_entity_loader.go:46-48)")
	assert.Equal(t, "user-owner-xyz", value,
		"loader should return the stored UserID for the entity")
}

// TestGormEntityLoader_NamingStrategyContract is a lightweight contract test
// confirming what GORM's default NamingStrategy returns for the Terminal
// model. The fix relies on calling `db.NamingStrategy.TableName("Terminal")`
// and `db.NamingStrategy.ColumnName("", "UserID")` — this test pins the
// expected mapping so the fix can rely on it.
func TestGormEntityLoader_NamingStrategyContract(t *testing.T) {
	ns := schema.NamingStrategy{}

	assert.Equal(t, "terminals", ns.TableName("Terminal"),
		"NamingStrategy must map Go struct name 'Terminal' to table 'terminals'")
	assert.Equal(t, "user_id", ns.ColumnName("", "UserID"),
		"NamingStrategy must map Go field name 'UserID' to column 'user_id'")
	assert.Equal(t, "scenario_sessions", ns.TableName("ScenarioSession"),
		"NamingStrategy must map 'ScenarioSession' to 'scenario_sessions'")
	assert.Equal(t, "purchaser_user_id", ns.ColumnName("", "PurchaserUserID"),
		"NamingStrategy must map 'PurchaserUserID' to 'purchaser_user_id'")
}

// TestGormEntityLoader_NotFoundWithGoNames verifies the not-found behaviour
// AFTER the naming-strategy fix: a missing entity should return an error,
// not silently succeed.
//
// Today this test also fails (the same naming bug aborts before the
// not-found path is reached), but post-fix it must continue to pass —
// pinning the not-found contract.
func TestGormEntityLoader_NotFoundWithGoNames(t *testing.T) {
	db := setupTerminalLoaderDB(t)

	loader := access.NewGormEntityLoader(db)

	missingID, _ := uuid.NewV7()
	_, err := loader.GetOwnerField("Terminal", missingID.String(), "UserID")
	assert.Error(t, err,
		"loader must return an error when the entity does not exist, "+
			"not an empty owner string (which would silently grant access)")
}
