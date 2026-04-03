package worker_tests

import (
	"os"
	"path/filepath"
	"testing"

	authMocks "soli/formations/src/auth/mocks"
	"soli/formations/src/courses/models"
	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/worker/services"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Helper ---

func newMockCasdoorWithUser(email, name, displayName string) *authMocks.MockCasdoorService {
	mock := authMocks.NewMockCasdoorService()
	mock.AddUser(email, &casdoorsdk.User{
		Id:          "test-user-id",
		Name:        name,
		DisplayName: displayName,
		Email:       email,
	})
	return mock
}

func newMinimalCourse() *models.Course {
	return &models.Course{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Name:      "Linux Basics",
		Title:     "Introduction to Linux",
		Subtitle:  "Getting started with the command line",
		Version:   "1.0.0",
		Chapters:  []*models.Chapter{},
	}
}

// chdirTemp changes the working directory to a temp dir and returns a cleanup
// function. This allows config.COURSES_ROOT ("./courses/") and config.THEMES_ROOT
// ("./themes/") to resolve relative to the temp dir.
func chdirTemp(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmpDir))
	t.Cleanup(func() { os.Chdir(origDir) })
	return tmpDir
}

// --- GenerateMDContent ---

func TestGenerationPackageService_GenerateMDContent_ValidCourse(t *testing.T) {
	email := "author@example.com"
	mockCasdoor := newMockCasdoorWithUser(email, "authoruser", "Author User")
	svc := services.NewGenerationPackageServiceWithDependencies(mockCasdoor)

	course := newMinimalCourse()

	md, err := svc.GenerateMDContent(course, email)
	require.NoError(t, err)
	assert.NotEmpty(t, md)
}

func TestGenerationPackageService_GenerateMDContent_ReplacesPlaceholders(t *testing.T) {
	email := "author@example.com"
	mockCasdoor := newMockCasdoorWithUser(email, "jdoe", "Jane Doe")
	svc := services.NewGenerationPackageServiceWithDependencies(mockCasdoor)

	course := newMinimalCourse()
	course.Version = "2.3.1"

	md, err := svc.GenerateMDContent(course, email)
	require.NoError(t, err)

	// @@author_fullname@@, @@author_email@@, @@version@@ placeholders should be replaced.
	// Note: @@author@@ placeholder is not used in the current Slidev template,
	// so user.Name ("jdoe") won't appear unless the template changes.
	assert.Contains(t, md, "Jane Doe", "@@author_fullname@@ should be replaced with user.DisplayName")
	assert.Contains(t, md, "author@example.com", "@@author_email@@ should be replaced with email")
	assert.Contains(t, md, "2.3.1", "@@version@@ should be replaced with course version")

	// Raw placeholders that ARE used in the template should NOT remain
	assert.NotContains(t, md, "@@author_fullname@@")
	assert.NotContains(t, md, "@@author_email@@")
	assert.NotContains(t, md, "@@version@@")
}

func TestGenerationPackageService_GenerateMDContent_EmptyCourse(t *testing.T) {
	email := "author@example.com"
	mockCasdoor := newMockCasdoorWithUser(email, "testuser", "Test User")
	svc := services.NewGenerationPackageServiceWithDependencies(mockCasdoor)

	course := &models.Course{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Name:      "",
		Title:     "",
		Version:   "",
		Chapters:  []*models.Chapter{},
	}

	md, err := svc.GenerateMDContent(course, email)
	require.NoError(t, err)
	// Even an empty course should produce some Slidev frontmatter
	assert.NotEmpty(t, md)
}

func TestGenerationPackageService_GenerateMDContent_UnknownUser_ReturnsError(t *testing.T) {
	mockCasdoor := authMocks.NewMockCasdoorService()
	// Do NOT add the user -- "unknown@example.com" will not be found
	svc := services.NewGenerationPackageServiceWithDependencies(mockCasdoor)

	course := newMinimalCourse()

	_, err := svc.GenerateMDContent(course, "unknown@example.com")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "user")
}

func TestGenerationPackageService_GenerateMDContent_ContainsFrontMatter(t *testing.T) {
	email := "author@example.com"
	mockCasdoor := newMockCasdoorWithUser(email, "authoruser", "Author User")
	svc := services.NewGenerationPackageServiceWithDependencies(mockCasdoor)

	course := newMinimalCourse()
	course.Title = "Docker Deep Dive"

	md, err := svc.GenerateMDContent(course, email)
	require.NoError(t, err)

	// Slidev frontmatter should start with ---
	assert.True(t, len(md) > 0 && md[:3] == "---", "MD content should start with Slidev frontmatter delimiter")
}

// --- CollectAssets ---

func TestGenerationPackageService_CollectAssets_NoFolderName_ReturnsEmptyMap(t *testing.T) {
	mockCasdoor := authMocks.NewMockCasdoorService()
	svc := services.NewGenerationPackageServiceWithDependencies(mockCasdoor)

	course := &models.Course{
		BaseModel:  entityManagementModels.BaseModel{ID: uuid.New()},
		FolderName: "", // No folder
		Chapters:   []*models.Chapter{},
	}

	assets, err := svc.CollectAssets(course)
	require.NoError(t, err)
	assert.NotNil(t, assets)
	assert.Empty(t, assets)
}

func TestGenerationPackageService_CollectAssets_NonExistentFolder_ReturnsEmptyMap(t *testing.T) {
	mockCasdoor := authMocks.NewMockCasdoorService()
	svc := services.NewGenerationPackageServiceWithDependencies(mockCasdoor)

	course := &models.Course{
		BaseModel:  entityManagementModels.BaseModel{ID: uuid.New()},
		FolderName: "nonexistent-course-folder-12345",
		Chapters:   []*models.Chapter{},
	}

	assets, err := svc.CollectAssets(course)
	require.NoError(t, err)
	assert.NotNil(t, assets)
	assert.Empty(t, assets)
}

func TestGenerationPackageService_CollectAssets_WithRealTempDir(t *testing.T) {
	// config.COURSES_ROOT is "./courses/" (relative), so we chdir to a temp dir
	// and create the expected directory structure there.
	tmpDir := chdirTemp(t)

	courseName := "test-course"
	assetsDir := filepath.Join(tmpDir, "courses", courseName, "assets")
	err := os.MkdirAll(assetsDir, 0755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(assetsDir, "image.png"), []byte("fake png data"), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(assetsDir, "style.css"), []byte("body { color: red; }"), 0644)
	require.NoError(t, err)

	mockCasdoor := authMocks.NewMockCasdoorService()
	svc := services.NewGenerationPackageServiceWithDependencies(mockCasdoor)

	course := &models.Course{
		BaseModel:  entityManagementModels.BaseModel{ID: uuid.New()},
		FolderName: courseName,
		Chapters:   []*models.Chapter{},
	}

	assets, err := svc.CollectAssets(course)
	require.NoError(t, err)
	assert.Len(t, assets, 2)
	assert.Equal(t, []byte("fake png data"), assets["assets/image.png"])
	assert.Equal(t, []byte("body { color: red; }"), assets["assets/style.css"])
}

func TestGenerationPackageService_CollectAssets_NestedDirectories(t *testing.T) {
	tmpDir := chdirTemp(t)

	courseName := "nested-course"
	subDir := filepath.Join(tmpDir, "courses", courseName, "assets", "images", "chapter1")
	err := os.MkdirAll(subDir, 0755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(subDir, "diagram.svg"), []byte("<svg/>"), 0644)
	require.NoError(t, err)

	mockCasdoor := authMocks.NewMockCasdoorService()
	svc := services.NewGenerationPackageServiceWithDependencies(mockCasdoor)

	course := &models.Course{
		BaseModel:  entityManagementModels.BaseModel{ID: uuid.New()},
		FolderName: courseName,
		Chapters:   []*models.Chapter{},
	}

	assets, err := svc.CollectAssets(course)
	require.NoError(t, err)
	assert.Len(t, assets, 1)
	assert.Equal(t, []byte("<svg/>"), assets["assets/images/chapter1/diagram.svg"])
}

// --- CollectThemeFiles ---

func TestGenerationPackageService_CollectThemeFiles_NonExistentTheme_ReturnsEmpty(t *testing.T) {
	mockCasdoor := authMocks.NewMockCasdoorService()
	svc := services.NewGenerationPackageServiceWithDependencies(mockCasdoor)

	themeFiles, err := svc.CollectThemeFiles("nonexistent-custom-theme-xyz")
	require.NoError(t, err)
	assert.NotNil(t, themeFiles)
	assert.Empty(t, themeFiles)
}

func TestGenerationPackageService_CollectThemeFiles_WithRealTempDir(t *testing.T) {
	tmpDir := chdirTemp(t)

	themeName := "my-custom-theme"
	themeDir := filepath.Join(tmpDir, "themes", themeName)
	err := os.MkdirAll(themeDir, 0755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(themeDir, "style.css"), []byte("/* custom */"), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(themeDir, "setup.js"), []byte("export default {}"), 0644)
	require.NoError(t, err)

	mockCasdoor := authMocks.NewMockCasdoorService()
	svc := services.NewGenerationPackageServiceWithDependencies(mockCasdoor)

	themeFiles, err := svc.CollectThemeFiles(themeName)
	require.NoError(t, err)
	assert.Len(t, themeFiles, 2)
	assert.Equal(t, []byte("/* custom */"), themeFiles["theme/style.css"])
	assert.Equal(t, []byte("export default {}"), themeFiles["theme/setup.js"])
}

// --- PrepareGenerationPackage ---

func TestGenerationPackageService_PrepareGenerationPackage_ValidCourse(t *testing.T) {
	email := "author@example.com"
	mockCasdoor := newMockCasdoorWithUser(email, "authoruser", "Author User")
	svc := services.NewGenerationPackageServiceWithDependencies(mockCasdoor)

	course := newMinimalCourse()
	course.Theme = nil // defaults to "default"

	pkg, err := svc.PrepareGenerationPackage(course, email)
	require.NoError(t, err)
	assert.NotNil(t, pkg)
	assert.NotEmpty(t, pkg.MDContent)
	assert.NotNil(t, pkg.Assets)
	assert.NotNil(t, pkg.ThemeFiles)

	// Check metadata
	assert.Equal(t, course.ID.String(), pkg.Metadata.CourseID)
	assert.Equal(t, course.Name, pkg.Metadata.CourseName)
	assert.Equal(t, 1, pkg.Metadata.Format)
	assert.Equal(t, "default", pkg.Metadata.Theme)
	assert.Equal(t, "Author User", pkg.Metadata.Author)
	assert.Equal(t, course.Version, pkg.Metadata.Version)
}

func TestGenerationPackageService_PrepareGenerationPackage_UnknownUser_ReturnsError(t *testing.T) {
	mockCasdoor := authMocks.NewMockCasdoorService()
	mockCasdoor.ClearUsers()
	svc := services.NewGenerationPackageServiceWithDependencies(mockCasdoor)

	course := newMinimalCourse()

	_, err := svc.PrepareGenerationPackage(course, "nobody@example.com")
	assert.Error(t, err)
}

func TestGenerationPackageService_PrepareGenerationPackage_WithCustomTheme(t *testing.T) {
	tmpDir := chdirTemp(t)

	themeName := "ocf-custom"
	themeDir := filepath.Join(tmpDir, "themes", themeName)
	err := os.MkdirAll(themeDir, 0755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(themeDir, "layout.css"), []byte("/* layout */"), 0644)
	require.NoError(t, err)

	email := "trainer@example.com"
	mockCasdoor := newMockCasdoorWithUser(email, "trainer", "Master Trainer")
	svc := services.NewGenerationPackageServiceWithDependencies(mockCasdoor)

	course := newMinimalCourse()
	course.Theme = &models.Theme{Name: themeName}

	pkg, err := svc.PrepareGenerationPackage(course, email)
	require.NoError(t, err)
	assert.NotNil(t, pkg)
	assert.Equal(t, themeName, pkg.Metadata.Theme)
	assert.Len(t, pkg.ThemeFiles, 1)
	assert.Equal(t, []byte("/* layout */"), pkg.ThemeFiles["theme/layout.css"])
}

func TestGenerationPackageService_PrepareGenerationPackage_StandardTheme_NoThemeFiles(t *testing.T) {
	email := "author@example.com"
	mockCasdoor := newMockCasdoorWithUser(email, "authoruser", "Author User")
	svc := services.NewGenerationPackageServiceWithDependencies(mockCasdoor)

	course := newMinimalCourse()
	course.Theme = &models.Theme{Name: "seriph"}

	pkg, err := svc.PrepareGenerationPackage(course, email)
	require.NoError(t, err)
	assert.NotNil(t, pkg)
	assert.Equal(t, "seriph", pkg.Metadata.Theme)
	assert.Empty(t, pkg.ThemeFiles)
}

// --- Constructor ---

func TestNewGenerationPackageServiceWithDependencies_ReturnsNonNil(t *testing.T) {
	mockCasdoor := authMocks.NewMockCasdoorService()
	svc := services.NewGenerationPackageServiceWithDependencies(mockCasdoor)
	assert.NotNil(t, svc)
}

func TestNewGenerationPackageService_ReturnsNonNil(t *testing.T) {
	svc := services.NewGenerationPackageService()
	assert.NotNil(t, svc)
}
