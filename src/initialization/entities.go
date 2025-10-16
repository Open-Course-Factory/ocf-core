package initialization

import (
	"log"

	authRegistration "soli/formations/src/auth/entityRegistration"
	configModels "soli/formations/src/configuration/models"
	configRegistration "soli/formations/src/configuration/entityRegistration"
	configServices "soli/formations/src/configuration/services"
	"soli/formations/src/courses"
	courseRegistration "soli/formations/src/courses/entityRegistration"
	ems "soli/formations/src/entityManagement/entityManagementService"
	"soli/formations/src/groups"
	groupRegistration "soli/formations/src/groups/entityRegistration"
	"soli/formations/src/terminalTrainer"
	terminalRegistration "soli/formations/src/terminalTrainer/entityRegistration"

	"gorm.io/gorm"
)

// RegisterEntities registers all entities in the entity management system
func RegisterEntities() {
	ems.GlobalEntityRegistrationService.RegisterEntity(authRegistration.SshKeyRegistration{})
	ems.GlobalEntityRegistrationService.RegisterEntity(authRegistration.UserSettingsRegistration{})
	ems.GlobalEntityRegistrationService.RegisterEntity(courseRegistration.SessionRegistration{})
	ems.GlobalEntityRegistrationService.RegisterEntity(courseRegistration.CourseRegistration{})
	ems.GlobalEntityRegistrationService.RegisterEntity(courseRegistration.PageRegistration{})
	ems.GlobalEntityRegistrationService.RegisterEntity(courseRegistration.SectionRegistration{})
	ems.GlobalEntityRegistrationService.RegisterEntity(courseRegistration.ChapterRegistration{})
	ems.GlobalEntityRegistrationService.RegisterEntity(courseRegistration.ScheduleRegistration{})
	ems.GlobalEntityRegistrationService.RegisterEntity(courseRegistration.ThemeRegistration{})
	ems.GlobalEntityRegistrationService.RegisterEntity(courseRegistration.GenerationRegistration{})

	ems.GlobalEntityRegistrationService.RegisterEntity(terminalRegistration.TerminalRegistration{})
	ems.GlobalEntityRegistrationService.RegisterEntity(terminalRegistration.UserTerminalKeyRegistration{})
	ems.GlobalEntityRegistrationService.RegisterEntity(terminalRegistration.TerminalShareRegistration{})

	ems.GlobalEntityRegistrationService.RegisterEntity(groupRegistration.GroupRegistration{})
	ems.GlobalEntityRegistrationService.RegisterEntity(groupRegistration.GroupMemberRegistration{})

	ems.GlobalEntityRegistrationService.RegisterEntity(configRegistration.FeatureRegistration{})
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
