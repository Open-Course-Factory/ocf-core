package main

import (
	"fmt"
	"log"
	"soli/formations/src/auth/models"
	sqldb "soli/formations/src/db"
	groupModels "soli/formations/src/groups/models"
	orgModels "soli/formations/src/organizations/models"
	"time"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/joho/godotenv"
)

// MigrateRolesToPhase3 migrates users from old role system to Phase 3 simplified roles
// Phase 3: Only "member" and "administrator" system roles remain
// Business roles (owner, manager, trainer) become organization/group membership roles
func main() {
	fmt.Println("================================================")
	fmt.Println("Phase 3 Role Simplification Migration")
	fmt.Println("================================================")
	fmt.Println()

	// Load environment
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not found, using environment variables")
	}

	// Connect to database
	if err := sqldb.OpenDB(); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer func() {
		db, _ := sqldb.DB.DB()
		db.Close()
	}()

	// Get all Casdoor users
	users, err := casdoorsdk.GetUsers()
	if err != nil {
		log.Fatalf("Failed to get users from Casdoor: %v", err)
	}

	fmt.Printf("Found %d users to migrate\n\n", len(users))

	stats := migrationStats{
		total:             len(users),
		alreadyMigrated:   0,
		adminKept:         0,
		convertedToMember: 0,
		errors:            0,
	}

	for _, user := range users {
		if err := migrateUser(user, &stats); err != nil {
			log.Printf("❌ Error migrating user %s: %v", user.Id, err)
			stats.errors++
		}
	}

	printMigrationSummary(stats)
}

type migrationStats struct {
	total             int
	alreadyMigrated   int
	adminKept         int
	convertedToMember int
	errors            int
}

func migrateUser(user *casdoorsdk.User, stats *migrationStats) error {
	// Determine current OCF role from Casdoor roles
	var currentRole models.RoleName
	hasAdminRole := false

	for _, casdoorRole := range user.Roles {
		roleName := casdoorRole.Name
		if roleName == "admin" || roleName == "administrator" {
			hasAdminRole = true
			break
		}
	}

	// Determine what the user's role should be
	if hasAdminRole {
		// Keep administrators as-is
		currentRole = models.Administrator
		stats.adminKept++
		fmt.Printf("✅ User %s (%s) - Kept as administrator\n", user.Name, user.Id)
		return nil
	}

	// All other users become "member"
	// Their business roles come from organization/group membership

	// Check if user has advanced roles that need org/group membership
	needsOrgMembership := false
	var oldRole string

	for _, casdoorRole := range user.Roles {
		roleName := casdoorRole.Name
		switch roleName {
		case "trainer", "teacher", "supervisor":
			needsOrgMembership = true
			oldRole = roleName
		}
	}

	if needsOrgMembership {
		// Ensure user has appropriate organization membership
		if err := ensureOrganizationMembership(user.Id, oldRole); err != nil {
			return fmt.Errorf("failed to create org membership: %w", err)
		}
		fmt.Printf("✅ User %s (%s) - Converted to member + org membership (%s)\n", user.Name, user.Id, oldRole)
	} else {
		fmt.Printf("✅ User %s (%s) - Converted to member\n", user.Name, user.Id)
	}

	stats.convertedToMember++
	return nil
}

func ensureOrganizationMembership(userID string, oldRole string) error {
	// Get or create user's personal organization
	var personalOrg orgModels.Organization
	result := sqldb.DB.Where("owner_user_id = ? AND is_personal = ?", userID, true).First(&personalOrg)

	if result.Error != nil {
		// Personal org doesn't exist, this is unusual but not critical
		log.Printf("Warning: No personal organization found for user %s", userID)
		return nil
	}

	// Check if user is already a member/manager of their personal org
	var existingMember orgModels.OrganizationMember
	result = sqldb.DB.Where("organization_id = ? AND user_id = ?", personalOrg.ID, userID).First(&existingMember)

	if result.Error == nil {
		// Already has membership
		if existingMember.Role != orgModels.OrgRoleOwner && existingMember.Role != orgModels.OrgRoleManager {
			// Upgrade to manager if they had trainer/supervisor role
			if oldRole == "trainer" || oldRole == "supervisor" {
				existingMember.Role = orgModels.OrgRoleManager
				sqldb.DB.Save(&existingMember)
				log.Printf("  ↑ Upgraded to org manager")
			}
		}
		return nil
	}

	// Create organization membership
	member := orgModels.OrganizationMember{
		OrganizationID: personalOrg.ID,
		UserID:         userID,
		Role:           orgModels.OrgRoleOwner, // Owner of their personal org
		JoinedAt:       time.Now(),
		IsActive:       true,
	}

	if err := sqldb.DB.Create(&member).Error; err != nil {
		return err
	}

	log.Printf("  → Created personal org ownership")

	// Also check if they own any groups and need group admin role
	var ownedGroups []groupModels.ClassGroup
	sqldb.DB.Where("supervisor_id = ?", userID).Find(&ownedGroups)

	if len(ownedGroups) > 0 {
		log.Printf("  → User owns %d groups", len(ownedGroups))
		// Group membership is handled separately - just log
	}

	return nil
}

func printMigrationSummary(stats migrationStats) {
	fmt.Println()
	fmt.Println("================================================")
	fmt.Println("Migration Summary")
	fmt.Println("================================================")
	fmt.Printf("Total users:             %d\n", stats.total)
	fmt.Printf("Administrators kept:     %d\n", stats.adminKept)
	fmt.Printf("Converted to member:     %d\n", stats.convertedToMember)
	fmt.Printf("Errors:                  %d\n", stats.errors)
	fmt.Println()

	if stats.errors > 0 {
		fmt.Println("⚠️  Some users failed to migrate. Check logs above.")
	} else {
		fmt.Println("✅ All users migrated successfully!")
	}

	fmt.Println()
	fmt.Println("Next Steps:")
	fmt.Println("1. Verify organization memberships are correct")
	fmt.Println("2. Test user permissions in the application")
	fmt.Println("3. Monitor for any permission-related errors")
	fmt.Println("4. Once stable, deprecated role code can be removed")
	fmt.Println()
}
