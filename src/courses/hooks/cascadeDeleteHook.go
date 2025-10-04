// src/courses/hooks/cascadeDeleteHook.go
package courseHooks

import (
	"fmt"
	"log"
	"soli/formations/src/courses/models"
	"soli/formations/src/entityManagement/hooks"
	"soli/formations/src/entityManagement/services"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// CourseCascadeDeleteHook handles cascade deletion of orphaned chapters when a course is deleted
type CourseCascadeDeleteHook struct {
	db       *gorm.DB
	enabled  bool
	priority int
}

func NewCourseCascadeDeleteHook(db *gorm.DB) hooks.Hook {
	return &CourseCascadeDeleteHook{
		db:       db,
		enabled:  true,
		priority: 10,
	}
}

func (h *CourseCascadeDeleteHook) GetName() string {
	return "course_cascade_delete"
}

func (h *CourseCascadeDeleteHook) GetEntityName() string {
	return "Course"
}

func (h *CourseCascadeDeleteHook) GetHookTypes() []hooks.HookType {
	return []hooks.HookType{hooks.BeforeDelete}
}

func (h *CourseCascadeDeleteHook) IsEnabled() bool {
	return h.enabled
}

func (h *CourseCascadeDeleteHook) GetPriority() int {
	return h.priority
}

func (h *CourseCascadeDeleteHook) Execute(ctx *hooks.HookContext) error {
	course, ok := ctx.NewEntity.(*models.Course)
	if !ok {
		return fmt.Errorf("expected Course, got %T", ctx.NewEntity)
	}

	log.Printf("🗑️ Checking for orphaned chapters after deleting course: %s (ID: %s)", course.Name, course.ID)

	// Get all chapters that were associated with this course
	var chapterIDs []uuid.UUID
	err := h.db.Table("course_chapters").
		Where("course_id = ?", course.ID).
		Pluck("chapter_id", &chapterIDs).Error

	if err != nil {
		log.Printf("❌ Failed to get chapters for deleted course %s: %v", course.ID, err)
		return nil // Don't fail the deletion
	}

	if len(chapterIDs) == 0 {
		log.Printf("✅ No chapters to check for course %s", course.ID)
		return nil
	}

	// For each chapter, check if it belongs to any other course
	deletedCount := 0
	for _, chapterID := range chapterIDs {
		var count int64
		err := h.db.Table("course_chapters").
			Where("chapter_id = ? AND course_id != ?", chapterID, course.ID).
			Count(&count).Error

		if err != nil {
			log.Printf("❌ Failed to check chapter relationships for %s: %v", chapterID, err)
			continue
		}

		// If the chapter doesn't belong to any other course, delete it
		if count == 0 {
			chapter := &models.Chapter{}
			chapter.ID = chapterID

			// Clean up parent relationship only (where this chapter is referenced)
			// Child relationships (chapter_sections) will be handled by the chapter's own hook
			if err := h.db.Exec("DELETE FROM course_chapters WHERE chapter_id = ?", chapterID).Error; err != nil {
				log.Printf("❌ Failed to clean up course_chapters for chapter %s: %v", chapterID, err)
				continue
			}

			// Use service to delete so hooks cascade
			service := services.NewGenericService(h.db, nil)
			if err := service.DeleteEntity(chapterID, chapter, false); err != nil {
				log.Printf("❌ Failed to delete orphaned chapter %s: %v", chapterID, err)
			} else {
				deletedCount++
				log.Printf("✅ Deleted orphaned chapter %s", chapterID)
			}
		}
	}

	log.Printf("✅ Course cascade delete complete: deleted %d orphaned chapters", deletedCount)
	return nil
}

// ChapterCascadeDeleteHook handles cascade deletion of orphaned sections when a chapter is deleted
type ChapterCascadeDeleteHook struct {
	db       *gorm.DB
	enabled  bool
	priority int
}

func NewChapterCascadeDeleteHook(db *gorm.DB) hooks.Hook {
	return &ChapterCascadeDeleteHook{
		db:       db,
		enabled:  true,
		priority: 10,
	}
}

func (h *ChapterCascadeDeleteHook) GetName() string {
	return "chapter_cascade_delete"
}

func (h *ChapterCascadeDeleteHook) GetEntityName() string {
	return "Chapter"
}

func (h *ChapterCascadeDeleteHook) GetHookTypes() []hooks.HookType {
	return []hooks.HookType{hooks.BeforeDelete}
}

func (h *ChapterCascadeDeleteHook) IsEnabled() bool {
	return h.enabled
}

func (h *ChapterCascadeDeleteHook) GetPriority() int {
	return h.priority
}

func (h *ChapterCascadeDeleteHook) Execute(ctx *hooks.HookContext) error {
	chapter, ok := ctx.NewEntity.(*models.Chapter)
	if !ok {
		return fmt.Errorf("expected Chapter, got %T", ctx.NewEntity)
	}

	log.Printf("🗑️ Checking for orphaned sections after deleting chapter: %s (ID: %s)", chapter.Title, chapter.ID)

	// Get all sections that were associated with this chapter
	var sectionIDs []uuid.UUID
	err := h.db.Table("chapter_sections").
		Where("chapter_id = ?", chapter.ID).
		Pluck("section_id", &sectionIDs).Error

	if err != nil {
		log.Printf("❌ Failed to get sections for deleted chapter %s: %v", chapter.ID, err)
		return nil
	}

	if len(sectionIDs) == 0 {
		log.Printf("✅ No sections to check for chapter %s", chapter.ID)
		return nil
	}

	// For each section, check if it belongs to any other chapter
	deletedCount := 0
	for _, sectionID := range sectionIDs {
		var count int64
		err := h.db.Table("chapter_sections").
			Where("section_id = ? AND chapter_id != ?", sectionID, chapter.ID).
			Count(&count).Error

		if err != nil {
			log.Printf("❌ Failed to check section relationships for %s: %v", sectionID, err)
			continue
		}

		// If the section doesn't belong to any other chapter, delete it
		if count == 0 {
			section := &models.Section{}
			section.ID = sectionID

			// Clean up parent relationship only (where this section is referenced)
			// Child relationships (section_pages) will be handled by the section's own hook
			if err := h.db.Exec("DELETE FROM chapter_sections WHERE section_id = ?", sectionID).Error; err != nil {
				log.Printf("❌ Failed to clean up chapter_sections for section %s: %v", sectionID, err)
				continue
			}

			// Use service to delete so hooks cascade
			service := services.NewGenericService(h.db, nil)
			if err := service.DeleteEntity(sectionID, section, false); err != nil {
				log.Printf("❌ Failed to delete orphaned section %s: %v", sectionID, err)
			} else {
				deletedCount++
				log.Printf("✅ Deleted orphaned section %s", sectionID)
			}
		}
	}

	log.Printf("✅ Chapter cascade delete complete: deleted %d orphaned sections", deletedCount)
	return nil
}

// SectionCascadeDeleteHook handles cascade deletion of orphaned pages when a section is deleted
type SectionCascadeDeleteHook struct {
	db       *gorm.DB
	enabled  bool
	priority int
}

func NewSectionCascadeDeleteHook(db *gorm.DB) hooks.Hook {
	return &SectionCascadeDeleteHook{
		db:       db,
		enabled:  true,
		priority: 10,
	}
}

func (h *SectionCascadeDeleteHook) GetName() string {
	return "section_cascade_delete"
}

func (h *SectionCascadeDeleteHook) GetEntityName() string {
	return "Section"
}

func (h *SectionCascadeDeleteHook) GetHookTypes() []hooks.HookType {
	return []hooks.HookType{hooks.BeforeDelete}
}

func (h *SectionCascadeDeleteHook) IsEnabled() bool {
	return h.enabled
}

func (h *SectionCascadeDeleteHook) GetPriority() int {
	return h.priority
}

func (h *SectionCascadeDeleteHook) Execute(ctx *hooks.HookContext) error {
	section, ok := ctx.NewEntity.(*models.Section)
	if !ok {
		return fmt.Errorf("expected Section, got %T", ctx.NewEntity)
	}

	log.Printf("🗑️ Checking for orphaned pages after deleting section: %s (ID: %s)", section.Title, section.ID)

	// Get all pages that were associated with this section
	var pageIDs []uuid.UUID
	err := h.db.Table("section_pages").
		Where("section_id = ?", section.ID).
		Pluck("page_id", &pageIDs).Error

	if err != nil {
		log.Printf("❌ Failed to get pages for deleted section %s: %v", section.ID, err)
		return nil
	}

	if len(pageIDs) == 0 {
		log.Printf("✅ No pages to check for section %s", section.ID)
		return nil
	}

	// For each page, check if it belongs to any other section
	deletedCount := 0
	for _, pageID := range pageIDs {
		var count int64
		err := h.db.Table("section_pages").
			Where("page_id = ? AND section_id != ?", pageID, section.ID).
			Count(&count).Error

		if err != nil {
			log.Printf("❌ Failed to check page relationships for %s: %v", pageID, err)
			continue
		}

		// If the page doesn't belong to any other section, delete it
		if count == 0 {
			page := &models.Page{}
			page.ID = pageID

			// Clean up join table entries first
			if err := h.db.Exec("DELETE FROM section_pages WHERE page_id = ?", pageID).Error; err != nil {
				log.Printf("❌ Failed to clean up section_pages for page %s: %v", pageID, err)
				continue
			}

			// Use service to delete so hooks cascade
			service := services.NewGenericService(h.db, nil)
			if err := service.DeleteEntity(pageID, page, false); err != nil {
				log.Printf("❌ Failed to delete orphaned page %s: %v", pageID, err)
			} else {
				deletedCount++
				log.Printf("✅ Deleted orphaned page %s", pageID)
			}
		}
	}

	log.Printf("✅ Section cascade delete complete: deleted %d orphaned pages", deletedCount)
	return nil
}
