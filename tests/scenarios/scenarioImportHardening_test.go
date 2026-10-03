package scenarios_test

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	groupModels "soli/formations/src/groups/models"
	"soli/formations/src/scenarios/models"
	scenarioController "soli/formations/src/scenarios/routes"
)

// setupImportHardeningRouter wires the import handlers alone (no Layer 2):
// the guards under test live in the handlers themselves.
func setupImportHardeningRouter(db *gorm.DB, userID string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Set("userRoles", []string{"Member"})
		c.Next()
	})
	mc := scenarioController.NewScenarioManagementController(db)
	r.POST("/groups/:groupId/scenarios/import-json", mc.GroupImportJSON)
	r.POST("/groups/:groupId/scenarios/upload", mc.GroupUploadScenario)
	r.POST("/organizations/:id/scenarios/import-json", mc.OrgImportJSON)
	return r
}

func createOrglessGroup(t *testing.T, db *gorm.DB, ownerUserID string) uuid.UUID {
	t.Helper()
	groupID := uuid.New()
	group := &groupModels.ClassGroup{
		Name:        "Legacy Class",
		DisplayName: "Legacy Class",
		OwnerUserID: ownerUserID,
		MaxMembers:  50,
	}
	group.ID = groupID
	require.NoError(t, db.Omit("Metadata").Create(group).Error)
	return groupID
}

func TestGroupImportJSON_OrglessClass_RefusedAndPlatformScenarioUntouched(t *testing.T) {
	db := freshTestDB(t)
	managerID := "orgless-class-manager"
	groupID := createOrglessGroup(t, db, managerID)
	platform := createTestScenarioNoOrg(t, db, "platform-target")
	platform.Title = "Platform Target"
	require.NoError(t, db.Save(platform).Error)

	router := setupImportHardeningRouter(db, managerID)
	body, _ := json.Marshal(map[string]any{
		"title":         "Platform Target",
		"description":   "hijacked",
		"instance_type": "ubuntu:22.04",
		"is_public":     true,
		"steps":         []map[string]any{{"title": "Step 1", "text_content": "x"}},
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/groups/"+groupID.String()+"/scenarios/import-json", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())

	var reloaded models.Scenario
	require.NoError(t, db.First(&reloaded, "id = ?", platform.ID).Error)
	assert.NotEqual(t, "hijacked", reloaded.Description)
	assert.False(t, reloaded.IsPublic)
	var count int64
	db.Model(&models.Scenario{}).Count(&count)
	assert.Equal(t, int64(1), count, "no scenario may be created either")
}

func TestGroupUploadScenario_OrglessClass_Refused(t *testing.T) {
	db := freshTestDB(t)
	managerID := "orgless-class-uploader"
	groupID := createOrglessGroup(t, db, managerID)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, _ := mw.CreateFormFile("file", "s.zip")
	part.Write([]byte("not really a zip"))
	mw.Close()

	router := setupImportHardeningRouter(db, managerID)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/groups/"+groupID.String()+"/scenarios/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())
}

func TestOrgImportJSON_OversizedBody_Returns413(t *testing.T) {
	db := freshTestDB(t)
	ownerID := "org-owner-oversized"
	orgID := createTestOrg(t, db, ownerID)

	payload := `{"title":"Huge","instance_type":"ubuntu:22.04","description":"` +
		strings.Repeat("a", 11*1024*1024) + `"}`

	router := setupImportHardeningRouter(db, ownerID)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/organizations/"+orgID.String()+"/scenarios/import-json", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
	var count int64
	db.Model(&models.Scenario{}).Count(&count)
	assert.Equal(t, int64(0), count)
}
