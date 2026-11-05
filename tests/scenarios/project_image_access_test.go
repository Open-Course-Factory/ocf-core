package scenarios_test

// A scenario never leaves its organisation, and that includes its images. They
// are served to whoever can see the scenario or play it, and nobody else.

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	groupModels "soli/formations/src/groups/models"
	orgModels "soli/formations/src/organizations/models"
	"soli/formations/src/scenarios/models"
	scenarioController "soli/formations/src/scenarios/routes"
)

const scenarioImagePath = "step1/diagram.png"

func seedScenarioImage(t *testing.T, db *gorm.DB, scenarioID uuid.UUID) {
	t.Helper()
	require.NoError(t, db.Create(&models.ProjectFile{
		Name:        "diagram.png",
		RelPath:     scenarioImagePath,
		ContentType: "image",
		MimeType:    "image/png",
		Content:     "aW1hZ2U=", // base64 of "image"
		StorageType: "database",
		ScenarioID:  &scenarioID,
	}).Error)
}

func requestScenarioImage(t *testing.T, db *gorm.DB, scenarioID uuid.UUID, userID string, roles ...string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(ctx *gin.Context) {
		ctx.Set("userId", userID)
		ctx.Set("userRoles", append([]string{"member"}, roles...))
	})
	router.GET("/project-files/image/:scenarioId/*relPath", scenarioController.NewProjectFileController(db).GetImage)

	recorder := httptest.NewRecorder()
	request, err := http.NewRequest(http.MethodGet, "/project-files/image/"+scenarioID.String()+"/"+scenarioImagePath, nil)
	require.NoError(t, err)
	router.ServeHTTP(recorder, request)
	return recorder
}

func seedOrgScenarioWithImage(t *testing.T, db *gorm.DB) (uuid.UUID, *models.Scenario) {
	t.Helper()
	orgID := createTestOrg(t, db, "org-owner")
	scenario := createTestScenarioForOrg(t, db, orgID, "scenario-with-image")
	seedScenarioImage(t, db, scenario.ID)
	return orgID, scenario
}

func seedRunOf(t *testing.T, db *gorm.DB, scenarioID uuid.UUID, userID string) {
	t.Helper()
	require.NoError(t, db.Create(&models.ScenarioSession{
		ScenarioID: scenarioID,
		UserID:     userID,
		Status:     "completed",
		StartedAt:  time.Now(),
	}).Error)
}

func TestGetImage_ServesWhoeverCanSeeOrPlayTheScenario(t *testing.T) {
	tests := map[string]func(t *testing.T, db *gorm.DB, orgID uuid.UUID, scenario *models.Scenario){
		"a teacher of its organisation": func(t *testing.T, db *gorm.DB, orgID uuid.UUID, _ *models.Scenario) {
			addOrgMember(t, db, orgID, "the-user", orgModels.OrganizationMemberRole("teacher"))
		},
		"a manager of its organisation": func(t *testing.T, db *gorm.DB, orgID uuid.UUID, _ *models.Scenario) {
			addOrgMember(t, db, orgID, "the-user", orgModels.OrgRoleManager)
		},
		"a learner of a class it is assigned to": func(t *testing.T, db *gorm.DB, orgID uuid.UUID, scenario *models.Scenario) {
			groupID := createTestGroupInOrg(t, db, orgID, "org-owner")
			addGroupMember(t, db, groupID, "the-user", groupModels.GroupMemberRoleMember)
			createScenarioAssignment(t, db, scenario.ID, &groupID, nil, "group")
		},
		"a learner reviewing a run they did, with no assignment left": func(t *testing.T, db *gorm.DB, _ uuid.UUID, scenario *models.Scenario) {
			seedRunOf(t, db, scenario.ID, "the-user")
		},
	}
	for name, arrange := range tests {
		t.Run(name, func(t *testing.T) {
			db := freshTestDB(t)
			orgID, scenario := seedOrgScenarioWithImage(t, db)
			arrange(t, db, orgID, scenario)

			recorder := requestScenarioImage(t, db, scenario.ID, "the-user")

			assert.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			assert.Equal(t, "image", recorder.Body.String())
		})
	}
}

func TestGetImage_ServesAPublicCatalogueScenarioToAnyMember(t *testing.T) {
	db := freshTestDB(t)
	scenario := createTestScenarioNoOrg(t, db, "public-scenario")
	require.NoError(t, db.Model(scenario).Update("is_public", true).Error)
	seedScenarioImage(t, db, scenario.ID)

	assert.Equal(t, http.StatusOK, requestScenarioImage(t, db, scenario.ID, "anyone").Code)
}

func TestGetImage_ServesAnAdministrator(t *testing.T) {
	db := freshTestDB(t)
	_, scenario := seedOrgScenarioWithImage(t, db)

	assert.Equal(t, http.StatusOK, requestScenarioImage(t, db, scenario.ID, "platform-admin", "administrator").Code)
}

// Answered like a missing image, so the route does not confirm what exists.
func TestGetImage_RefusesEveryoneElseAsNotFound(t *testing.T) {
	tests := map[string]func(t *testing.T, db *gorm.DB, orgID uuid.UUID, scenario *models.Scenario){
		"a member of no organisation": func(*testing.T, *gorm.DB, uuid.UUID, *models.Scenario) {},
		"a manager of another organisation": func(t *testing.T, db *gorm.DB, _ uuid.UUID, _ *models.Scenario) {
			addOrgMember(t, db, createTestOrg(t, db, "other-owner"), "the-user", orgModels.OrgRoleManager)
		},
		"a plain member of its organisation, with no assignment": func(t *testing.T, db *gorm.DB, orgID uuid.UUID, _ *models.Scenario) {
			addOrgMember(t, db, orgID, "the-user", orgModels.OrgRoleMember)
		},
		"a learner of a class it is not assigned to": func(t *testing.T, db *gorm.DB, orgID uuid.UUID, _ *models.Scenario) {
			groupID := createTestGroupInOrg(t, db, orgID, "org-owner")
			addGroupMember(t, db, groupID, "the-user", groupModels.GroupMemberRoleMember)
		},
		"someone else's run": func(t *testing.T, db *gorm.DB, _ uuid.UUID, scenario *models.Scenario) {
			seedRunOf(t, db, scenario.ID, "another-user")
		},
	}
	for name, arrange := range tests {
		t.Run(name, func(t *testing.T) {
			db := freshTestDB(t)
			orgID, scenario := seedOrgScenarioWithImage(t, db)
			arrange(t, db, orgID, scenario)

			recorder := requestScenarioImage(t, db, scenario.ID, "the-user")

			assert.Equal(t, http.StatusNotFound, recorder.Code, recorder.Body.String())
		})
	}
}

func TestGetImage_UnknownScenario_IsNotFound(t *testing.T) {
	db := freshTestDB(t)

	assert.Equal(t, http.StatusNotFound, requestScenarioImage(t, db, uuid.New(), "anyone").Code)
}
