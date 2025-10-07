// scripts/seed_terminal_plans.go
// Run with: go run scripts/seed_terminal_plans.go
package main

import (
	"fmt"
	"log"

	sqldb "soli/formations/src/db"
	paymentModels "soli/formations/src/payment/models"

	"github.com/joho/godotenv"
	"gorm.io/gorm"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	// Initialize database
	sqldb.InitDBConnection("")
	db := sqldb.DB

	// Run migration first to ensure new fields exist
	fmt.Println("🔄 Running AutoMigrate for SubscriptionPlan...")
	if err := db.AutoMigrate(&paymentModels.SubscriptionPlan{}); err != nil {
		log.Fatalf("❌ Migration failed: %v", err)
	}
	fmt.Println("✅ Migration completed")

	// Define the 4 plans (updated with Terminal Trainer size naming: XS, S, M, L, XL)
	plans := []paymentModels.SubscriptionPlan{
		// 1. Trial (Free)
		{
			Name:        "Trial",
			Description: "Free plan for testing the platform. 1 hour sessions, no network or storage. Perfect for trying out terminals.",
			PriceAmount: 0,
			Currency:    "eur",
			BillingInterval: "month",
			TrialDays:   0,
			Features: []string{
				"Unlimited restarts",
				"1 hour max session",
				"1 concurrent terminal",
				"XS machine only",
				"No network access",
				"No data persistence",
			},
			MaxConcurrentUsers: 1,
			MaxCourses:         0,
			MaxLabSessions:     -1, // Legacy field - not used for terminals
			IsActive:           true,
			RequiredRole:       "member",
			StripeCreated:      false,

			// Terminal-specific limits
			MaxSessionDurationMinutes: 60,  // 1 hour
			MaxConcurrentTerminals:    1,   // Only 1 at a time
			AllowedMachineSizes:       []string{"XS"}, // XS size only
			NetworkAccessEnabled:      false,
			DataPersistenceEnabled:    false,
			DataPersistenceGB:         0,
			AllowedTemplates:          []string{"ubuntu-basic", "alpine-basic"},
		},

		// 2. Solo (€9/mo)
		{
			Name:        "Solo",
			Description: "Perfect for individual learning. Full 8-hour sessions with network and storage for personal projects.",
			PriceAmount: 900, // €9.00
			Currency:    "eur",
			BillingInterval: "month",
			TrialDays:   0,
			Features: []string{
				"Unlimited restarts",
				"8 hour max session",
				"1 concurrent terminal",
				"XS + S machines",
				"Network access included",
				"2GB data persistence",
				"All standard templates",
			},
			MaxConcurrentUsers: 1,
			MaxCourses:         0,
			MaxLabSessions:     -1,
			IsActive:           true,
			RequiredRole:       "member",
			StripeCreated:      false,

			// Terminal-specific limits
			MaxSessionDurationMinutes: 480, // 8 hours
			MaxConcurrentTerminals:    1,
			AllowedMachineSizes:       []string{"XS", "S"}, // XS + S sizes
			NetworkAccessEnabled:      true,  // Key upgrade from Free
			DataPersistenceEnabled:    true,  // Key upgrade from Free
			DataPersistenceGB:         2,
			AllowedTemplates: []string{
				"ubuntu-basic", "ubuntu-dev", "alpine-basic",
				"debian-basic", "python", "nodejs", "docker",
			},
		},

		// 3. Trainer (€19/mo)
		{
			Name:        "Trainer",
			Description: "For professional trainers. Run training sessions with up to 3 concurrent terminals and 5GB storage.",
			PriceAmount: 1900, // €19.00
			Currency:    "eur",
			BillingInterval: "month",
			TrialDays:   0,
			Features: []string{
				"Unlimited restarts",
				"8 hour max session",
				"3 concurrent terminals",
				"XS, S, and M machines",
				"Network access included",
				"5GB data persistence",
				"All standard templates",
			},
			MaxConcurrentUsers: 3,
			MaxCourses:         0,
			MaxLabSessions:     -1,
			IsActive:           true,
			RequiredRole:       "trainer",
			StripeCreated:      false,

			// Terminal-specific limits
			MaxSessionDurationMinutes: 480, // 8 hours
			MaxConcurrentTerminals:    3,   // Key upgrade from Solo
			AllowedMachineSizes:       []string{"XS", "S", "M"}, // XS, S, M sizes
			NetworkAccessEnabled:      true,
			DataPersistenceEnabled:    true,
			DataPersistenceGB:         5,
			AllowedTemplates: []string{
				"ubuntu-basic", "ubuntu-dev", "alpine-basic",
				"debian-basic", "python", "nodejs", "docker",
			},
		},

		// 4. Organization (€49/mo)
		{
			Name:        "Organization",
			Description: "For training companies and organizations. Unlimited sessions, 10 concurrent terminals, all machine sizes, and custom Docker support.",
			PriceAmount: 4900, // €49.00
			Currency:    "eur",
			BillingInterval: "month",
			TrialDays:   0,
			Features: []string{
				"Unlimited restarts",
				"8 hour max session",
				"10 concurrent terminals",
				"All machine sizes (XS/S/M/L/XL)",
				"Network access included",
				"20GB data persistence",
				"All templates + custom Docker",
			},
			MaxConcurrentUsers: 10,
			MaxCourses:         -1,
			MaxLabSessions:     -1,
			IsActive:           true,
			RequiredRole:       "organization",
			StripeCreated:      false,

			// Terminal-specific limits
			MaxSessionDurationMinutes: 480, // 8 hours
			MaxConcurrentTerminals:    10,  // Key upgrade from Trainer
			AllowedMachineSizes:       []string{"all"}, // All sizes: XS, S, M, L, XL
			NetworkAccessEnabled:      true,
			DataPersistenceEnabled:    true,
			DataPersistenceGB:         20,
			AllowedTemplates:          []string{"all"}, // Special value meaning all templates + custom
		},
	}

	// Seed the plans
	fmt.Println("\n🌱 Seeding subscription plans...")
	for i, plan := range plans {
		// Check if plan already exists
		var existing paymentModels.SubscriptionPlan
		result := db.Where("name = ?", plan.Name).First(&existing)

		if result.Error == gorm.ErrRecordNotFound {
			// Create new plan
			if err := db.Create(&plan).Error; err != nil {
				log.Printf("❌ Failed to create plan '%s': %v", plan.Name, err)
				continue
			}
			fmt.Printf("✅ Created plan %d: %s (€%.2f/month)\n", i+1, plan.Name, float64(plan.PriceAmount)/100)
		} else if result.Error != nil {
			log.Printf("❌ Error checking plan '%s': %v", plan.Name, result.Error)
		} else {
			// Update existing plan
			plan.ID = existing.ID // Preserve the ID
			if err := db.Model(&existing).Updates(&plan).Error; err != nil {
				log.Printf("❌ Failed to update plan '%s': %v", plan.Name, err)
				continue
			}
			fmt.Printf("♻️  Updated existing plan: %s (€%.2f/month)\n", plan.Name, float64(plan.PriceAmount)/100)
		}
	}

	fmt.Println("\n✨ Seeding completed!")
	fmt.Println("\n📋 Plan Summary:")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("%-15s | %-10s | %-12s | %-10s | %-10s\n", "Plan", "Price", "Concurrent", "Duration", "Network")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	var allPlans []paymentModels.SubscriptionPlan
	db.Find(&allPlans)
	for _, p := range allPlans {
		network := "❌"
		if p.NetworkAccessEnabled {
			network = "✅"
		}
		fmt.Printf("%-15s | €%-9.2f | %-12d | %-10dh | %-10s\n",
			p.Name,
			float64(p.PriceAmount)/100,
			p.MaxConcurrentTerminals,
			p.MaxSessionDurationMinutes/60,
			network,
		)
	}
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	fmt.Println("\n⚠️  Next steps:")
	fmt.Println("1. Create these plans in Stripe (manually or via API)")
	fmt.Println("2. Update StripeProductID and StripePriceID in database")
	fmt.Println("3. Test terminal creation with each plan")
	fmt.Println("4. Update middleware to enforce limits")
}
