package test_tools

import (
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// MemoryDB opens a private in-memory SQLite DB and migrates the given models.
// Meant for TestMain, which has no testing.TB, hence the panic on failure.
func MemoryDB(models ...any) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		panic("failed to open test DB: " + err.Error())
	}
	if err := db.AutoMigrate(models...); err != nil {
		panic("failed to migrate test DB: " + err.Error())
	}
	return db
}

// Truncate hard-deletes every row of the given tables, in the order given
// (callers list children before parents to respect foreign keys). Plain SQL
// on purpose: GORM's Delete would only soft-delete models with DeletedAt.
func Truncate(db *gorm.DB, tables ...string) error {
	for _, table := range tables {
		if err := db.Exec("DELETE FROM " + table).Error; err != nil {
			return err
		}
	}
	return nil
}
