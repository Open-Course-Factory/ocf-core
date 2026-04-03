package scenarios_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	access "soli/formations/src/auth/access"
	"soli/formations/src/auth/mocks"
	orgModels "soli/formations/src/organizations/models"
	"soli/formations/src/scenarios/models"
	scenarioController "soli/formations/src/scenarios/routes"
	"soli/formations/src/scenarios/services"

	"gorm.io/gorm"
)

// --- Helper: create a full source scenario with steps, hints, project files, and instance types ---

func createFullSourceScenario(t *testing.T, db *gorm.DB, orgID *uuid.UUID) models.Scenario {
	t.Helper()

	scenario := models.Scenario{
		Name:           "test-scenario",
		Title:          "Test Scenario",
		Description:    "A test scenario for duplication",
		Difficulty:     "intermediate",
		EstimatedTime:  "30m",
		InstanceType:   "ubuntu:22.04",
		Hostname:       "lab-host",
		OsType:         "deb",
		SourceType:     "upload",
		FlagsEnabled:   true,
		FlagSecret:     "original-secret-that-should-not-be-copied",
		GshEnabled:     true,
		CrashTraps:     false,
		Objectives:     "Learn to duplicate",
		Prerequisites:  "None",
		IntroText:      "Welcome to the test",
		FinishText:     "Congratulations!",
		SetupScript:    "#!/bin/bash\necho setup",
		CreatedByID:    "original-user",
		OrganizationID: orgID,
		IsPublic:       true,
	}
	require.NoError(t, db.Create(&scenario).Error)

	// Create scenario-level project files
	setupFile := models.ProjectFile{
		Name: "background.sh", RelPath: "background.sh", ContentType: "script",
		Content: "#!/bin/bash\necho setup", StorageType: "database", SizeBytes: 25,
	}
	require.NoError(t, db.Create(&setupFile).Error)

	introFile := models.ProjectFile{
		Name: "intro.md", RelPath: "intro.md", ContentType: "markdown",
		Content: "Welcome to the test", StorageType: "database", SizeBytes: 19,
	}
	require.NoError(t, db.Create(&introFile).Error)

	finishFile := models.ProjectFile{
		Name: "finish.md", RelPath: "finish.md", ContentType: "markdown",
		Content: "Congratulations!", StorageType: "database", SizeBytes: 17,
	}
	require.NoError(t, db.Create(&finishFile).Error)

	// Update scenario FKs
	require.NoError(t, db.Model(&scenario).Updates(map[string]any{
		"setup_script_id": setupFile.ID,
		"intro_file_id":   introFile.ID,
		"finish_file_id":  finishFile.ID,
	}).Error)

	// Create step-level project files
	verifyFile := models.ProjectFile{
		Name: "verify.sh", RelPath: "step1/verify.sh", ContentType: "script",
		Content: "#!/bin/bash\nexit 0", StorageType: "database", SizeBytes: 20,
	}
	require.NoError(t, db.Create(&verifyFile).Error)

	bgFile := models.ProjectFile{
		Name: "background.sh", RelPath: "step1/background.sh", ContentType: "script",
		Content: "#!/bin/bash\necho bg", StorageType: "database", SizeBytes: 21,
	}
	require.NoError(t, db.Create(&bgFile).Error)

	textFile := models.ProjectFile{
		Name: "text.md", RelPath: "step1/text.md", ContentType: "markdown",
		Content: "# Step 1\nDo something", StorageType: "database", SizeBytes: 22,
	}
	require.NoError(t, db.Create(&textFile).Error)

	// Create an image file linked to the scenario
	imageFile := models.ProjectFile{
		Name: "diagram.png", RelPath: "images/diagram.png", ContentType: "image",
		MimeType: "image/png", Content: "base64imagedata", StorageType: "database",
		SizeBytes: 15, ScenarioID: &scenario.ID,
	}
	require.NoError(t, db.Create(&imageFile).Error)

	// Create step
	step1 := models.ScenarioStep{
		ScenarioID:         scenario.ID,
		Order:              0,
		Title:              "Step 1: Do the thing",
		TextContent:        "# Step 1\nDo something",
		HintContent:        "Try running the command",
		VerifyScript:       "#!/bin/bash\nexit 0",
		BackgroundScript:   "#!/bin/bash\necho bg",
		HasFlag:            true,
		FlagPath:           "/tmp/flag1",
		FlagLevel:          1,
		VerifyScriptID:     &verifyFile.ID,
		BackgroundScriptID: &bgFile.ID,
		TextFileID:         &textFile.ID,
	}
	require.NoError(t, db.Create(&step1).Error)

	step2 := models.ScenarioStep{
		ScenarioID:  scenario.ID,
		Order:       1,
		Title:       "Step 2: Verify",
		TextContent: "Check the result",
		HasFlag:     false,
	}
	require.NoError(t, db.Create(&step2).Error)

	// Create hints for step 1
	hint1 := models.ScenarioStepHint{StepID: step1.ID, Level: 1, Content: "Hint 1: Read the docs"}
	hint2 := models.ScenarioStepHint{StepID: step1.ID, Level: 2, Content: "Hint 2: Use --help"}
	require.NoError(t, db.Create(&hint1).Error)
	require.NoError(t, db.Create(&hint2).Error)

	// Create hint for step 2
	hint3 := models.ScenarioStepHint{StepID: step2.ID, Level: 1, Content: "Just verify"}
	require.NoError(t, db.Create(&hint3).Error)

	// Create compatible instance types
	it1 := models.ScenarioInstanceType{
		ScenarioID: scenario.ID, InstanceType: "ubuntu:22.04", OsType: "deb", Priority: 0,
	}
	it2 := models.ScenarioInstanceType{
		ScenarioID: scenario.ID, InstanceType: "debian:12", OsType: "deb", Priority: 1,
	}
	require.NoError(t, db.Create(&it1).Error)
	require.NoError(t, db.Create(&it2).Error)

	// Create a session and assignment to verify they are NOT duplicated
	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "some-user", Status: "active",
	}
	require.NoError(t, db.Create(&session).Error)

	groupID := uuid.New()
	assignment := models.ScenarioAssignment{
		ScenarioID: scenario.ID, GroupID: &groupID, Scope: "group", CreatedByID: "original-user",
	}
	require.NoError(t, db.Create(&assignment).Error)

	// Reload with all relations
	var loaded models.Scenario
	require.NoError(t, db.
		Preload("Steps", func(db *gorm.DB) *gorm.DB { return db.Order("\"order\" ASC") }).
		Preload("Steps.Hints").
		Preload("CompatibleInstanceTypes").
		First(&loaded, "id = ?", scenario.ID).Error)

	return loaded
}

// --- Service-level tests ---

func TestDuplicateScenario_Success(t *testing.T) {
	db := freshTestDB(t)
	source := createFullSourceScenario(t, db, nil)
	svc := services.NewScenarioDuplicateService(db)

	result, err := svc.DuplicateScenario(source.ID, "new-user", nil)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Basic fields copied
	assert.Equal(t, "Test Scenario (Copy)", result.Title)
	assert.Equal(t, source.Description, result.Description)
	assert.Equal(t, source.Difficulty, result.Difficulty)
	assert.Equal(t, source.EstimatedTime, result.EstimatedTime)
	assert.Equal(t, source.InstanceType, result.InstanceType)
	assert.Equal(t, source.Hostname, result.Hostname)
	assert.Equal(t, source.OsType, result.OsType)
	assert.Equal(t, source.FlagsEnabled, result.FlagsEnabled)
	assert.Equal(t, source.GshEnabled, result.GshEnabled)
	assert.Equal(t, source.CrashTraps, result.CrashTraps)
	assert.Equal(t, source.Objectives, result.Objectives)
	assert.Equal(t, source.Prerequisites, result.Prerequisites)
	assert.Equal(t, source.IntroText, result.IntroText)
	assert.Equal(t, source.FinishText, result.FinishText)
	assert.Equal(t, source.IsPublic, result.IsPublic)

	// Steps count matches
	assert.Len(t, result.Steps, 2)
	assert.Equal(t, "Step 1: Do the thing", result.Steps[0].Title)
	assert.Equal(t, "Step 2: Verify", result.Steps[1].Title)
	assert.Equal(t, 0, result.Steps[0].Order)
	assert.Equal(t, 1, result.Steps[1].Order)

	// Step content copied
	assert.Equal(t, source.Steps[0].TextContent, result.Steps[0].TextContent)
	assert.Equal(t, source.Steps[0].HintContent, result.Steps[0].HintContent)
	assert.Equal(t, source.Steps[0].HasFlag, result.Steps[0].HasFlag)
	assert.Equal(t, source.Steps[0].FlagPath, result.Steps[0].FlagPath)

	// Hints copied
	assert.Len(t, result.Steps[0].Hints, 2)
	assert.Equal(t, "Hint 1: Read the docs", result.Steps[0].Hints[0].Content)
	assert.Equal(t, "Hint 2: Use --help", result.Steps[0].Hints[1].Content)
	assert.Len(t, result.Steps[1].Hints, 1)

	// Instance types copied
	assert.Len(t, result.CompatibleInstanceTypes, 2)

	// CreatedByID is the new user
	assert.Equal(t, "new-user", result.CreatedByID)
}

func TestDuplicateScenario_TitleHasCopySuffix(t *testing.T) {
	db := freshTestDB(t)
	source := createFullSourceScenario(t, db, nil)
	svc := services.NewScenarioDuplicateService(db)

	result, err := svc.DuplicateScenario(source.ID, "user1", nil)
	require.NoError(t, err)

	assert.Equal(t, "Test Scenario (Copy)", result.Title)
	assert.Contains(t, result.Name, "copy")
}

func TestDuplicateScenario_NewIDs(t *testing.T) {
	db := freshTestDB(t)
	source := createFullSourceScenario(t, db, nil)
	svc := services.NewScenarioDuplicateService(db)

	result, err := svc.DuplicateScenario(source.ID, "user1", nil)
	require.NoError(t, err)

	// Scenario ID is new
	assert.NotEqual(t, source.ID, result.ID)

	// Step IDs are new
	for i, step := range result.Steps {
		assert.NotEqual(t, source.Steps[i].ID, step.ID)
		assert.Equal(t, result.ID, step.ScenarioID)
	}

	// Hint IDs are new
	for i, step := range result.Steps {
		for j, hint := range step.Hints {
			assert.NotEqual(t, source.Steps[i].Hints[j].ID, hint.ID)
			assert.Equal(t, step.ID, hint.StepID)
		}
	}

	// Instance type IDs are new
	for i, it := range result.CompatibleInstanceTypes {
		assert.NotEqual(t, source.CompatibleInstanceTypes[i].ID, it.ID)
		assert.Equal(t, result.ID, it.ScenarioID)
	}
}

func TestDuplicateScenario_StepFKsUpdated(t *testing.T) {
	db := freshTestDB(t)
	source := createFullSourceScenario(t, db, nil)
	svc := services.NewScenarioDuplicateService(db)

	result, err := svc.DuplicateScenario(source.ID, "user1", nil)
	require.NoError(t, err)

	// Reload steps with FK fields (the preload may not include them)
	var newSteps []models.ScenarioStep
	require.NoError(t, db.Where("scenario_id = ?", result.ID).Order("\"order\" ASC").Find(&newSteps).Error)
	require.Len(t, newSteps, 2)

	// Step 0 had VerifyScriptID, BackgroundScriptID, TextFileID set
	assert.NotNil(t, newSteps[0].VerifyScriptID)
	assert.NotNil(t, newSteps[0].BackgroundScriptID)
	assert.NotNil(t, newSteps[0].TextFileID)

	// Verify the FKs point to new project files (not the source ones)
	var sourceSteps []models.ScenarioStep
	require.NoError(t, db.Where("scenario_id = ?", source.ID).Order("\"order\" ASC").Find(&sourceSteps).Error)

	assert.NotEqual(t, *sourceSteps[0].VerifyScriptID, *newSteps[0].VerifyScriptID)
	assert.NotEqual(t, *sourceSteps[0].BackgroundScriptID, *newSteps[0].BackgroundScriptID)
	assert.NotEqual(t, *sourceSteps[0].TextFileID, *newSteps[0].TextFileID)

	// Verify the new project files actually exist and have the right content
	var verifyFile models.ProjectFile
	require.NoError(t, db.First(&verifyFile, "id = ?", *newSteps[0].VerifyScriptID).Error)
	assert.Equal(t, "#!/bin/bash\nexit 0", verifyFile.Content)

	// Verify scenario-level FKs are also remapped
	var newScenario models.Scenario
	require.NoError(t, db.First(&newScenario, "id = ?", result.ID).Error)
	assert.NotNil(t, newScenario.SetupScriptID)
	assert.NotNil(t, newScenario.IntroFileID)
	assert.NotNil(t, newScenario.FinishFileID)

	// Reload the source to compare
	var srcScenario models.Scenario
	require.NoError(t, db.First(&srcScenario, "id = ?", source.ID).Error)
	assert.NotEqual(t, *srcScenario.SetupScriptID, *newScenario.SetupScriptID)
	assert.NotEqual(t, *srcScenario.IntroFileID, *newScenario.IntroFileID)
	assert.NotEqual(t, *srcScenario.FinishFileID, *newScenario.FinishFileID)
}

func TestDuplicateScenario_FlagSecretRegenerated(t *testing.T) {
	db := freshTestDB(t)
	source := createFullSourceScenario(t, db, nil)
	svc := services.NewScenarioDuplicateService(db)

	result, err := svc.DuplicateScenario(source.ID, "user1", nil)
	require.NoError(t, err)

	// FlagSecret should be regenerated (not copied from source)
	var newScenario models.Scenario
	require.NoError(t, db.First(&newScenario, "id = ?", result.ID).Error)
	assert.NotEmpty(t, newScenario.FlagSecret)
	assert.NotEqual(t, "original-secret-that-should-not-be-copied", newScenario.FlagSecret)
}

func TestDuplicateScenario_AssignmentsNotCopied(t *testing.T) {
	db := freshTestDB(t)
	source := createFullSourceScenario(t, db, nil)
	svc := services.NewScenarioDuplicateService(db)

	result, err := svc.DuplicateScenario(source.ID, "user1", nil)
	require.NoError(t, err)

	// No assignments on the new scenario
	var assignmentCount int64
	db.Model(&models.ScenarioAssignment{}).Where("scenario_id = ?", result.ID).Count(&assignmentCount)
	assert.Equal(t, int64(0), assignmentCount)

	// No sessions on the new scenario
	var sessionCount int64
	db.Model(&models.ScenarioSession{}).Where("scenario_id = ?", result.ID).Count(&sessionCount)
	assert.Equal(t, int64(0), sessionCount)
}

func TestDuplicateScenario_NotFound(t *testing.T) {
	db := freshTestDB(t)
	svc := services.NewScenarioDuplicateService(db)

	_, err := svc.DuplicateScenario(uuid.New(), "user1", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scenario not found")
}

func TestDuplicateScenario_ImageFilesCopied(t *testing.T) {
	db := freshTestDB(t)
	source := createFullSourceScenario(t, db, nil)
	svc := services.NewScenarioDuplicateService(db)

	result, err := svc.DuplicateScenario(source.ID, "user1", nil)
	require.NoError(t, err)

	// Check that image files were copied and linked to the new scenario
	var imageFiles []models.ProjectFile
	require.NoError(t, db.Where("scenario_id = ? AND content_type = ?", result.ID, "image").Find(&imageFiles).Error)
	assert.Len(t, imageFiles, 1)
	assert.Equal(t, "diagram.png", imageFiles[0].Name)
	assert.Equal(t, "base64imagedata", imageFiles[0].Content)
}

func TestDuplicateScenario_OrgIDSetOnCopy(t *testing.T) {
	db := freshTestDB(t)
	orgID := createTestOrg(t, db, "org-owner")
	source := createFullSourceScenario(t, db, &orgID)
	svc := services.NewScenarioDuplicateService(db)

	result, err := svc.DuplicateScenario(source.ID, "org-owner", &orgID)
	require.NoError(t, err)
	require.NotNil(t, result.OrganizationID)
	assert.Equal(t, orgID, *result.OrganizationID)
}

// --- Controller-level tests (HTTP) ---

func setupDuplicateTestRouter(t *testing.T, db *gorm.DB, userID string, roles []string) *gin.Engine {
	t.Helper()
	access.RouteRegistry.Reset()
	access.ResetEnforcers()
	t.Cleanup(func() {
		access.RouteRegistry.Reset()
		access.ResetEnforcers()
	})

	mockEnforcer := mocks.NewMockEnforcer()
	scenarioController.RegisterScenarioPermissions(mockEnforcer)
	access.RegisterBuiltinEnforcers(nil, access.NewGormMembershipChecker(db))

	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	api.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Set("userRoles", roles)
		c.Next()
	})
	api.Use(access.Layer2Enforcement())

	controller := scenarioController.NewScenarioController(db)
	api.POST("/scenarios/:id/duplicate", controller.DuplicateScenario)

	orgScenarios := api.Group("/organizations/:id/scenarios")
	orgScenarios.POST("/:scenarioId/duplicate", controller.OrgDuplicateScenario)

	return r
}

func TestDuplicateScenarioController_Success(t *testing.T) {
	db := freshTestDB(t)
	source := createFullSourceScenario(t, db, nil)

	router := setupDuplicateTestRouter(t, db, "admin-user", []string{"administrator"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/scenarios/"+source.ID.String()+"/duplicate", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "Test Scenario (Copy)", resp["title"])
	assert.NotEqual(t, source.ID.String(), resp["id"])
}

func TestDuplicateScenarioController_NotFound(t *testing.T) {
	db := freshTestDB(t)

	router := setupDuplicateTestRouter(t, db, "admin-user", []string{"administrator"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/scenarios/"+uuid.New().String()+"/duplicate", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestDuplicateScenarioController_InvalidID(t *testing.T) {
	db := freshTestDB(t)

	router := setupDuplicateTestRouter(t, db, "admin-user", []string{"administrator"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/scenarios/not-a-uuid/duplicate", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestOrgDuplicateScenarioController_Success(t *testing.T) {
	db := freshTestDB(t)
	orgID := createTestOrg(t, db, "org-manager")
	addOrgMember(t, db, orgID, "org-manager", orgModels.OrgRoleManager)
	source := createFullSourceScenario(t, db, &orgID)

	router := setupDuplicateTestRouter(t, db, "org-manager", []string{"member"})

	w := httptest.NewRecorder()
	url := "/api/v1/organizations/" + orgID.String() + "/scenarios/" + source.ID.String() + "/duplicate"
	req, _ := http.NewRequest("POST", url, nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "Test Scenario (Copy)", resp["title"])
}

func TestOrgDuplicateScenarioController_NotInOrg(t *testing.T) {
	db := freshTestDB(t)
	orgID := createTestOrg(t, db, "org-manager")
	addOrgMember(t, db, orgID, "org-manager", orgModels.OrgRoleManager)
	// Create scenario WITHOUT org
	source := createFullSourceScenario(t, db, nil)

	router := setupDuplicateTestRouter(t, db, "org-manager", []string{"member"})

	w := httptest.NewRecorder()
	url := "/api/v1/organizations/" + orgID.String() + "/scenarios/" + source.ID.String() + "/duplicate"
	req, _ := http.NewRequest("POST", url, nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
