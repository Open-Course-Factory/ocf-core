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

	// Define the MVP plans: Trial (XS - Free) + Solo (S - Paid)
	// Other plans marked as coming soon (IsActive = false)
	plans := []paymentModels.SubscriptionPlan{
		// 1. Trial (XS - Free) - ACTIVE AT LAUNCH
		{
			Name:            "Trial",
			Description:     "Free plan for testing the platform. 1 hour sessions, no network access. Perfect for trying out terminals.",
			PriceAmount:     0,
			Currency:        "eur",
			BillingInterval: "month",
			TrialDays:       0,
			Features: []string{
				"Unlimited restarts",
				"1 hour max session",
				"1 concurrent terminal",
				"XS machine (0.5 vCPU, 256MB RAM)",
				"No network access",
				"Ephemeral storage only",
			},
			MaxConcurrentUsers: 1,
			MaxCourses:         0,
			IsActive:           true, // ACTIVE
			RequiredRole:       "member",
			StripeCreated:      false,

			// Terminal-specific limits
			MaxSessionDurationMinutes: 60,             // 1 hour
			MaxConcurrentTerminals:    1,              // Only 1 at a time
			AllowedMachineSizes:       []string{"XS"}, // XS size: 0.5 vCPU, 256MB RAM
			NetworkAccessEnabled:      false,          // No network in free plan
			DataPersistenceEnabled:    false,          // No persistence
			DataPersistenceGB:         0,
			AllowedTemplates:          []string{"ubuntu-basic", "alpine-basic"},
			PlannedFeatures:           []string{}, // No planned features for free plan
		},

		// 2. Solo (S - €9/mo) - ACTIVE AT LAUNCH
		{
			Name:            "Solo",
			Description:     "Perfect for individual learning. Full 8-hour sessions with outbound network and ephemeral storage for personal projects.",
			PriceAmount:     900, // €9.00
			Currency:        "eur",
			BillingInterval: "month",
			TrialDays:       0,
			Features: []string{
				"Unlimited restarts",
				"8 hour max session",
				"1 concurrent terminal",
				"S machine (1 vCPU, 1GB RAM)",
				"Outbound network access",
				"Ephemeral storage",
				"All standard templates",
			},
			MaxConcurrentUsers: 1,
			MaxCourses:         0,
			IsActive:           true, // ACTIVE
			RequiredRole:       "member",
			StripeCreated:      false,

			// Terminal-specific limits
			MaxSessionDurationMinutes: 480, // 8 hours
			MaxConcurrentTerminals:    1,
			AllowedMachineSizes:       []string{"XS", "S"}, // S size: 1 vCPU, 1GB RAM
			NetworkAccessEnabled:      true,                // Outbound only
			DataPersistenceEnabled:    false,               // No persistence yet
			DataPersistenceGB:         0,
			AllowedTemplates: []string{
				"ubuntu-basic", "ubuntu-dev", "alpine-basic",
				"debian-basic", "python", "nodejs", "docker",
			},
			PlannedFeatures: []string{
				"🔜 200MB persistent storage",
			},
		},

		// 3. Trainer (M - €19/mo) - COMING SOON
		{
			Name:            "Trainer",
			Description:     "Coming soon: For professional trainers. Run training sessions with up to 3 concurrent terminals.",
			PriceAmount:     1900, // €19.00
			Currency:        "eur",
			BillingInterval: "month",
			TrialDays:       0,
			Features: []string{
				"Unlimited restarts",
				"8 hour max session",
				"3 concurrent terminals",
				"M machine (2 vCPU, 2GB RAM)",
				"Outbound network access",
				"Ephemeral storage",
				"All standard templates",
			},
			MaxConcurrentUsers: 3,
			MaxCourses:         0,
			IsActive:           false, // COMING SOON
			RequiredRole:       "trainer",
			StripeCreated:      false,

			// Terminal-specific limits
			MaxSessionDurationMinutes: 480, // 8 hours
			MaxConcurrentTerminals:    3,
			AllowedMachineSizes:       []string{"XS", "S", "M"}, // M size: 2 vCPU, 2GB RAM
			NetworkAccessEnabled:      true,
			DataPersistenceEnabled:    false,
			DataPersistenceGB:         0,
			AllowedTemplates: []string{
				"ubuntu-basic", "ubuntu-dev", "alpine-basic",
				"debian-basic", "python", "nodejs", "docker",
			},
			PlannedFeatures: []string{
				"🔜 1GB persistent storage",
				"🔜 Web development with port forwarding",
				"🔜 Custom images",
				"🔜 Team collaboration features",
			},
		},

		// 4. Organization (L - €49/mo) - COMING SOON
		{
			Name:            "Organization",
			Description:     "Coming soon: For training companies and organizations. Multiple concurrent terminals and larger machine sizes.",
			PriceAmount:     4900, // €49.00
			Currency:        "eur",
			BillingInterval: "month",
			TrialDays:       0,
			Features: []string{
				"Unlimited restarts",
				"8 hour max session",
				"10 concurrent terminals",
				"L machine (4 vCPU, 4GB RAM)",
				"Outbound network access",
				"Ephemeral storage",
				"All templates",
			},
			MaxConcurrentUsers: 10,
			MaxCourses:         -1,
			IsActive:           false, // COMING SOON
			RequiredRole:       "organization",
			StripeCreated:      false,

			// Terminal-specific limits
			MaxSessionDurationMinutes: 480, // 8 hours
			MaxConcurrentTerminals:    10,
			AllowedMachineSizes:       []string{"XS", "S", "M", "L"}, // L size: 4 vCPU, 4GB RAM
			NetworkAccessEnabled:      true,
			DataPersistenceEnabled:    false,
			DataPersistenceGB:         0,
			AllowedTemplates:          []string{"all"},
			PlannedFeatures: []string{
				"🔜 5GB persistent storage",
				"🔜 Web development with port forwarding",
				"🔜 Custom images",
				"🔜 Team collaboration features",
				"🔜 Priority support",
			},
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
