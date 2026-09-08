// src/courses/hooks/initHooks.go
package courseHooks

import (
	"log"
	"soli/formations/src/entityManagement/hooks"

	"gorm.io/gorm"
)

// InitCourseHooks registers all course-related hooks
func InitCourseHooks(db *gorm.DB) {
	log.Println("🔗 Initializing course hooks...")

	// Hook for cascading deletion of orphaned chapters when a course is deleted
	courseCascadeHook := NewCourseCascadeDeleteHook(db)
	if err := hooks.GlobalHookRegistry.RegisterHook(courseCascadeHook); err != nil {
		log.Printf("❌ Failed to register course cascade delete hook: %v", err)
	} else {
		log.Println("✅ Course cascade delete hook registered")
	}

	// Hook for cascading deletion of orphaned sections when a chapter is deleted
	chapterCascadeHook := NewChapterCascadeDeleteHook(db)
	if err := hooks.GlobalHookRegistry.RegisterHook(chapterCascadeHook); err != nil {
		log.Printf("❌ Failed to register chapter cascade delete hook: %v", err)
	} else {
		log.Println("✅ Chapter cascade delete hook registered")
	}

	// Hook for cascading deletion of orphaned pages when a section is deleted
	sectionCascadeHook := NewSectionCascadeDeleteHook(db)
	if err := hooks.GlobalHookRegistry.RegisterHook(sectionCascadeHook); err != nil {
		log.Printf("❌ Failed to register section cascade delete hook: %v", err)
	} else {
		log.Println("✅ Section cascade delete hook registered")
	}

	log.Println("🔗 Course hooks initialization complete")
}

