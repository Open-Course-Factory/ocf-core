package initialization

import (
	"log"

	authRegistration "soli/formations/src/auth/entityRegistration"
	configRegistration "soli/formations/src/configuration/entityRegistration"
	configModels "soli/formations/src/configuration/models"
	configServices "soli/formations/src/configuration/services"
	"soli/formations/src/courses"
	courseRegistration "soli/formations/src/courses/entityRegistration"
	emailRegistration "soli/formations/src/email/entityRegistration"
	ems "soli/formations/src/entityManagement/entityManagementService"
	"soli/formations/src/groups"
	groupRegistration "soli/formations/src/groups/entityRegistration"
	organizationRegistration "soli/formations/src/organizations/entityRegistration"
	"soli/formations/src/terminalTrainer"
	terminalRegistration "soli/formations/src/terminalTrainer/entityRegistration"

	"gorm.io/gorm"
)

// RegisterEntities registers all entities in the entity management system
func RegisterEntities() {
	authRegistration.RegisterSshKey(ems.GlobalEntityRegistrationService)
	ems.GlobalEntityRegistrationService.RegisterEntity(authRegistration.UserSettingsRegistration{})

	courseRegistration.RegisterSession(ems.GlobalEntityRegistrationService)
	courseRegistration.RegisterCourse(ems.GlobalEntityRegistrationService)
	courseRegistration.RegisterPage(ems.GlobalEntityRegistrationService)
	courseRegistration.RegisterSection(ems.GlobalEntityRegistrationService)
	courseRegistration.RegisterChapter(ems.GlobalEntityRegistrationService)
	courseRegistration.RegisterSchedule(ems.GlobalEntityRegistrationService)
	courseRegistration.RegisterTheme(ems.GlobalEntityRegistrationService)
	courseRegistration.RegisterGeneration(ems.GlobalEntityRegistrationService)

	terminalRegistration.RegisterTerminal(ems.GlobalEntityRegistrationService)
	terminalRegistration.RegisterUserTerminalKey(ems.GlobalEntityRegistrationService)
	terminalRegistration.RegisterTerminalShare(ems.GlobalEntityRegistrationService)

	groupRegistration.RegisterGroup(ems.GlobalEntityRegistrationService)
	groupRegistration.RegisterGroupMember(ems.GlobalEntityRegistrationService)

	organizationRegistration.RegisterOrganization(ems.GlobalEntityRegistrationService)
	organizationRegistration.RegisterOrganizationMember(ems.GlobalEntityRegistrationService)

	configRegistration.RegisterFeature(ems.GlobalEntityRegistrationService)

	emailRegistration.RegisterEmailTemplate(ems.GlobalEntityRegistrationService)
}

// RegisterModuleFeatures registers features from all modules
// Each module declares its own features via the ModuleConfig interface
func RegisterModuleFeatures(db *gorm.DB) {
	log.Println("🔧 Registering module features...")

	// Initialize feature registry
	configServices.InitFeatureRegistry(db)

	// Register each module's features
	modules := []interface {
		GetModuleName() string
		GetFeatures() []configModels.FeatureDefinition
	}{
		courses.NewCoursesModuleConfig(),
		terminalTrainer.NewTerminalTrainerModuleConfig(),
		groups.NewGroupsModuleConfig(),
	}

	for _, module := range modules {
		features := module.GetFeatures()
		configServices.GlobalFeatureRegistry.RegisterFeatures(features)
	}

	log.Printf("✅ Registered features from %d modules", len(modules))

	// Seed all registered features into database
	configServices.GlobalFeatureRegistry.SeedRegisteredFeatures()
}
