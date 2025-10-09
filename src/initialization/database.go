package initialization

import (
	"log"
	"os"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"gorm.io/gorm"

	authModels "soli/formations/src/auth/models"
	configModels "soli/formations/src/configuration/models"
	courseModels "soli/formations/src/courses/models"
	paymentModels "soli/formations/src/payment/models"
	terminalModels "soli/formations/src/terminalTrainer/models"
	testtools "soli/formations/tests/testTools"
)

// AutoMigrateAll performs database migrations for all entities
func AutoMigrateAll(db *gorm.DB) {
	db.AutoMigrate()

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

	// Terminal entities
	db.AutoMigrate(&terminalModels.Terminal{})
	db.AutoMigrate(&terminalModels.UserTerminalKey{})
	db.AutoMigrate(&terminalModels.TerminalShare{})

	// Payment entities
	db.AutoMigrate(&paymentModels.SubscriptionPlan{})
	db.AutoMigrate(&paymentModels.UserSubscription{})
	db.AutoMigrate(&paymentModels.Invoice{})
	db.AutoMigrate(&paymentModels.PaymentMethod{})
	db.AutoMigrate(&paymentModels.UsageMetrics{})
	db.AutoMigrate(&paymentModels.BillingAddress{})

	// Configuration entities
	db.AutoMigrate(&configModels.Feature{})
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

	// Plan Member Pro
	memberProPlan := &paymentModels.SubscriptionPlan{
		Name:               "Member Pro",
		Description:        "Accès à un terminal",
		PriceAmount:        490, // 4.90€
		Currency:           "eur",
		BillingInterval:    "month",
		TrialDays:          14,
		Features:           []string{"unlimited_courses", "advanced_labs", "export", "custom_themes"},
		MaxConcurrentUsers: 1,
		MaxCourses:         0,
		MaxLabSessions:     1,
		IsActive:           true,
		RequiredRole:       "member_pro",
	}

	plans := []*paymentModels.SubscriptionPlan{memberProPlan}

	for _, plan := range plans {
		if err := db.Create(plan).Error; err != nil {
			log.Printf("Warning: Failed to create subscription plan %s: %v\n", plan.Name, err)
		} else {
			log.Printf("Created subscription plan: %s\n", plan.Name)
		}
	}
}
