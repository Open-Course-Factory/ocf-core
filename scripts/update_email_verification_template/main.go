package main

import (
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"

	sqldb "soli/formations/src/db"
	emailModels "soli/formations/src/email/models"
	emailServices "soli/formations/src/email/services"
)

func main() {
	// Load environment variables
	envFile := ".env"
	err := godotenv.Load(envFile)
	if err != nil {
		log.Printf("Warning: Could not load .env file: %v\n", err)
	}

	// Initialize database connection
	sqldb.InitDBConnection(envFile)

	fmt.Println("🔄 Updating email verification template...")

	// Permanently delete the old template (including soft-deleted ones)
	result := sqldb.DB.Unscoped().Where("name = ?", "email_verification").Delete(&emailModels.EmailTemplate{})
	if result.Error != nil {
		log.Fatalf("❌ Failed to delete old template: %v\n", result.Error)
	}

	if result.RowsAffected > 0 {
		fmt.Printf("✅ Deleted old email_verification template (%d rows)\n", result.RowsAffected)
	} else {
		fmt.Println("ℹ️  No existing template found")
	}

	// Reinitialize templates (this will create the new version)
	emailServices.InitDefaultTemplates(sqldb.DB)

	fmt.Println("✅ Email verification template updated successfully!")
	fmt.Println("The new template now includes the verification token for manual copy/paste.")

	os.Exit(0)
}
