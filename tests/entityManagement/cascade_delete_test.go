package entityManagement_tests

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	courseHooks "soli/formations/src/courses/hooks"
	"soli/formations/src/courses/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	"soli/formations/src/entityManagement/hooks"
	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/entityManagement/services"

	courseRegistration "soli/formations/src/courses/entityRegistration"
)

func setupCascadeDeleteTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Migrate all necessary tables
	err = db.AutoMigrate(
		&models.Course{},
		&models.Chapter{},
		&models.Section{},
		&models.Page{},
		&models.CourseChapters{},
		&models.ChapterSections{},
		&models.SectionPages{},
	)
	require.NoError(t, err)

	return db
}

func setupCascadeDeleteTestWithHooks(t *testing.T) (*gorm.DB, services.GenericService) {
	t.Helper()

	// Clear all hooks and entities before each test
	hooks.GlobalHookRegistry.ClearAllHooks()
	ems.GlobalEntityRegistrationService = ems.NewEntityRegistrationService()

	db := setupCascadeDeleteTestDB(t)

	// Register entities
	ems.GlobalEntityRegistrationService.RegisterEntity(courseRegistration.CourseRegistration{})
	ems.GlobalEntityRegistrationService.RegisterEntity(courseRegistration.ChapterRegistration{})
	ems.GlobalEntityRegistrationService.RegisterEntity(courseRegistration.SectionRegistration{})
	ems.GlobalEntityRegistrationService.RegisterEntity(courseRegistration.PageRegistration{})

	// Initialize cascade delete hooks
	courseHooks.InitCourseHooks(db)

	// Create service (with nil enforcer for tests)
	service := services.NewGenericService(db, nil)

	return db, service
}

// Test: Deleting a course should delete orphaned chapters
func TestCascadeDelete_Course_DeletesOrphanedChapters(t *testing.T) {
	db, service := setupCascadeDeleteTestWithHooks(t)

	// Create a course with 2 chapters
	course := &models.Course{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Name:      "Test Course",
		Title:     "Test Course Title",
	}
	require.NoError(t, db.Create(course).Error)

	chapter1 := &models.Chapter{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Title:     "Chapter 1",
	}
	chapter2 := &models.Chapter{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Title:     "Chapter 2",
	}
	require.NoError(t, db.Create(chapter1).Error)
	require.NoError(t, db.Create(chapter2).Error)

	// Link chapters to course
	require.NoError(t, db.Create(&models.CourseChapters{
		CourseID:  course.ID,
		ChapterID: chapter1.ID,
		Order:     1,
	}).Error)
	require.NoError(t, db.Create(&models.CourseChapters{
		CourseID:  course.ID,
		ChapterID: chapter2.ID,
		Order:     2,
	}).Error)

	// Delete the course
	err := service.DeleteEntity(course.ID, course, false)
	require.NoError(t, err)

	// Verify chapters are deleted (orphaned)
	var chapterCount int64
	db.Model(&models.Chapter{}).Count(&chapterCount)
	assert.Equal(t, int64(0), chapterCount, "Orphaned chapters should be deleted")
}

// Test: Deleting a course should NOT delete shared chapters
func TestCascadeDelete_Course_PreservesSharedChapters(t *testing.T) {
	db, service := setupCascadeDeleteTestWithHooks(t)

	// Create two courses
	course1 := &models.Course{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Name:      "Course 1",
		Title:     "Course 1 Title",
	}
	course2 := &models.Course{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Name:      "Course 2",
		Title:     "Course 2 Title",
	}
	require.NoError(t, db.Create(course1).Error)
	require.NoError(t, db.Create(course2).Error)

	// Create a shared chapter
	sharedChapter := &models.Chapter{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Title:     "Shared Chapter",
	}
	// Create a non-shared chapter
	nonSharedChapter := &models.Chapter{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Title:     "Non-Shared Chapter",
	}
	require.NoError(t, db.Create(sharedChapter).Error)
	require.NoError(t, db.Create(nonSharedChapter).Error)

	// Link shared chapter to both courses
	require.NoError(t, db.Create(&models.CourseChapters{
		CourseID:  course1.ID,
		ChapterID: sharedChapter.ID,
		Order:     1,
	}).Error)
	require.NoError(t, db.Create(&models.CourseChapters{
		CourseID:  course2.ID,
		ChapterID: sharedChapter.ID,
		Order:     1,
	}).Error)

	// Link non-shared chapter to course1 only
	require.NoError(t, db.Create(&models.CourseChapters{
		CourseID:  course1.ID,
		ChapterID: nonSharedChapter.ID,
		Order:     2,
	}).Error)

	// Delete course1
	err := service.DeleteEntity(course1.ID, course1, false)
	require.NoError(t, err)

	// Verify shared chapter still exists
	var sharedExists models.Chapter
	err = db.First(&sharedExists, sharedChapter.ID).Error
	assert.NoError(t, err, "Shared chapter should still exist")

	// Verify non-shared chapter is deleted
	var nonSharedExists models.Chapter
	err = db.First(&nonSharedExists, nonSharedChapter.ID).Error
	assert.Error(t, err, "Non-shared chapter should be deleted")
	assert.Equal(t, gorm.ErrRecordNotFound, err)
}

// Test: Deleting a chapter should delete orphaned sections
func TestCascadeDelete_Chapter_DeletesOrphanedSections(t *testing.T) {
	db, service := setupCascadeDeleteTestWithHooks(t)

	// Create a chapter with 2 sections
	chapter := &models.Chapter{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Title:     "Test Chapter",
	}
	require.NoError(t, db.Create(chapter).Error)

	section1 := &models.Section{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Title:     "Section 1",
	}
	section2 := &models.Section{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Title:     "Section 2",
	}
	require.NoError(t, db.Create(section1).Error)
	require.NoError(t, db.Create(section2).Error)

	// Link sections to chapter
	require.NoError(t, db.Create(&models.ChapterSections{
		ChapterID: chapter.ID,
		SectionID: section1.ID,
		Order:     1,
	}).Error)
	require.NoError(t, db.Create(&models.ChapterSections{
		ChapterID: chapter.ID,
		SectionID: section2.ID,
		Order:     2,
	}).Error)

	// Delete the chapter
	err := service.DeleteEntity(chapter.ID, chapter, false)
	require.NoError(t, err)

	// Verify sections are deleted (orphaned)
	var sectionCount int64
	db.Model(&models.Section{}).Count(&sectionCount)
	assert.Equal(t, int64(0), sectionCount, "Orphaned sections should be deleted")
}

// Test: Deleting a chapter should NOT delete shared sections
func TestCascadeDelete_Chapter_PreservesSharedSections(t *testing.T) {
	db, service := setupCascadeDeleteTestWithHooks(t)

	// Create two chapters
	chapter1 := &models.Chapter{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Title:     "Chapter 1",
	}
	chapter2 := &models.Chapter{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Title:     "Chapter 2",
	}
	require.NoError(t, db.Create(chapter1).Error)
	require.NoError(t, db.Create(chapter2).Error)

	// Create a shared section
	sharedSection := &models.Section{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Title:     "Shared Section",
	}
	require.NoError(t, db.Create(sharedSection).Error)

	// Link shared section to both chapters
	require.NoError(t, db.Create(&models.ChapterSections{
		ChapterID: chapter1.ID,
		SectionID: sharedSection.ID,
		Order:     1,
	}).Error)
	require.NoError(t, db.Create(&models.ChapterSections{
		ChapterID: chapter2.ID,
		SectionID: sharedSection.ID,
		Order:     1,
	}).Error)

	// Delete chapter1
	err := service.DeleteEntity(chapter1.ID, chapter1, false)
	require.NoError(t, err)

	// Verify shared section still exists
	var sharedExists models.Section
	err = db.First(&sharedExists, sharedSection.ID).Error
	assert.NoError(t, err, "Shared section should still exist")
}

// Test: Deleting a section should delete orphaned pages
func TestCascadeDelete_Section_DeletesOrphanedPages(t *testing.T) {
	db, service := setupCascadeDeleteTestWithHooks(t)

	// Create a section with 2 pages
	section := &models.Section{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Title:     "Test Section",
	}
	require.NoError(t, db.Create(section).Error)

	page1 := &models.Page{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Order:     1,
	}
	page2 := &models.Page{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Order:     2,
	}
	require.NoError(t, db.Create(page1).Error)
	require.NoError(t, db.Create(page2).Error)

	// Link pages to section
	require.NoError(t, db.Create(&models.SectionPages{
		SectionID: section.ID,
		PageID:    page1.ID,
		Order:     1,
	}).Error)
	require.NoError(t, db.Create(&models.SectionPages{
		SectionID: section.ID,
		PageID:    page2.ID,
		Order:     2,
	}).Error)

	// Delete the section
	err := service.DeleteEntity(section.ID, section, false)
	require.NoError(t, err)

	// Verify pages are deleted (orphaned)
	var pageCount int64
	db.Model(&models.Page{}).Count(&pageCount)
	assert.Equal(t, int64(0), pageCount, "Orphaned pages should be deleted")
}

// Test: Deleting a section should NOT delete shared pages
func TestCascadeDelete_Section_PreservesSharedPages(t *testing.T) {
	db, service := setupCascadeDeleteTestWithHooks(t)

	// Create two sections
	section1 := &models.Section{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Title:     "Section 1",
	}
	section2 := &models.Section{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Title:     "Section 2",
	}
	require.NoError(t, db.Create(section1).Error)
	require.NoError(t, db.Create(section2).Error)

	// Create a shared page
	sharedPage := &models.Page{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Order:     1,
	}
	require.NoError(t, db.Create(sharedPage).Error)

	// Link shared page to both sections
	require.NoError(t, db.Create(&models.SectionPages{
		SectionID: section1.ID,
		PageID:    sharedPage.ID,
		Order:     1,
	}).Error)
	require.NoError(t, db.Create(&models.SectionPages{
		SectionID: section2.ID,
		PageID:    sharedPage.ID,
		Order:     1,
	}).Error)

	// Delete section1
	err := service.DeleteEntity(section1.ID, section1, false)
	require.NoError(t, err)

	// Verify shared page still exists
	var sharedExists models.Page
	err = db.First(&sharedExists, sharedPage.ID).Error
	assert.NoError(t, err, "Shared page should still exist")
}

// Test: Full cascade - deleting a course should cascade through all levels
func TestCascadeDelete_Course_FullCascade(t *testing.T) {
	db, service := setupCascadeDeleteTestWithHooks(t)

	// Create a course with chapter -> section -> page
	course := &models.Course{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Name:      "Full Cascade Course",
		Title:     "Full Cascade Course Title",
	}
	require.NoError(t, db.Create(course).Error)

	chapter := &models.Chapter{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Title:     "Full Cascade Chapter",
	}
	require.NoError(t, db.Create(chapter).Error)

	section := &models.Section{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Title:     "Full Cascade Section",
	}
	require.NoError(t, db.Create(section).Error)

	page := &models.Page{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Order:     1,
	}
	require.NoError(t, db.Create(page).Error)

	// Link them all together
	require.NoError(t, db.Create(&models.CourseChapters{
		CourseID:  course.ID,
		ChapterID: chapter.ID,
		Order:     1,
	}).Error)
	require.NoError(t, db.Create(&models.ChapterSections{
		ChapterID: chapter.ID,
		SectionID: section.ID,
		Order:     1,
	}).Error)
	require.NoError(t, db.Create(&models.SectionPages{
		SectionID: section.ID,
		PageID:    page.ID,
		Order:     1,
	}).Error)

	// Delete the course
	err := service.DeleteEntity(course.ID, course, false)
	require.NoError(t, err)

	// Verify everything is deleted
	var chapterCount, sectionCount, pageCount int64
	db.Model(&models.Chapter{}).Count(&chapterCount)
	db.Model(&models.Section{}).Count(&sectionCount)
	db.Model(&models.Page{}).Count(&pageCount)

	assert.Equal(t, int64(0), chapterCount, "Chapter should be deleted")
	assert.Equal(t, int64(0), sectionCount, "Section should be deleted")
	assert.Equal(t, int64(0), pageCount, "Page should be deleted")
}
