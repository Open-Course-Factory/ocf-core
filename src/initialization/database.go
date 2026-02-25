package initialization

import (
	"log"
	"os"
	"time"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"gorm.io/gorm"

	auditModels "soli/formations/src/audit/models"
	"soli/formations/src/auth/casdoor"
	authModels "soli/formations/src/auth/models"
	configModels "soli/formations/src/configuration/models"
	courseModels "soli/formations/src/courses/models"
	emailModels "soli/formations/src/email/models"
	groupModels "soli/formations/src/groups/models"
	organizationModels "soli/formations/src/organizations/models"
	paymentModels "soli/formations/src/payment/models"
	terminalModels "soli/formations/src/terminalTrainer/models"
	testtools "soli/formations/tests/testTools"
)

// AutoMigrateAll performs database migrations for all entities
func AutoMigrateAll(db *gorm.DB) {
	// Course entities
	db.AutoMigrate(&courseModels.Page{})
	db.AutoMigrate(&courseModels.Section{})
	db.AutoMigrate(&courseModels.Chapter{})
	db.AutoMigrate(&courseModels.Course{})
	db.AutoMigrate(&courseModels.Session{})

	// Course many-to-many relationships
	db.AutoMigrate(&courseModels.CourseChapters{})
	errJTChapterC := db.SetupJoinTable(&courseModels.Course{}, "Chapters", &courseModels.CourseChapters{})
	if errJTChapterC != nil {
		log.Default().Println(errJTChapterC)
	}
	errJTCoursesC := db.SetupJoinTable(&courseModels.Chapter{}, "Courses", &courseModels.CourseChapters{})
	if errJTCoursesC != nil {
		log.Default().Println(errJTCoursesC)
	}

	db.AutoMigrate(&courseModels.ChapterSections{})
	errJTSectionC := db.SetupJoinTable(&courseModels.Chapter{}, "Sections", &courseModels.ChapterSections{})
	if errJTSectionC != nil {
		log.Default().Println(errJTSectionC)
	}
	errJTChaptersS := db.SetupJoinTable(&courseModels.Section{}, "Chapters", &courseModels.ChapterSections{})
	if errJTChaptersS != nil {
		log.Default().Println(errJTChaptersS)
	}

	db.AutoMigrate(&courseModels.SectionPages{})
	errJTPage := db.SetupJoinTable(&courseModels.Section{}, "Pages", &courseModels.SectionPages{})
	if errJTPage != nil {
		log.Default().Println(errJTPage)
	}
	errJTSectionP := db.SetupJoinTable(&courseModels.Page{}, "Sections", &courseModels.SectionPages{})
	if errJTSectionP != nil {
		log.Default().Println(errJTSectionP)
	}

	// Other course entities
	db.AutoMigrate(&courseModels.Schedule{})
	db.AutoMigrate(&courseModels.Theme{})
	db.AutoMigrate(&courseModels.Generation{})

	// Auth entities
	db.AutoMigrate(&authModels.SshKey{})
	db.AutoMigrate(&authModels.UserSettings{})
	db.AutoMigrate(&authModels.TokenBlacklist{})
	db.AutoMigrate(&authModels.PasswordResetToken{})
	db.AutoMigrate(&authModels.EmailVerificationToken{})

	// Email entities
	db.AutoMigrate(&emailModels.EmailTemplate{})

	// Terminal entities
	db.AutoMigrate(&terminalModels.Terminal{})
	db.AutoMigrate(&terminalModels.UserTerminalKey{})
	db.AutoMigrate(&terminalModels.TerminalShare{})

	// Group entities
	db.AutoMigrate(&groupModels.ClassGroup{})
	db.AutoMigrate(&groupModels.GroupMember{})

	// Organization entities (Phase 1)
	db.AutoMigrate(&organizationModels.Organization{})
	db.AutoMigrate(&organizationModels.OrganizationMember{})

	// Payment entities
	db.AutoMigrate(&paymentModels.SubscriptionPlan{})
	db.AutoMigrate(&paymentModels.SubscriptionBatch{})
	db.AutoMigrate(&paymentModels.UserSubscription{})         // DEPRECATED in Phase 2 (kept for backward compat)
	db.AutoMigrate(&paymentModels.OrganizationSubscription{}) // NEW: Phase 2 - Organization subscriptions
	db.AutoMigrate(&paymentModels.Invoice{})
	db.AutoMigrate(&paymentModels.PaymentMethod{})
	db.AutoMigrate(&paymentModels.UsageMetrics{})
	db.AutoMigrate(&paymentModels.BillingAddress{})
	db.AutoMigrate(&paymentModels.PlanFeature{})
	db.AutoMigrate(&paymentModels.WebhookEvent{}) // ✅ SECURITY: Track processed webhooks in database

	// Configuration entities
	db.AutoMigrate(&configModels.Feature{})

	// Audit logging entities (compliance & security)
	db.AutoMigrate(&auditModels.AuditLog{})

	// Seed default data (idempotent - safe for all environments)
	SeedPlanFeatures(db)

	// Ensure the free Trial plan always exists (regardless of environment)
	EnsureTrialPlanExists(db)
}

// InitDevelopmentData sets up development data in debug mode
func InitDevelopmentData(db *gorm.DB) {
	env := os.Getenv("ENVIRONMENT")
	if env == "development" || env == "test" {
		db = db.Debug()
		SetupDefaultSubscriptionPlans(db)
		SeedPlanFeatures(db)
		setupExternalUsersData()
		syncCasdoorRolesToCasbin()
		ensureUsersHaveTrialPlan(db)
	}
}

// setupExternalUsersData initializes test users if none exist
func setupExternalUsersData() {
	users, _ := casdoorsdk.GetUsers()
	var notDeletedUser []*casdoorsdk.User
	for _, user := range users {
		if !user.IsDeleted {
			notDeletedUser = append(notDeletedUser, user)
		}
	}
	if len(notDeletedUser) == 0 {
		testtools.SetupBasicRoles()
		testtools.SetupUsers()
		testtools.SetupGroups()
		testtools.SetupRoles()
	}
}

// syncCasdoorRolesToCasbin ensures all Casdoor role assignments are reflected
// as Casbin grouping policies. This fixes cases where the casbin_rule table
// was reset but users still exist in Casdoor, leaving them with no roles.
func syncCasdoorRolesToCasbin() {
	orgName := os.Getenv("CASDOOR_ORGANIZATION_NAME")

	roles, err := casdoorsdk.GetRoles()
	if err != nil {
		log.Printf("[ROLE-SYNC] Could not get Casdoor roles: %v", err)
		return
	}

	users, err := casdoorsdk.GetUsers()
	if err != nil {
		log.Printf("[ROLE-SYNC] Could not get Casdoor users: %v", err)
		return
	}

	if err := casdoor.Enforcer.LoadPolicy(); err != nil {
		log.Printf("[ROLE-SYNC] Could not load Casbin policy: %v", err)
		return
	}

	// Build mapping: "orgName/username" -> userID
	userIDMap := make(map[string]string)
	for _, user := range users {
		if user != nil && !user.IsDeleted {
			userIDMap[orgName+"/"+user.Name] = user.Id
		}
	}

	// Ensure every active user has at least the "member" role
	for _, user := range users {
		if user == nil || user.IsDeleted {
			continue
		}
		existingRoles, _ := casdoor.Enforcer.GetRolesForUser(user.Id)
		hasMember := false
		for _, r := range existingRoles {
			if r == "member" {
				hasMember = true
				break
			}
		}
		if !hasMember {
			if _, err := casdoor.Enforcer.AddGroupingPolicy(user.Id, "member"); err != nil {
				log.Printf("[ROLE-SYNC] Failed to add 'member' role to user %s: %v", user.Id, err)
			} else {
				log.Printf("[ROLE-SYNC] Added missing 'member' role to user %s (%s)", user.Name, user.Id)
			}
		}
	}

	// Sync each Casdoor role to Casbin grouping policies
	for _, role := range roles {
		if role == nil {
			continue
		}
		for _, userRef := range role.Users {
			userID, ok := userIDMap[userRef]
			if !ok {
				continue
			}
			existingRoles, _ := casdoor.Enforcer.GetRolesForUser(userID)
			hasRole := false
			for _, r := range existingRoles {
				if r == role.Name {
					hasRole = true
					break
				}
			}
			if !hasRole {
				if _, err := casdoor.Enforcer.AddGroupingPolicy(userID, role.Name); err != nil {
					log.Printf("[ROLE-SYNC] Failed to add '%s' role to user %s: %v", role.Name, userID, err)
				} else {
					log.Printf("[ROLE-SYNC] Added missing '%s' role to user %s", role.Name, userID)
				}
			}
		}
	}

	log.Println("[ROLE-SYNC] Casdoor-to-Casbin role sync complete")
}

// EnsureTrialPlanExists ensures the free Trial plan always exists in the database.
// Uses FirstOrCreate so it is idempotent and safe to call in any environment.
func EnsureTrialPlanExists(db *gorm.DB) {
	trialPlan := paymentModels.SubscriptionPlan{
		Name:                      "Trial",
		Description:               "Free plan for testing the platform. 1 hour sessions, no network access. Perfect for trying out terminals.",
		PriceAmount:               0,
		Currency:                  "eur",
		BillingInterval:           "month",
		TrialDays:                 0,
		Features:                  []string{"machine_size_xs"},
		MaxConcurrentUsers:        1,
		MaxCourses:                -1,
		IsActive:                  true,
		RequiredRole:              "member",
		UseTieredPricing:          false,
		MaxSessionDurationMinutes: 60,
		MaxConcurrentTerminals:    1,
		AllowedMachineSizes:       []string{"XS"},
		NetworkAccessEnabled:      false,
		DataPersistenceEnabled:    false,
		DataPersistenceGB:         0,
		AllowedTemplates:          []string{"ubuntu-basic", "alpine-basic"},
		AllowedBackends:           []string{},
		DefaultBackend:            "",
		CommandHistoryRetentionDays: 0,
	}

	result := db.Where("name = ? AND price_amount = 0", "Trial").FirstOrCreate(&trialPlan)
	if result.Error != nil {
		log.Printf("Warning: Failed to ensure Trial plan exists: %v\n", result.Error)
	} else if result.RowsAffected > 0 {
		log.Println("Created missing Trial plan")
	}
}

// SetupDefaultSubscriptionPlans initializes default subscription plans
func SetupDefaultSubscriptionPlans(db *gorm.DB) {
	// Always ensure Trial plan exists first
	EnsureTrialPlanExists(db)

	// Vérifier si les other plans existent déjà
	var count int64
	db.Model(&paymentModels.SubscriptionPlan{}).Where("price_amount > 0").Count(&count)
	if count > 0 {
		return // Paid plans déjà créés
	}

	// Plan Member Pro (Individual)
	memberProPlan := &paymentModels.SubscriptionPlan{
		Name:               "Member Pro",
		Description:        "Accès à un terminal - Utilisateur individuel",
		PriceAmount:        1200, // 12€ per license
		Currency:           "eur",
		BillingInterval:    "month",
		TrialDays:          14,
		Features:           []string{"unlimited_courses", "advanced_labs", "export", "custom_themes", "machine_size_xs", "machine_size_s", "machine_size_m", "network_access", "data_persistence", "command_history"},
		MaxConcurrentUsers: 1,
		MaxCourses:         -1,
		IsActive:           true,
		RequiredRole:       "member", // Changed from "member_pro" (deprecated) to "member"
		UseTieredPricing:   false,
		MaxSessionDurationMinutes: 180,
		MaxConcurrentTerminals:    3,
		AllowedMachineSizes:       []string{"XS", "S", "M"},
		NetworkAccessEnabled:      true,
		DataPersistenceEnabled:    true,
		DataPersistenceGB:         5,
		AllowedBackends:             []string{}, // empty = all backends allowed
		DefaultBackend:              "",         // empty = TT default
		CommandHistoryRetentionDays: 90,
	}

	// Plan Trainer (With Bulk Purchase & Tiered Pricing)
	trainerPlan := &paymentModels.SubscriptionPlan{
		Name:               "Trainer Plan",
		Description:        "Pour formateurs - Achat en gros avec tarifs dégressifs",
		PriceAmount:        1200, // 12€ base price per license
		Currency:           "eur",
		BillingInterval:    "month",
		TrialDays:          0,
		Features:           []string{"unlimited_courses", "advanced_labs", "export", "custom_themes", "bulk_purchase", "group_management", "machine_size_xs", "machine_size_s", "machine_size_m", "machine_size_l", "machine_size_xl", "network_access", "data_persistence", "command_history"},
		MaxConcurrentUsers: 1,
		MaxCourses:         -1,
		IsActive:           true,
		RequiredRole:       "trainer",
		UseTieredPricing:   true,
		MaxSessionDurationMinutes: 480,
		MaxConcurrentTerminals:    10,
		AllowedMachineSizes:       []string{"XS", "S", "M", "L", "XL"},
		NetworkAccessEnabled:      true,
		DataPersistenceEnabled:    true,
		DataPersistenceGB:         20,
		AllowedBackends:             []string{}, // empty = all backends allowed
		DefaultBackend:              "",         // empty = TT default
		CommandHistoryRetentionDays: 365,
		PricingTiers: []paymentModels.PricingTier{
			{MinQuantity: 1, MaxQuantity: 5, UnitAmount: 1200, Description: "1-5 licences: 12€/licence"},
			{MinQuantity: 6, MaxQuantity: 15, UnitAmount: 1000, Description: "6-15 licences: 10€/licence"},
			{MinQuantity: 16, MaxQuantity: 30, UnitAmount: 800, Description: "16-30 licences: 8€/licence"},
			{MinQuantity: 31, MaxQuantity: 0, UnitAmount: 600, Description: "31+ licences: 6€/licence (illimité)"},
		},
	}

	plans := []*paymentModels.SubscriptionPlan{memberProPlan, trainerPlan}

	for _, plan := range plans {
		if err := db.Create(plan).Error; err != nil {
			log.Printf("Warning: Failed to create subscription plan %s: %v\n", plan.Name, err)
		} else {
			log.Printf("Created subscription plan: %s\n", plan.Name)
		}
	}
}

// ensureUsersHaveTrialPlan checks all Casdoor users and assigns the free Trial
// plan to any user who doesn't have an active subscription. This heals cases
// where the subscription assignment failed during user creation (e.g. due to
// initialization order issues or Casdoor resets).
func ensureUsersHaveTrialPlan(db *gorm.DB) {
	var trialPlan paymentModels.SubscriptionPlan
	result := db.Where("name = ? AND price_amount = 0 AND is_active = true", "Trial").First(&trialPlan)
	if result.Error != nil {
		log.Printf("[TRIAL-SYNC] Could not find active Trial plan: %v", result.Error)
		return
	}

	users, err := casdoorsdk.GetUsers()
	if err != nil {
		log.Printf("[TRIAL-SYNC] Could not get Casdoor users: %v", err)
		return
	}

	fixed := 0
	for _, user := range users {
		if user == nil || user.IsDeleted {
			continue
		}

		var existingSub paymentModels.UserSubscription
		subResult := db.Where("user_id = ? AND status = ?", user.Id, "active").First(&existingSub)
		if subResult.Error == nil {
			continue // User already has an active subscription
		}

		now := time.Now()
		newSub := paymentModels.UserSubscription{
			UserID:             user.Id,
			SubscriptionPlanID: trialPlan.ID,
			Status:             "active",
			CurrentPeriodStart: now,
			CurrentPeriodEnd:   now.AddDate(1, 0, 0),
			SubscriptionType:   "personal",
		}

		if err := db.Create(&newSub).Error; err != nil {
			log.Printf("[TRIAL-SYNC] Failed to assign Trial plan to user %s: %v", user.Id, err)
		} else {
			fixed++
		}
	}

	if fixed > 0 {
		log.Printf("[TRIAL-SYNC] Assigned Trial plan to %d users who were missing subscriptions", fixed)
	}
}

// SeedPlanFeatures populates the plan_features catalog table with default features.
// Only seeds if the table is empty.
func SeedPlanFeatures(db *gorm.DB) {
	var count int64
	db.Model(&paymentModels.PlanFeature{}).Count(&count)
	if count > 0 {
		return
	}

	features := []paymentModels.PlanFeature{
		// Capabilities (boolean)
		{Key: "unlimited_courses", DisplayNameEn: "Unlimited Courses", DisplayNameFr: "Cours illimités", Category: "capabilities", ValueType: "boolean", DefaultValue: "false", IsActive: true},
		{Key: "advanced_labs", DisplayNameEn: "Advanced Labs", DisplayNameFr: "Laboratoires avancés", Category: "capabilities", ValueType: "boolean", DefaultValue: "false", IsActive: true},
		{Key: "export", DisplayNameEn: "Course Export", DisplayNameFr: "Export de cours", Category: "capabilities", ValueType: "boolean", DefaultValue: "false", IsActive: true},
		{Key: "custom_themes", DisplayNameEn: "Custom Themes", DisplayNameFr: "Thèmes personnalisés", Category: "capabilities", ValueType: "boolean", DefaultValue: "false", IsActive: true},
		{Key: "bulk_purchase", DisplayNameEn: "Bulk License Purchase", DisplayNameFr: "Achat de licences en gros", Category: "capabilities", ValueType: "boolean", DefaultValue: "false", IsActive: true},
		{Key: "group_management", DisplayNameEn: "Group Management", DisplayNameFr: "Gestion des groupes", Category: "capabilities", ValueType: "boolean", DefaultValue: "false", IsActive: true},
		{Key: "api_access", DisplayNameEn: "API Access", DisplayNameFr: "Accès API", Category: "capabilities", ValueType: "boolean", DefaultValue: "false", IsActive: true},
		{Key: "analytics", DisplayNameEn: "Analytics Dashboard", DisplayNameFr: "Tableau de bord analytique", Category: "capabilities", ValueType: "boolean", DefaultValue: "false", IsActive: true},
		{Key: "priority_support", DisplayNameEn: "Priority Support", DisplayNameFr: "Support prioritaire", Category: "capabilities", ValueType: "boolean", DefaultValue: "false", IsActive: true},

		// Machine sizes (boolean)
		{Key: "machine_size_xs", DisplayNameEn: "XS Machine (0.5 CPU, 256MB)", DisplayNameFr: "Machine XS (0.5 CPU, 256Mo)", Category: "machine_sizes", ValueType: "boolean", DefaultValue: "false", IsActive: true},
		{Key: "machine_size_s", DisplayNameEn: "S Machine (1 CPU, 512MB)", DisplayNameFr: "Machine S (1 CPU, 512Mo)", Category: "machine_sizes", ValueType: "boolean", DefaultValue: "false", IsActive: true},
		{Key: "machine_size_m", DisplayNameEn: "M Machine (2 CPU, 1GB)", DisplayNameFr: "Machine M (2 CPU, 1Go)", Category: "machine_sizes", ValueType: "boolean", DefaultValue: "false", IsActive: true},
		{Key: "machine_size_l", DisplayNameEn: "L Machine (4 CPU, 4GB)", DisplayNameFr: "Machine L (4 CPU, 4Go)", Category: "machine_sizes", ValueType: "boolean", DefaultValue: "false", IsActive: true},
		{Key: "machine_size_xl", DisplayNameEn: "XL Machine (8 CPU, 8GB)", DisplayNameFr: "Machine XL (8 CPU, 8Go)", Category: "machine_sizes", ValueType: "boolean", DefaultValue: "false", IsActive: true},

		// Terminal limits (mixed types)
		{Key: "network_access", DisplayNameEn: "External Network Access", DisplayNameFr: "Accès réseau externe", Category: "terminal_limits", ValueType: "boolean", DefaultValue: "false", IsActive: true},
		{Key: "data_persistence", DisplayNameEn: "Persistent Storage", DisplayNameFr: "Stockage persistant", Category: "terminal_limits", ValueType: "boolean", DefaultValue: "false", IsActive: true},
		{Key: "data_persistence_gb", DisplayNameEn: "Storage Quota", DisplayNameFr: "Quota de stockage", Category: "terminal_limits", ValueType: "number", Unit: "GB", DefaultValue: "0", IsActive: true},
		{Key: "command_history", DisplayNameEn: "Command History Recording", DisplayNameFr: "Enregistrement historique", Category: "terminal_limits", ValueType: "boolean", DefaultValue: "false", IsActive: true},
		{Key: "command_history_retention_days", DisplayNameEn: "History Retention", DisplayNameFr: "Rétention de l'historique", Category: "terminal_limits", ValueType: "number", Unit: "days", DefaultValue: "0", IsActive: true},
		{Key: "max_session_duration_minutes", DisplayNameEn: "Max Session Duration", DisplayNameFr: "Durée max de session", Category: "terminal_limits", ValueType: "number", Unit: "minutes", DefaultValue: "60", IsActive: true},
		{Key: "max_concurrent_terminals", DisplayNameEn: "Max Concurrent Terminals", DisplayNameFr: "Terminaux simultanés max", Category: "terminal_limits", ValueType: "number", Unit: "count", DefaultValue: "1", IsActive: true},

		// Course limits (number)
		{Key: "max_courses", DisplayNameEn: "Max Courses (-1 = unlimited)", DisplayNameFr: "Cours max (-1 = illimité)", Category: "course_limits", ValueType: "number", Unit: "count", DefaultValue: "-1", IsActive: true},
		{Key: "max_concurrent_users", DisplayNameEn: "Max Concurrent Users", DisplayNameFr: "Utilisateurs simultanés max", Category: "course_limits", ValueType: "number", Unit: "count", DefaultValue: "1", IsActive: true},
	}

	for _, feature := range features {
		if err := db.Create(&feature).Error; err != nil {
			log.Printf("Warning: Failed to create plan feature %s: %v\n", feature.Key, err)
		}
	}

	log.Printf("Seeded %d plan features\n", len(features))
}
