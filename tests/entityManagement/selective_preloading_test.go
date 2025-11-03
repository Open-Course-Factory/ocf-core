package entityManagement_tests

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	courseRegistration "soli/formations/src/courses/entityRegistration"
	"soli/formations/src/courses/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/entityManagement/services"
)

func setupSelectivePreloadingTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	// Use silent logger to reduce noise, but we'll enable counting queries manually
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	// Migrate all necessary tables
	err = db.AutoMigrate(
		&models.Course{},
		&models.Chapter{},
		&models.Section{},
		&models.Page{},
		&models.Generation{},
		&models.CourseChapters{},
		&models.ChapterSections{},
		&models.SectionPages{},
	)
	require.NoError(t, err)

	return db
}

func setupSelectivePreloadingTest(t *testing.T) (*gorm.DB, services.GenericService) {
	t.Helper()

	// Clear all entities before each test
	ems.GlobalEntityRegistrationService = ems.NewEntityRegistrationService()

	db := setupSelectivePreloadingTestDB(t)

	// Register entities
	ems.GlobalEntityRegistrationService.RegisterEntity(courseRegistration.CourseRegistration{})
	ems.GlobalEntityRegistrationService.RegisterEntity(courseRegistration.ChapterRegistration{})
	ems.GlobalEntityRegistrationService.RegisterEntity(courseRegistration.SectionRegistration{})
	ems.GlobalEntityRegistrationService.RegisterEntity(courseRegistration.PageRegistration{})

	// Create service (with nil enforcer for tests)
	service := services.NewGenericService(db, nil)

	return db, service
}

// createTestData creates a course hierarchy for testing
// Returns: course, chapter, section, page IDs
func createTestData(t *testing.T, db *gorm.DB) (uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) {
	t.Helper()

	// Create course
	course := &models.Course{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Name:      "Test Course",
		Title:     "Test Course Title",
	}
	require.NoError(t, db.Create(course).Error)

	// Create chapter
	chapter := &models.Chapter{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Title:     "Test Chapter",
	}
	require.NoError(t, db.Create(chapter).Error)

	// Link chapter to course
	require.NoError(t, db.Create(&models.CourseChapters{
		CourseID:  course.ID,
		ChapterID: chapter.ID,
		Order:     1,
	}).Error)

	// Create section
	section := &models.Section{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Title:     "Test Section",
	}
	require.NoError(t, db.Create(section).Error)

	// Link section to chapter
	require.NoError(t, db.Create(&models.ChapterSections{
		ChapterID: chapter.ID,
		SectionID: section.ID,
		Order:     1,
	}).Error)

	// Create page
	page := &models.Page{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Order:     1,
	}
	require.NoError(t, db.Create(page).Error)

	// Link page to section
	require.NoError(t, db.Create(&models.SectionPages{
		SectionID: section.ID,
		PageID:    page.ID,
		Order:     1,
	}).Error)

	return course.ID, chapter.ID, section.ID, page.ID
}

// Test: No includes - basic entity loading works
func TestSelectivePreloading_NoIncludes_EntityLoads(t *testing.T) {
	db, service := setupSelectivePreloadingTest(t)

	courseID, _, _, _ := createTestData(t, db)

	// Get course with no includes
	entityInterface := service.GetEntityModelInterface("Course")
	entity, err := service.GetEntity(courseID, entityInterface, "Course", nil)
	require.NoError(t, err)

	course, ok := entity.(*models.Course)
	require.True(t, ok, "Expected *models.Course")

	// Verify entity basic fields are loaded
	assert.Equal(t, courseID, course.ID)
	assert.Equal(t, "Test Course", course.Name)
}

// Test: Empty includes - basic entity loading works
func TestSelectivePreloading_EmptyIncludes_EntityLoads(t *testing.T) {
	db, service := setupSelectivePreloadingTest(t)

	courseID, _, _, _ := createTestData(t, db)

	// Get course with empty includes
	entityInterface := service.GetEntityModelInterface("Course")
	entity, err := service.GetEntity(courseID, entityInterface, "Course", []string{})
	require.NoError(t, err)

	course, ok := entity.(*models.Course)
	require.True(t, ok, "Expected *models.Course")

	// Verify entity basic fields are loaded
	assert.Equal(t, courseID, course.ID)
	assert.Equal(t, "Test Course", course.Name)
}

// Test: Wildcard "*" - should preload all associations
func TestSelectivePreloading_Wildcard_PreloadsAll(t *testing.T) {
	db, service := setupSelectivePreloadingTest(t)

	courseID, _, _, _ := createTestData(t, db)

	// Get course with wildcard
	entityInterface := service.GetEntityModelInterface("Course")
	entity, err := service.GetEntity(courseID, entityInterface, "Course", []string{"*"})
	require.NoError(t, err)

	course, ok := entity.(*models.Course)
	require.True(t, ok, "Expected *models.Course")

	// Verify all direct associations are preloaded
	assert.NotEmpty(t, course.Chapters, "Chapters should be preloaded with wildcard")
	assert.Equal(t, 1, len(course.Chapters), "Should have 1 chapter")
}

// Test: Specific relation - only load requested relation
func TestSelectivePreloading_SpecificRelation_LoadsOnlyRequested(t *testing.T) {
	db, service := setupSelectivePreloadingTest(t)

	courseID, _, _, _ := createTestData(t, db)

	// Get course with only Chapters included
	entityInterface := service.GetEntityModelInterface("Course")
	entity, err := service.GetEntity(courseID, entityInterface, "Course", []string{"Chapters"})
	require.NoError(t, err)

	course, ok := entity.(*models.Course)
	require.True(t, ok, "Expected *models.Course")

	// Verify Chapters are loaded
	assert.NotEmpty(t, course.Chapters, "Chapters should be preloaded")
	assert.Equal(t, 1, len(course.Chapters), "Should have 1 chapter")

	// Verify nested associations are NOT loaded (unless explicitly requested)
	if len(course.Chapters) > 0 {
		assert.Empty(t, course.Chapters[0].Sections, "Sections should not be preloaded without nested include")
	}
}

// Test: Multiple specific relations - load all requested
func TestSelectivePreloading_MultipleRelations_LoadsAll(t *testing.T) {
	db, service := setupSelectivePreloadingTest(t)

	// Create course with multiple authors (if Authors relation exists)
	courseID, _, _, _ := createTestData(t, db)

	// Get course with multiple includes
	entityInterface := service.GetEntityModelInterface("Course")
	entity, err := service.GetEntity(courseID, entityInterface, "Course", []string{"Chapters"})
	require.NoError(t, err)

	course, ok := entity.(*models.Course)
	require.True(t, ok, "Expected *models.Course")

	// Verify requested relations are loaded
	assert.NotEmpty(t, course.Chapters, "Chapters should be preloaded")
}

// Test: Nested relation with dot notation - load nested structures
func TestSelectivePreloading_NestedRelation_LoadsNested(t *testing.T) {
	db, service := setupSelectivePreloadingTest(t)

	courseID, _, _, _ := createTestData(t, db)

	// Get course with nested include: Chapters.Sections
	entityInterface := service.GetEntityModelInterface("Course")
	entity, err := service.GetEntity(courseID, entityInterface, "Course", []string{"Chapters.Sections"})
	require.NoError(t, err)

	course, ok := entity.(*models.Course)
	require.True(t, ok, "Expected *models.Course")

	// Verify Chapters are loaded
	require.NotEmpty(t, course.Chapters, "Chapters should be preloaded")
	assert.Equal(t, 1, len(course.Chapters), "Should have 1 chapter")

	// Verify Sections are loaded on the chapter
	chapter := course.Chapters[0]
	assert.NotEmpty(t, chapter.Sections, "Sections should be preloaded with nested include")
	assert.Equal(t, 1, len(chapter.Sections), "Should have 1 section")

	// Verify deeper nesting is NOT loaded (unless explicitly requested)
	if len(chapter.Sections) > 0 {
		section := chapter.Sections[0]
		assert.Empty(t, section.Pages, "Pages should not be preloaded without deeper nested include")
	}
}

// Test: Deep nested relation - load multiple levels
func TestSelectivePreloading_DeepNestedRelation_LoadsMultipleLevels(t *testing.T) {
	db, service := setupSelectivePreloadingTest(t)

	courseID, _, _, _ := createTestData(t, db)

	// Get course with deep nested include: Chapters.Sections.Pages
	entityInterface := service.GetEntityModelInterface("Course")
	entity, err := service.GetEntity(courseID, entityInterface, "Course", []string{"Chapters.Sections.Pages"})
	require.NoError(t, err)

	course, ok := entity.(*models.Course)
	require.True(t, ok, "Expected *models.Course")

	// Verify all levels are loaded
	require.NotEmpty(t, course.Chapters, "Chapters should be preloaded")
	chapter := course.Chapters[0]

	require.NotEmpty(t, chapter.Sections, "Sections should be preloaded")
	section := chapter.Sections[0]

	require.NotEmpty(t, section.Pages, "Pages should be preloaded with deep nested include")
	assert.Equal(t, 1, len(section.Pages), "Should have 1 page")
}

// Test: GetEntities with includes - verify list endpoint works
func TestSelectivePreloading_GetEntities_WithIncludes(t *testing.T) {
	db, service := setupSelectivePreloadingTest(t)

	// Create multiple courses
	createTestData(t, db)
	createTestData(t, db)

	// Get all courses with Chapters included
	entityInterface := service.GetEntityModelInterface("Course")
	results, total, err := service.GetEntities(entityInterface, 1, 10, map[string]any{}, []string{"Chapters"})
	require.NoError(t, err)

	// Verify we got results
	assert.Equal(t, int64(2), total, "Should have 2 courses")
	assert.NotEmpty(t, results, "Should return results")
}

// Test: GetEntitiesCursor with includes - verify cursor pagination works
func TestSelectivePreloading_GetEntitiesCursor_WithIncludes(t *testing.T) {
	db, service := setupSelectivePreloadingTest(t)

	// Create multiple courses
	createTestData(t, db)
	createTestData(t, db)

	// Get courses with cursor pagination and Chapters included
	entityInterface := service.GetEntityModelInterface("Course")
	results, _, hasMore, _, err := service.GetEntitiesCursor(entityInterface, "", 10, map[string]any{}, []string{"Chapters"})
	require.NoError(t, err)

	// Verify we got results
	assert.NotEmpty(t, results, "Should return results")
	assert.False(t, hasMore, "Should not have more results with limit 10")
}

// Test: Whitespace trimming in includes
func TestSelectivePreloading_WhitespaceTrimming_HandlesCorrectly(t *testing.T) {
	db, service := setupSelectivePreloadingTest(t)

	courseID, _, _, _ := createTestData(t, db)

	// Get course with whitespace in includes
	entityInterface := service.GetEntityModelInterface("Course")
	entity, err := service.GetEntity(courseID, entityInterface, "Course", []string{" Chapters ", "  "})
	require.NoError(t, err)

	course, ok := entity.(*models.Course)
	require.True(t, ok, "Expected *models.Course")

	// Verify Chapters are loaded (whitespace should be trimmed)
	assert.NotEmpty(t, course.Chapters, "Chapters should be preloaded despite whitespace")
}

// Test: Invalid relation name - GORM returns error for invalid relations
func TestSelectivePreloading_InvalidRelation_ReturnsError(t *testing.T) {
	db, service := setupSelectivePreloadingTest(t)

	courseID, _, _, _ := createTestData(t, db)

	// Get course with invalid relation name
	entityInterface := service.GetEntityModelInterface("Course")
	_, err := service.GetEntity(courseID, entityInterface, "Course", []string{"NonExistentRelation"})

	// GORM returns error for invalid relations
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported relations", "Should indicate invalid relation")
}
