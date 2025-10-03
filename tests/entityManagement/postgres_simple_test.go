// tests/entityManagement/postgres_simple_test.go
// PostgreSQL-specific tests for relationship filtering
// These tests require PostgreSQL and are skipped if not available

package entityManagement_tests

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	ems "soli/formations/src/entityManagement/entityManagementService"
	"soli/formations/src/entityManagement/hooks"
	entityManagementModels "soli/formations/src/entityManagement/models"
)

// Simple test entities for PostgreSQL
type PGTestCourse struct {
	entityManagementModels.BaseModel
	Name string `json:"name"`
}

type PGTestChapter struct {
	entityManagementModels.BaseModel
	Name      string `json:"name"`
	CourseID  uuid.UUID
	Course    PGTestCourse `gorm:"foreignKey:CourseID"`
}

type PGTestPage struct {
	entityManagementModels.BaseModel
	Name      string `json:"name"`
	Content   string `json:"content"`
	ChapterID uuid.UUID
	Chapter   PGTestChapter `gorm:"foreignKey:ChapterID"`
}

// Many-to-many test entities
type Student struct {
	entityManagementModels.BaseModel
	Name    string
	Courses []PGCourse `gorm:"many2many:student_courses;"`
}

type PGCourse struct {
	entityManagementModels.BaseModel
	Name     string
	Students []Student `gorm:"many2many:student_courses;"`
}

func TestPostgres_BasicCRUD(t *testing.T) {
	SkipIfNoPostgres(t)

	db := SetupPostgresTestDB(t)
	if db == nil {
		return
	}

	// Disable hooks
	hooks.GlobalHookRegistry.DisableAllHooks(true)
	defer hooks.GlobalHookRegistry.DisableAllHooks(false)

	// Reset global service
	ems.GlobalEntityRegistrationService = ems.NewEntityRegistrationService()

	// Migrate tables
	err := db.AutoMigrate(&PGTestCourse{}, &PGTestChapter{}, &PGTestPage{})
	require.NoError(t, err)

	// Create test data
	course := PGTestCourse{Name: "Test Course"}
	err = db.Create(&course).Error
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, course.ID)

	chapter := PGTestChapter{Name: "Chapter 1", CourseID: course.ID}
	err = db.Create(&chapter).Error
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, chapter.ID)

	page := PGTestPage{Name: "Page 1", Content: "Test content", ChapterID: chapter.ID}
	err = db.Create(&page).Error
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, page.ID)

	// Query with join
	var retrievedPage PGTestPage
	err = db.Preload("Chapter.Course").First(&retrievedPage, page.ID).Error
	require.NoError(t, err)

	assert.Equal(t, "Page 1", retrievedPage.Name)
	assert.Equal(t, "Chapter 1", retrievedPage.Chapter.Name)
	assert.Equal(t, "Test Course", retrievedPage.Chapter.Course.Name)

	// Cleanup
	db.Exec("DROP TABLE pg_test_pages CASCADE")
	db.Exec("DROP TABLE pg_test_chapters CASCADE")
	db.Exec("DROP TABLE pg_test_courses CASCADE")

	t.Log("✅ PostgreSQL basic CRUD test passed")
}

func TestPostgres_ForeignKeyRelationships(t *testing.T) {
	SkipIfNoPostgres(t)

	db := SetupPostgresTestDB(t)
	if db == nil {
		return
	}

	hooks.GlobalHookRegistry.DisableAllHooks(true)
	defer hooks.GlobalHookRegistry.DisableAllHooks(false)

	// Migrate
	err := db.AutoMigrate(&PGTestCourse{}, &PGTestChapter{}, &PGTestPage{})
	require.NoError(t, err)

	// Create hierarchy
	course := PGTestCourse{Name: "Course A"}
	db.Create(&course)

	chapter1 := PGTestChapter{Name: "Chapter 1", CourseID: course.ID}
	chapter2 := PGTestChapter{Name: "Chapter 2", CourseID: course.ID}
	db.Create(&chapter1)
	db.Create(&chapter2)

	page1 := PGTestPage{Name: "Page 1.1", ChapterID: chapter1.ID}
	page2 := PGTestPage{Name: "Page 1.2", ChapterID: chapter1.ID}
	page3 := PGTestPage{Name: "Page 2.1", ChapterID: chapter2.ID}
	db.Create(&page1)
	db.Create(&page2)
	db.Create(&page3)

	// Query pages by chapter
	var pagesInChapter1 []PGTestPage
	err = db.Where("chapter_id = ?", chapter1.ID).Find(&pagesInChapter1).Error
	require.NoError(t, err)
	assert.Equal(t, 2, len(pagesInChapter1))

	// Query pages by course (via join)
	var pagesInCourse []PGTestPage
	err = db.Joins("JOIN pg_test_chapters ON pg_test_pages.chapter_id = pg_test_chapters.id").
		Where("pg_test_chapters.course_id = ?", course.ID).
		Find(&pagesInCourse).Error
	require.NoError(t, err)
	assert.Equal(t, 3, len(pagesInCourse))

	// Cleanup
	db.Exec("DROP TABLE pg_test_pages CASCADE")
	db.Exec("DROP TABLE pg_test_chapters CASCADE")
	db.Exec("DROP TABLE pg_test_courses CASCADE")

	t.Logf("✅ PostgreSQL FK relationships test passed - found %d pages in course", len(pagesInCourse))
}

func TestPostgres_ManyToManyJoinTable(t *testing.T) {
	SkipIfNoPostgres(t)

	db := SetupPostgresTestDB(t)
	if db == nil {
		return
	}

	hooks.GlobalHookRegistry.DisableAllHooks(true)
	defer hooks.GlobalHookRegistry.DisableAllHooks(false)

	// Migrate
	err := db.AutoMigrate(&Student{}, &PGCourse{})
	require.NoError(t, err)

	// Create students
	student1 := Student{Name: "Alice"}
	student2 := Student{Name: "Bob"}
	db.Create(&student1)
	db.Create(&student2)

	// Create courses
	course1 := PGCourse{Name: "Go Programming"}
	course2 := PGCourse{Name: "PostgreSQL"}
	db.Create(&course1)
	db.Create(&course2)

	// Associate students with courses
	err = db.Model(&student1).Association("Courses").Append(&course1, &course2)
	require.NoError(t, err)

	err = db.Model(&student2).Association("Courses").Append(&course1)
	require.NoError(t, err)

	// Query: Get all students in "Go Programming" - use association
	var studentsInGo []Student
	err = db.Model(&course1).Association("Students").Find(&studentsInGo)
	require.NoError(t, err)
	assert.Equal(t, 2, len(studentsInGo), "Go Programming should have 2 students")

	// Query: Get all courses for Alice - use association
	var aliceCourses []PGCourse
	err = db.Model(&student1).Association("Courses").Find(&aliceCourses)
	require.NoError(t, err)
	assert.Equal(t, 2, len(aliceCourses), "Alice should have 2 courses")

	// Cleanup
	db.Exec("DROP TABLE student_courses CASCADE")
	db.Exec("DROP TABLE students CASCADE")
	db.Exec("DROP TABLE pg_courses CASCADE")

	t.Logf("✅ PostgreSQL many-to-many test passed - %d students in Go course", len(studentsInGo))
}

func TestPostgres_TransactionSupport(t *testing.T) {
	SkipIfNoPostgres(t)

	db := SetupPostgresTestDB(t)
	if db == nil {
		return
	}

	hooks.GlobalHookRegistry.DisableAllHooks(true)
	defer hooks.GlobalHookRegistry.DisableAllHooks(false)

	err := db.AutoMigrate(&PGTestCourse{})
	require.NoError(t, err)

	// Test successful transaction
	err = db.Transaction(func(tx *gorm.DB) error {
		course1 := PGTestCourse{Name: "TX Course 1"}
		if err := tx.Create(&course1).Error; err != nil {
			return err
		}

		course2 := PGTestCourse{Name: "TX Course 2"}
		if err := tx.Create(&course2).Error; err != nil {
			return err
		}

		return nil
	})
	require.NoError(t, err)

	var count int64
	db.Model(&PGTestCourse{}).Count(&count)
	assert.Equal(t, int64(2), count, "Transaction should have committed 2 courses")

	// Test rollback
	initialCount := count
	err = db.Transaction(func(tx *gorm.DB) error {
		course3 := PGTestCourse{Name: "TX Course 3"}
		if err := tx.Create(&course3).Error; err != nil {
			return err
		}

		// Force rollback
		return assert.AnError
	})
	require.Error(t, err)

	db.Model(&PGTestCourse{}).Count(&count)
	assert.Equal(t, initialCount, count, "Failed transaction should have rolled back")

	// Cleanup
	db.Exec("DROP TABLE pg_test_courses CASCADE")

	t.Log("✅ PostgreSQL transaction test passed")
}

func TestPostgres_ConcurrentAccess(t *testing.T) {
	SkipIfNoPostgres(t)

	db := SetupPostgresTestDB(t)
	if db == nil {
		return
	}

	hooks.GlobalHookRegistry.DisableAllHooks(true)
	defer hooks.GlobalHookRegistry.DisableAllHooks(false)

	err := db.AutoMigrate(&PGTestCourse{})
	require.NoError(t, err)

	// Simulate concurrent writes
	done := make(chan bool)
	for i := 0; i < 5; i++ {
		go func(id int) {
			course := PGTestCourse{Name: "Concurrent Course " + string(rune('A'+id))}
			db.Create(&course)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 5; i++ {
		<-done
	}

	var count int64
	db.Model(&PGTestCourse{}).Count(&count)
	assert.Equal(t, int64(5), count, "All concurrent inserts should succeed")

	// Cleanup
	db.Exec("DROP TABLE pg_test_courses CASCADE")

	t.Log("✅ PostgreSQL concurrent access test passed")
}
