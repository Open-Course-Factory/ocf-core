package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/joho/godotenv"

	"soli/formations/src/auth/casdoor"
)

// MarkExistingUsersVerified marks all existing users as email verified
// This is a one-time migration script for the grandfather clause
func main() {
	// Load environment variables
	envFile := ".env"
	err := godotenv.Load(envFile)
	if err != nil {
		log.Printf("Warning: Could not load .env file: %v\n", err)
	}

	// Initialize Casdoor connection
	casdoor.InitCasdoorConnection("", envFile)

	fmt.Println("🔄 Starting migration: Marking all existing users as email verified...")

	// Get all users from Casdoor
	users, err := casdoorsdk.GetUsers()
	if err != nil {
		log.Fatalf("❌ Failed to get users from Casdoor: %v\n", err)
	}

	markedCount := 0
	skippedCount := 0
	errorCount := 0
	verificationTime := time.Now().Format(time.RFC3339)

	for _, user := range users {
		// Skip deleted users
		if user.IsDeleted {
			skippedCount++
			continue
		}

		// Only mark if not already verified (using native Casdoor field)
		if !user.EmailVerified {
			user.EmailVerified = true

			// Initialize Properties map if nil
			if user.Properties == nil {
				user.Properties = make(map[string]string)
			}
			user.Properties["email_verified_at"] = verificationTime

			// Update user in Casdoor
			_, err := casdoorsdk.UpdateUser(user)
			if err != nil {
				log.Printf("Failed to update user %s (%s): %v\n", user.Name, user.Email, err)
				errorCount++
			} else {
				fmt.Printf("Marked user %s (%s) as verified\n", user.Name, user.Email)
				markedCount++
			}
		} else {
			fmt.Printf("User %s (%s) already verified, skipping\n", user.Name, user.Email)
			skippedCount++
		}
	}

	fmt.Println("\n📊 Migration Summary:")
	fmt.Printf("   ✅ Users marked as verified: %d\n", markedCount)
	fmt.Printf("   ⏭️  Users skipped (already verified or deleted): %d\n", skippedCount)
	fmt.Printf("   ❌ Errors: %d\n", errorCount)
	fmt.Println("\n✅ Migration completed!")

	if errorCount > 0 {
		os.Exit(1)
	}
}
