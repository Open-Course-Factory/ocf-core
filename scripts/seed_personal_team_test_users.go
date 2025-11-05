package main

import (
	"fmt"
	"log"
	"time"

	"soli/formations/src/auth/casdoor"
	sqldb "soli/formations/src/db"
	groupModels "soli/formations/src/groups/models"
	orgModels "soli/formations/src/organizations/models"
	orgServices "soli/formations/src/organizations/services"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
)

// SeedPersonalTeamTestUsers creates demo users for testing personal-to-team organization feature
func main() {
	fmt.Println("================================================")
	fmt.Println("Seeding Personal-to-Team Test Users")
	fmt.Println("================================================")
	fmt.Println()

	// Load environment and connect to database
	envFile := ".env"
	sqldb.InitDBConnection(envFile)
	defer func() {
		db, _ := sqldb.DB.DB()
		db.Close()
	}()

	// Initialize Casdoor
	casdoor.InitCasdoorConnection("", envFile)

	stats := seedStats{
		usersCreated:   0,
		orgsCreated:    0,
		groupsCreated:  0,
		membersAdded:   0,
		errors:         0,
	}

	// Create all test users
	createTestUsers(&stats)

	printSeedSummary(stats)
}

type seedStats struct {
	usersCreated  int
	orgsCreated   int
	groupsCreated int
	membersAdded  int
	errors        int
}

type testUser struct {
	Email       string
	FirstName   string
	LastName    string
	Password    string
	Description string
}

// createPersonalOrgDB creates a personal organization directly in the database
// without calling service methods (to avoid permission issues in standalone scripts)
func createPersonalOrgDB(userID string) (*orgModels.Organization, error) {
	personalOrg := &orgModels.Organization{
		Name:             fmt.Sprintf("personal_%s", userID),
		DisplayName:      "Personal Organization",
		Description:      "Your personal workspace",
		OwnerUserID:      userID,
		OrganizationType: orgModels.OrgTypePersonal,
		MaxGroups:        -1, // Unlimited
		MaxMembers:       1,  // Only owner
		IsActive:         true,
	}

	err := sqldb.DB.Create(personalOrg).Error
	if err != nil {
		return nil, err
	}

	// Add owner as member
	ownerMember := &orgModels.OrganizationMember{
		OrganizationID: personalOrg.ID,
		UserID:         userID,
		Role:           orgModels.OrgRoleOwner,
		InvitedBy:      userID,
		JoinedAt:       time.Now(),
		IsActive:       true,
	}

	err = sqldb.DB.Create(ownerMember).Error
	if err != nil {
		// Clean up the org
		sqldb.DB.Delete(personalOrg)
		return nil, err
	}

	return personalOrg, nil
}

func createTestUsers(stats *seedStats) {
	users := []testUser{
		{
			Email:       "personal@test.com",
			FirstName:   "Alice",
			LastName:    "Personal",
			Password:    "Test123!",
			Description: "Personal Organization User",
		},
		{
			Email:       "converted@test.com",
			FirstName:   "Bob",
			LastName:    "Converted",
			Password:    "Test123!",
			Description: "Converted Team (Single Member)",
		},
		{
			Email:       "teamowner@test.com",
			FirstName:   "Charlie",
			LastName:    "Owner",
			Password:    "Test123!",
			Description: "Team Organization Owner",
		},
		{
			Email:       "teammember@test.com",
			FirstName:   "Diana",
			LastName:    "Member",
			Password:    "Test123!",
			Description: "Team Member",
		},
		{
			Email:       "multi@test.com",
			FirstName:   "Eve",
			LastName:    "Multi",
			Password:    "Test123!",
			Description: "Multi-Organization User",
		},
		{
			Email:       "invited@test.com",
			FirstName:   "Frank",
			LastName:    "Invited",
			Password:    "Test123!",
			Description: "Personal Org User Invited to Group",
		},
	}

	fmt.Println("Creating test users...")
	fmt.Println()

	for _, userData := range users {
		if err := createUserScenario(userData, stats); err != nil {
			log.Printf("❌ Failed to create user %s: %v", userData.Email, err)
			stats.errors++
		}
	}
}

func createUserScenario(userData testUser, stats *seedStats) error {
	// Check if user already exists
	existingUser, _ := casdoorsdk.GetUserByEmail(userData.Email)
	if existingUser != nil {
		fmt.Printf("⏭️  User %s already exists, skipping\n", userData.Email)
		return nil
	}

	// Create user in Casdoor
	user := &casdoorsdk.User{
		Owner:       "soli",
		Name:        fmt.Sprintf("%s_%s", userData.FirstName, userData.LastName),
		CreatedTime: time.Now().Format(time.RFC3339),
		DisplayName: fmt.Sprintf("%s %s", userData.FirstName, userData.LastName),
		FirstName:   userData.FirstName,
		LastName:    userData.LastName,
		Email:       userData.Email,
		Password:    userData.Password,
		Type:        "normal-user",
	}

	affected, err := casdoorsdk.AddUser(user)
	if err != nil || !affected {
		return fmt.Errorf("failed to create user in Casdoor: %w", err)
	}

	// Get created user with ID
	createdUser, err := casdoorsdk.GetUserByEmail(userData.Email)
	if err != nil {
		return fmt.Errorf("failed to get created user: %w", err)
	}

	stats.usersCreated++
	fmt.Printf("✅ Created user: %s (%s)\n", userData.Email, userData.Description)

	// Note: Permissions will be granted on first login, skipping here to avoid enforcer initialization issues

	// Handle specific scenarios
	orgService := orgServices.NewOrganizationService(sqldb.DB)

	switch userData.Email {
	case "personal@test.com":
		// Scenario 1: Personal Organization User (auto-created)
		personalOrg, err := createPersonalOrgDB(createdUser.Id)
		if err != nil {
			return fmt.Errorf("failed to create personal org: %w", err)
		}
		stats.orgsCreated++
		fmt.Printf("   → Personal org: %s\n", personalOrg.Name)

	case "converted@test.com":
		// Scenario 2: Converted Team (create personal, then convert)
		personalOrg, err := createPersonalOrgDB(createdUser.Id)
		if err != nil {
			return fmt.Errorf("failed to create personal org: %w", err)
		}
		stats.orgsCreated++

		// Convert to team
		convertedOrg, err := orgService.ConvertToTeam(personalOrg.ID, createdUser.Id, "Bob's Team")
		if err != nil {
			return fmt.Errorf("failed to convert to team: %w", err)
		}
		fmt.Printf("   → Converted org: %s (type: %s)\n", convertedOrg.Name, convertedOrg.OrganizationType)

	case "teamowner@test.com":
		// Scenario 3: Team Organization Owner with multiple members
		// First create personal org
		_, err := createPersonalOrgDB(createdUser.Id)
		if err != nil {
			return fmt.Errorf("failed to create personal org: %w", err)
		}
		stats.orgsCreated++

		// Create team organization
		teamOrg := &orgModels.Organization{
			Name:             "charlie_company",
			DisplayName:      "Charlie's Company",
			Description:      "A team organization with multiple members",
			OwnerUserID:      createdUser.Id,
			OrganizationType: orgModels.OrgTypeTeam,
			MaxGroups:        30,
			MaxMembers:       100,
			IsActive:         true,
		}

		err = sqldb.DB.Create(teamOrg).Error
		if err != nil {
			return fmt.Errorf("failed to create team org: %w", err)
		}
		stats.orgsCreated++

		// Add owner as member
		ownerMember := &orgModels.OrganizationMember{
			OrganizationID: teamOrg.ID,
			UserID:         createdUser.Id,
			Role:           orgModels.OrgRoleOwner,
			InvitedBy:      createdUser.Id,
			JoinedAt:       time.Now(),
			IsActive:       true,
		}
		sqldb.DB.Create(ownerMember)
		stats.membersAdded++

		// Note: Permissions will be granted on first login
		fmt.Printf("   → Team org: %s (ID: %s)\n", teamOrg.DisplayName, teamOrg.ID)

		// Create a group in the organization
		group := &groupModels.ClassGroup{
			Name:           "engineering_team",
			DisplayName:    "Engineering Team",
			Description:    "Engineering department group",
			OwnerUserID:    createdUser.Id,
			OrganizationID: &teamOrg.ID,
			MaxMembers:     50,
			IsActive:       true,
		}
		sqldb.DB.Create(group)
		stats.groupsCreated++
		fmt.Printf("   → Group: %s\n", group.DisplayName)

	case "teammember@test.com":
		// Scenario 4: Team Member (will be added to Charlie's Company)
		// Create personal org first
		_, err := createPersonalOrgDB(createdUser.Id)
		if err != nil {
			return fmt.Errorf("failed to create personal org: %w", err)
		}
		stats.orgsCreated++

		// Find Charlie's Company
		var charlieOrg orgModels.Organization
		err = sqldb.DB.Where("name = ?", "charlie_company").First(&charlieOrg).Error
		if err == nil {
			// Add as member
			member := &orgModels.OrganizationMember{
				OrganizationID: charlieOrg.ID,
				UserID:         createdUser.Id,
				Role:           orgModels.OrgRoleMember,
				InvitedBy:      charlieOrg.OwnerUserID,
				JoinedAt:       time.Now(),
				IsActive:       true,
			}
			sqldb.DB.Create(member)
			stats.membersAdded++

			// Note: Permissions will be granted on first login
			fmt.Printf("   → Added to: %s\n", charlieOrg.DisplayName)
		}

	case "multi@test.com":
		// Scenario 5: Multi-Organization User
		// Create personal org
		personalOrg, err := createPersonalOrgDB(createdUser.Id)
		if err != nil {
			return fmt.Errorf("failed to create personal org: %w", err)
		}
		stats.orgsCreated++
		fmt.Printf("   → Personal org: %s\n", personalOrg.Name)

		// Add to Charlie's Company as member
		var charlieOrg orgModels.Organization
		err = sqldb.DB.Where("name = ?", "charlie_company").First(&charlieOrg).Error
		if err == nil {
			member := &orgModels.OrganizationMember{
				OrganizationID: charlieOrg.ID,
				UserID:         createdUser.Id,
				Role:           orgModels.OrgRoleMember,
				InvitedBy:      charlieOrg.OwnerUserID,
				JoinedAt:       time.Now(),
				IsActive:       true,
			}
			sqldb.DB.Create(member)
			stats.membersAdded++

			// Note: Permissions will be granted on first login
			fmt.Printf("   → Added to: %s\n", charlieOrg.DisplayName)
		}

	case "invited@test.com":
		// Scenario 6: Personal Org User Invited to Group
		// Create personal org
		personalOrg, err := createPersonalOrgDB(createdUser.Id)
		if err != nil {
			return fmt.Errorf("failed to create personal org: %w", err)
		}
		stats.orgsCreated++
		fmt.Printf("   → Personal org: %s\n", personalOrg.Name)

		// Add to Engineering Team group
		var engineeringGroup groupModels.ClassGroup
		err = sqldb.DB.Where("name = ?", "engineering_team").First(&engineeringGroup).Error
		if err == nil {
			groupMember := &groupModels.GroupMember{
				GroupID:  engineeringGroup.ID,
				UserID:   createdUser.Id,
				Role:     groupModels.GroupMemberRoleMember,
				JoinedAt: time.Now(),
				IsActive: true,
			}
			sqldb.DB.Create(groupMember)
			fmt.Printf("   → Added to group: %s\n", engineeringGroup.DisplayName)
		}
	}

	fmt.Println()
	return nil
}

func printSeedSummary(stats seedStats) {
	fmt.Println("================================================")
	fmt.Println("Seed Summary")
	fmt.Println("================================================")
	fmt.Printf("Users created:              %d\n", stats.usersCreated)
	fmt.Printf("Organizations created:      %d\n", stats.orgsCreated)
	fmt.Printf("Groups created:             %d\n", stats.groupsCreated)
	fmt.Printf("Memberships added:          %d\n", stats.membersAdded)
	fmt.Printf("Errors:                     %d\n", stats.errors)
	fmt.Println("================================================")

	if stats.errors > 0 {
		fmt.Println("⚠️  Seeding completed with errors - please review logs above")
	} else {
		fmt.Println("✅ Test users created successfully!")
		fmt.Println()
		fmt.Println("Test Accounts (all use password: Test123!):")
		fmt.Println("  - personal@test.com    - Personal organization user")
		fmt.Println("  - converted@test.com   - Converted team (1 member)")
		fmt.Println("  - teamowner@test.com   - Team owner (2 members)")
		fmt.Println("  - teammember@test.com  - Team member")
		fmt.Println("  - multi@test.com       - Multi-organization user")
		fmt.Println("  - invited@test.com     - Personal org + group member")
	}
}
