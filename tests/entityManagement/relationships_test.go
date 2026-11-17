// tests/entityManagement/relationships_test.go
package entityManagement_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"soli/formations/src/auth/casdoor"
	authInterfaces "soli/formations/src/auth/interfaces"
	authMocks "soli/formations/src/auth/mocks"
	authModels "soli/formations/src/auth/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	entityManagementModels "soli/formations/src/entityManagement/models"
	controller "soli/formations/src/entityManagement/routes"
)

// ============================================================================
// Relationship Test Entities (Course → Chapter → Section → Page)
// ============================================================================

type RelTestCourse struct {
	entityManagementModels.BaseModel
	Name     string           `json:"name"`
	Chapters []RelTestChapter `gorm:"many2many:course_chapters" json:"chapters"`
}

type RelTestChapter struct {
	entityManagementModels.BaseModel
	Name     string           `json:"name"`
	Order    int              `json:"order"`
	Courses  []RelTestCourse  `gorm:"many2many:course_chapters" json:"courses"`
	Sections []RelTestSection `gorm:"many2many:chapter_sections" json:"sections"`
}

type RelTestSection struct {
	entityManagementModels.BaseModel
	Name     string           `json:"name"`
	Order    int              `json:"order"`
	Chapters []RelTestChapter `gorm:"many2many:chapter_sections" json:"chapters"`
	Pages    []RelTestPage    `gorm:"many2many:section_pages" json:"pages"`
}

type RelTestPage struct {
	entityManagementModels.BaseModel
	Name     string           `json:"name"`
	Content  string           `json:"content"`
	Order    int              `json:"order"`
	Sections []RelTestSection `gorm:"many2many:section_pages" json:"sections"`
}

// Join tables
type CourseChapters struct {
	CourseID  uuid.UUID `gorm:"primaryKey"`
	ChapterID uuid.UUID `gorm:"primaryKey"`
	Order     int       `json:"order"`
}

type ChapterSections struct {
	ChapterID uuid.UUID `gorm:"primaryKey"`
	SectionID uuid.UUID `gorm:"primaryKey"`
	Order     int       `json:"order"`
}

type SectionPages struct {
	SectionID uuid.UUID `gorm:"primaryKey"`
	PageID    uuid.UUID `gorm:"primaryKey"`
	Order     int       `json:"order"`
}

// DTOs
type RelTestPageInput struct {
	Name    string `json:"name" binding:"required"`
	Content string `json:"content"`
	OwnerID string `json:"owner_id"`
}

type RelTestPageOutput struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Content  string   `json:"content"`
	OwnerIDs []string `json:"owner_ids"`
}

type RelTestSectionInput struct {
	Name    string `json:"name" binding:"required"`
	OwnerID string `json:"owner_id"`
}

type RelTestSectionOutput struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	OwnerIDs []string `json:"owner_ids"`
}

// registerRelTestPage registers the RelTestPage entity using typed generics.
func registerRelTestPage(service *ems.EntityRegistrationService) {
	ems.RegisterTypedEntity[RelTestPage, RelTestPageInput, map[string]any, RelTestPageOutput](
		service,
		"RelTestPage",
		entityManagementInterfaces.TypedEntityRegistration[RelTestPage, RelTestPageInput, map[string]any, RelTestPageOutput]{
			Converters: entityManagementInterfaces.TypedEntityConverters[RelTestPage, RelTestPageInput, map[string]any, RelTestPageOutput]{
				ModelToDto: func(entity *RelTestPage) (RelTestPageOutput, error) {
					return RelTestPageOutput{
						ID:       entity.ID.String(),
						Name:     entity.Name,
						Content:  entity.Content,
						OwnerIDs: entity.OwnerIDs,
					}, nil
				},
				DtoToModel: func(dto RelTestPageInput) *RelTestPage {
					entity := &RelTestPage{
						Name:    dto.Name,
						Content: dto.Content,
					}
					entity.OwnerIDs = append(entity.OwnerIDs, dto.OwnerID)
					return entity
				},
				DtoToMap: func(dto map[string]any) map[string]any {
					return dto
				},
			},
			Roles: entityManagementInterfaces.EntityRoles{
				Roles: map[string]string{
					string(authModels.Member): "(" + http.MethodGet + "|" + http.MethodPost + ")",
				},
			},
			RelationshipFilters: []entityManagementInterfaces.RelationshipFilter{
				{
					FilterName:   "courseId",
					TargetColumn: "id",
					Path: []entityManagementInterfaces.RelationshipStep{
						{
							JoinTable:    "section_pages",
							SourceColumn: "page_id",
							TargetColumn: "section_id",
							NextTable:    "sections",
						},
						{
							JoinTable:    "chapter_sections",
							SourceColumn: "section_id",
							TargetColumn: "chapter_id",
							NextTable:    "chapters",
						},
						{
							JoinTable:    "course_chapters",
							SourceColumn: "chapter_id",
							TargetColumn: "course_id",
							NextTable:    "courses",
						},
					},
				},
				{
					FilterName:   "chapterId",
					TargetColumn: "id",
					Path: []entityManagementInterfaces.RelationshipStep{
						{
							JoinTable:    "section_pages",
							SourceColumn: "page_id",
							TargetColumn: "section_id",
							NextTable:    "sections",
						},
						{
							JoinTable:    "chapter_sections",
							SourceColumn: "section_id",
							TargetColumn: "chapter_id",
							NextTable:    "chapters",
						},
					},
				},
				{
					FilterName:   "sectionId",
					TargetColumn: "id",
					Path: []entityManagementInterfaces.RelationshipStep{
						{
							JoinTable:    "section_pages",
							SourceColumn: "page_id",
							TargetColumn: "section_id",
							NextTable:    "sections",
						},
					},
				},
			},
		},
	)
}

// registerRelTestSection registers the RelTestSection entity using typed generics.
func registerRelTestSection(service *ems.EntityRegistrationService) {
	ems.RegisterTypedEntity[RelTestSection, RelTestSectionInput, map[string]any, RelTestSectionOutput](
		service,
		"RelTestSection",
		entityManagementInterfaces.TypedEntityRegistration[RelTestSection, RelTestSectionInput, map[string]any, RelTestSectionOutput]{
			Converters: entityManagementInterfaces.TypedEntityConverters[RelTestSection, RelTestSectionInput, map[string]any, RelTestSectionOutput]{
				ModelToDto: func(entity *RelTestSection) (RelTestSectionOutput, error) {
					return RelTestSectionOutput{
						ID:       entity.ID.String(),
						Name:     entity.Name,
						OwnerIDs: entity.OwnerIDs,
					}, nil
				},
				DtoToModel: func(dto RelTestSectionInput) *RelTestSection {
					entity := &RelTestSection{
						Name: dto.Name,
					}
					entity.OwnerIDs = append(entity.OwnerIDs, dto.OwnerID)
					return entity
				},
				DtoToMap: func(dto map[string]any) map[string]any {
					return dto
				},
			},
			Roles: entityManagementInterfaces.EntityRoles{
				Roles: map[string]string{
					string(authModels.Member): "(" + http.MethodGet + "|" + http.MethodPost + ")",
				},
			},
			RelationshipFilters: []entityManagementInterfaces.RelationshipFilter{
				{
					FilterName:   "chapterId",
					TargetColumn: "id",
					Path: []entityManagementInterfaces.RelationshipStep{
						{
							JoinTable:    "chapter_sections",
							SourceColumn: "section_id",
							TargetColumn: "chapter_id",
							NextTable:    "chapters",
						},
					},
				},
			},
		},
	)
}

// ============================================================================
// Relationship Test Suite
// ============================================================================

type RelationshipTestSuite struct {
	db                *gorm.DB
	router            *gin.Engine
	mockEnforcer      *authMocks.MockEnforcer
	pageController    controller.GenericController
	sectionController controller.GenericController
	originalEnforcer  authInterfaces.EnforcerInterface
}

func setupRelationshipTest(t *testing.T) *RelationshipTestSuite {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Migrate all tables
	err = db.AutoMigrate(
		&RelTestCourse{},
		&RelTestChapter{},
		&RelTestSection{},
		&RelTestPage{},
		&CourseChapters{},
		&ChapterSections{},
		&SectionPages{},
	)
	require.NoError(t, err)

	// Setup join tables
	_ = db.SetupJoinTable(&RelTestCourse{}, "Chapters", &CourseChapters{})
	_ = db.SetupJoinTable(&RelTestChapter{}, "Courses", &CourseChapters{})
	_ = db.SetupJoinTable(&RelTestChapter{}, "Sections", &ChapterSections{})
	_ = db.SetupJoinTable(&RelTestSection{}, "Chapters", &ChapterSections{})
	_ = db.SetupJoinTable(&RelTestSection{}, "Pages", &SectionPages{})
	_ = db.SetupJoinTable(&RelTestPage{}, "Sections", &SectionPages{})

	mockEnforcer := authMocks.NewMockEnforcer()
	mockEnforcer.LoadPolicyFunc = func() error { return nil }
	mockEnforcer.AddPolicyFunc = func(params ...any) (bool, error) { return true, nil }

	suite := &RelationshipTestSuite{
		db:               db,
		mockEnforcer:     mockEnforcer,
		originalEnforcer: casdoor.Enforcer,
	}
	casdoor.Enforcer = mockEnforcer

	gin.SetMode(gin.TestMode)
	router := gin.New()
	suite.router = router

	// Register entities with Global service
	registerRelTestPage(ems.GlobalEntityRegistrationService)
	registerRelTestSection(ems.GlobalEntityRegistrationService)

	t.Cleanup(func() {
		casdoor.Enforcer = suite.originalEnforcer
	})

	suite.pageController = controller.NewGenericController(db, nil)
	suite.sectionController = controller.NewGenericController(db, nil)

	// Add test middleware to inject userId
	router.Use(func(ctx *gin.Context) {
		ctx.Set("userId", "test-user-123")
		ctx.Next()
	})

	apiGroup := router.Group("/api/v1")
	apiGroup.POST("/rel-test-pages", suite.pageController.AddEntity)
	apiGroup.GET("/rel-test-pages", suite.pageController.GetEntities)
	apiGroup.POST("/rel-test-sections", suite.sectionController.AddEntity)
	apiGroup.GET("/rel-test-sections", suite.sectionController.GetEntities)

	return suite
}

// ============================================================================
// Relationship Filter Tests
// ============================================================================

func TestRelationships_NoResults(t *testing.T) {
	suite := setupRelationshipTest(t)
	userID := "test-user"

	// Create a course with no pages
	course := RelTestCourse{Name: "Empty Course"}
	suite.db.Create(&course)

	// Create a page not linked to this course
	page := RelTestPage{Name: "Unlinked Page", Content: "Content", Order: 1}
	page.OwnerIDs = []string{userID}
	suite.db.Create(&page)

	// Filter by the empty course
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/rel-test-pages?courseId=%s&page=1&size=10", course.ID), nil)
	w := httptest.NewRecorder()

	suite.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Data  []RelTestPageOutput `json:"data"`
		Total int64               `json:"total"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err, "Failed to unmarshal response")

	assert.Equal(t, int64(0), response.Total)
	assert.Empty(t, response.Data)

	t.Logf("✅ No results returned correctly for unlinked course")
}
