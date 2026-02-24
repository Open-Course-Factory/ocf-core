// tests/entityManagement/postgres_test_utils.go
package entityManagement_tests

import (
	"fmt"
	"os"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// PostgresTestConfig holds the configuration for PostgreSQL test database
type PostgresTestConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
}

// GetPostgresConfigFromEnv reads PostgreSQL configuration from environment variables
func GetPostgresConfigFromEnv() PostgresTestConfig {
	return PostgresTestConfig{
		Host:     getEnvOrDefault("POSTGRES_HOST", "localhost"),
		Port:     getEnvOrDefault("POSTGRES_PORT", "5432"),
		User:     getEnvOrDefault("POSTGRES_USER", "postgres"),
		Password: getEnvOrDefault("POSTGRES_PASSWORD", "postgres"),
		DBName:   getEnvOrDefault("POSTGRES_DB", "ocf_test"),
		SSLMode:  getEnvOrDefault("POSTGRES_SSLMODE", "disable"),
	}
}

// getEnvOrDefault returns the environment variable value or a default
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// BuildPostgresDSN creates a PostgreSQL DSN from config
func (c PostgresTestConfig) BuildDSN() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s connect_timeout=5",
		c.Host, c.Port, c.User, c.Password, c.DBName, c.SSLMode,
	)
}

// SetupPostgresTestDB creates a PostgreSQL test database connection
func SetupPostgresTestDB(t *testing.T) *gorm.DB {
	config := GetPostgresConfigFromEnv()
	dsn := config.BuildDSN()

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent), // Quiet mode for tests
	})

	if err != nil {
		t.Skipf("PostgreSQL not available: %v. Set POSTGRES_HOST to run these tests.", err)
		return nil
	}

	// Test connection
	sqlDB, err := db.DB()
	if err != nil {
		t.Skipf("Failed to get database instance: %v", err)
		return nil
	}

	if err := sqlDB.Ping(); err != nil {
		t.Skipf("PostgreSQL ping failed: %v", err)
		return nil
	}

	t.Logf("✅ Connected to PostgreSQL at %s:%s", config.Host, config.Port)
	return db
}

// CleanupPostgresTestDB drops all test tables
func CleanupPostgresTestDB(t *testing.T, db *gorm.DB, tables ...any) {
	if db == nil {
		return
	}

	// Drop tables in reverse order to handle foreign keys
	for i := len(tables) - 1; i >= 0; i-- {
		if err := db.Migrator().DropTable(tables[i]); err != nil {
			t.Logf("Warning: failed to drop table: %v", err)
		}
	}
}

// IsPostgresAvailable checks if PostgreSQL is available for testing
func IsPostgresAvailable() bool {
	config := GetPostgresConfigFromEnv()
	dsn := config.BuildDSN()

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})

	if err != nil {
		return false
	}

	sqlDB, err := db.DB()
	if err != nil {
		return false
	}

	if err := sqlDB.Ping(); err != nil {
		return false
	}

	sqlDB.Close()
	return true
}

// SkipIfNoPostgres skips the test if PostgreSQL is not available
func SkipIfNoPostgres(t *testing.T) {
	if !IsPostgresAvailable() {
		t.Skip("PostgreSQL not available. Set POSTGRES_HOST environment variable to run these tests.")
	}
}
