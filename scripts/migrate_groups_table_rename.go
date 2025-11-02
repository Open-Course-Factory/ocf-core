package main

import (
	"fmt"
	"log"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Migration script to rename 'groups' table to 'class_groups'
// This standardizes naming across the codebase: ClassGroup model → class_groups table → /api/v1/class-groups routes
//
// Run with: go run scripts/migrate_groups_table_rename.go
//
// What this does:
// 1. Renames 'groups' table to 'class_groups'
// 2. PostgreSQL automatically updates foreign key references
// 3. Verifies the migration succeeded
//
// Rollback: ALTER TABLE class_groups RENAME TO groups;

func main() {
	// Get database connection from environment
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "host=postgres user=ocf_user password=yourpassword dbname=ocf_database port=5432 sslmode=disable"
		log.Println("⚠️  DATABASE_URL not set, using default connection string")
	}

	// Connect to database
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		log.Fatalf("❌ Failed to connect to database: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("❌ Failed to get underlying DB: %v", err)
	}
	defer sqlDB.Close()

	log.Println("✅ Connected to database")

	// Check if migration is needed
	var tableExists bool
	err = db.Raw("SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'groups')").Scan(&tableExists).Error
	if err != nil {
		log.Fatalf("❌ Failed to check if 'groups' table exists: %v", err)
	}

	if !tableExists {
		log.Println("ℹ️  Table 'groups' does not exist - checking if migration already ran...")

		var newTableExists bool
		err = db.Raw("SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'class_groups')").Scan(&newTableExists).Error
		if err != nil {
			log.Fatalf("❌ Failed to check if 'class_groups' table exists: %v", err)
		}

		if newTableExists {
			log.Println("✅ Migration already completed - 'class_groups' table exists")
			os.Exit(0)
		} else {
			log.Fatalf("❌ Neither 'groups' nor 'class_groups' table exists - database state is unexpected")
		}
	}

	log.Println("🔍 Pre-migration verification...")

	// Count rows in groups table
	var groupCount int64
	err = db.Raw("SELECT COUNT(*) FROM groups").Scan(&groupCount).Error
	if err != nil {
		log.Fatalf("❌ Failed to count groups: %v", err)
	}
	log.Printf("📊 Found %d groups in 'groups' table", groupCount)

	// Check for foreign key constraints
	var fkCount int64
	err = db.Raw(`
		SELECT COUNT(*)
		FROM information_schema.table_constraints
		WHERE constraint_type = 'FOREIGN KEY'
		AND table_name = 'groups'
	`).Scan(&fkCount).Error
	if err != nil {
		log.Fatalf("❌ Failed to count foreign keys: %v", err)
	}
	log.Printf("🔗 Found %d foreign key constraints on 'groups' table", fkCount)

	// Prompt for confirmation
	fmt.Println("\n⚠️  This will rename the 'groups' table to 'class_groups'")
	fmt.Println("⚠️  PostgreSQL will automatically update foreign key references")
	fmt.Println("⚠️  Make sure you have a database backup!")
	fmt.Print("\nType 'YES' to continue: ")

	var confirmation string
	fmt.Scanln(&confirmation)

	if confirmation != "YES" {
		log.Println("❌ Migration cancelled")
		os.Exit(1)
	}

	log.Println("\n🚀 Starting migration...")

	// Begin transaction
	tx := db.Begin()
	if tx.Error != nil {
		log.Fatalf("❌ Failed to start transaction: %v", tx.Error)
	}

	// Rename the table
	err = tx.Exec("ALTER TABLE groups RENAME TO class_groups").Error
	if err != nil {
		tx.Rollback()
		log.Fatalf("❌ Failed to rename table: %v", err)
	}
	log.Println("✅ Renamed 'groups' table to 'class_groups'")

	// Commit transaction
	err = tx.Commit().Error
	if err != nil {
		log.Fatalf("❌ Failed to commit transaction: %v", err)
	}

	log.Println("\n🔍 Post-migration verification...")

	// Verify new table exists
	var newTableExists bool
	err = db.Raw("SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'class_groups')").Scan(&newTableExists).Error
	if err != nil || !newTableExists {
		log.Fatalf("❌ Verification failed: 'class_groups' table does not exist")
	}
	log.Println("✅ 'class_groups' table exists")

	// Verify row count matches
	var newGroupCount int64
	err = db.Raw("SELECT COUNT(*) FROM class_groups").Scan(&newGroupCount).Error
	if err != nil {
		log.Fatalf("❌ Failed to count class_groups: %v", err)
	}

	if newGroupCount != groupCount {
		log.Fatalf("❌ Row count mismatch: expected %d, got %d", groupCount, newGroupCount)
	}
	log.Printf("✅ Row count verified: %d groups", newGroupCount)

	// Verify foreign key constraints were updated
	var newFkCount int64
	err = db.Raw(`
		SELECT COUNT(*)
		FROM information_schema.table_constraints
		WHERE constraint_type = 'FOREIGN KEY'
		AND table_name = 'class_groups'
	`).Scan(&newFkCount).Error
	if err != nil {
		log.Fatalf("❌ Failed to count foreign keys on new table: %v", err)
	}

	if newFkCount != fkCount {
		log.Printf("⚠️  Foreign key count changed: %d → %d (PostgreSQL may have renamed some)", fkCount, newFkCount)
	} else {
		log.Printf("✅ Foreign key constraints verified: %d constraints", newFkCount)
	}

	// List all tables that reference class_groups via foreign keys
	type FKReference struct {
		TableName      string
		ConstraintName string
		ColumnName     string
	}
	var references []FKReference
	err = db.Raw(`
		SELECT
			tc.table_name,
			tc.constraint_name,
			kcu.column_name
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
			ON tc.constraint_name = kcu.constraint_name
		JOIN information_schema.constraint_column_usage ccu
			ON ccu.constraint_name = tc.constraint_name
		WHERE tc.constraint_type = 'FOREIGN KEY'
		AND ccu.table_name = 'class_groups'
		ORDER BY tc.table_name
	`).Scan(&references).Error

	if err != nil {
		log.Printf("⚠️  Could not list foreign key references: %v", err)
	} else if len(references) > 0 {
		log.Println("\n📋 Tables referencing 'class_groups':")
		for _, ref := range references {
			log.Printf("   - %s.%s (constraint: %s)", ref.TableName, ref.ColumnName, ref.ConstraintName)
		}
	}

	log.Println("\n✅ Migration completed successfully!")
	log.Println("\n📝 Next steps:")
	log.Println("   1. Update code: ClassGroup.TableName() to return 'class_groups'")
	log.Println("   2. Update JOIN queries that reference the old table name")
	log.Println("   3. Deploy updated backend code")
	log.Println("\n💡 Rollback command (if needed):")
	log.Println("   ALTER TABLE class_groups RENAME TO groups;")
}
