package initialization

import (
	"log"
	"os"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"gorm.io/gorm"

	auditModels "soli/formations/src/audit/models"
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
	db.AutoMigrate(&paymentModels.WebhookEvent{}) // ✅ SECURITY: Track processed webhooks in database

	// Configuration entities
	db.AutoMigrate(&configModels.Feature{})

	// Audit logging entities (compliance & security)
	db.AutoMigrate(&auditModels.AuditLog{})
}

// InitDevelopmentData sets up development data in debug mode
func InitDevelopmentData(db *gorm.DB) {
	env := os.Getenv("ENVIRONMENT")
	if env == "development" || env == "test" {
		db = db.Debug()
		setupExternalUsersData()
		SetupDefaultSubscriptionPlans(db)
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

// SetupDefaultSubscriptionPlans initializes default subscription plans
func SetupDefaultSubscriptionPlans(db *gorm.DB) {
	// Vérifier si les plans existent déjà
	var count int64
	db.Model(&paymentModels.SubscriptionPlan{}).Count(&count)
	if count > 0 {
		return // Plans déjà créés
	}

	// Plan Member Pro (Individual)
	memberProPlan := &paymentModels.SubscriptionPlan{
		Name:               "Member Pro",
		Description:        "Accès à un terminal - Utilisateur individuel",
		PriceAmount:        1200, // 12€ per license
		Currency:           "eur",
		BillingInterval:    "month",
		TrialDays:          14,
		Features:           []string{"unlimited_courses", "advanced_labs", "export", "custom_themes"},
		MaxConcurrentUsers: 1,
		MaxCourses:         -1,
		IsActive:           true,
		RequiredRole:       "member", // Changed from "member_pro" (deprecated) to "member"
		UseTieredPricing:   false,
	}

	// Plan Trainer (With Bulk Purchase & Tiered Pricing)
	trainerPlan := &paymentModels.SubscriptionPlan{
		Name:               "Trainer Plan",
		Description:        "Pour formateurs - Achat en gros avec tarifs dégressifs",
		PriceAmount:        1200, // 12€ base price per license
		Currency:           "eur",
		BillingInterval:    "month",
		TrialDays:          0,
		Features:           []string{"unlimited_courses", "advanced_labs", "export", "custom_themes", "bulk_purchase", "group_management"},
		MaxConcurrentUsers: 1,
		MaxCourses:         -1,
		IsActive:           true,
		RequiredRole:       "trainer",
		UseTieredPricing:   true,
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
