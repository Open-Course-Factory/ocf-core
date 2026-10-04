package scenarios_test

// Teachers author scenarios (hotfix, 2026-10-04).
//
// An organisation teacher — the rank that runs classes,
// access.RoleMinimumForClassrooms — writes the labs those classes run, so
// every authoring route of an organisation opens at that rank. Editing,
// replacing and deleting a scenario stay with its author and with the
// organisation's managers: one teacher never rewrites or removes another's
// lab. Teachers of the organisation still run their colleagues'
// labs: see them in full, preview, assign, export and copy them.
//
// At class level the roles are owner / manager / member; whoever owns or
// manages a class teaches it, so the class routes keep MinRole manager.

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	access "soli/formations/src/auth/access"
	"soli/formations/src/auth/mocks"
	"soli/formations/src/entityManagement/hooks"
	groupModels "soli/formations/src/groups/models"
	orgModels "soli/formations/src/organizations/models"
	scenarioHooks "soli/formations/src/scenarios/hooks"
	"soli/formations/src/scenarios/models"
	scenarioController "soli/formations/src/scenarios/routes"
)

const orgRoleTeacher = orgModels.OrganizationMemberRole(access.RoleTeacher)

// teacherAuthoringFixture: one org with an owner, a manager, two teachers and
// a plain member; a class owned by teacher A; one scenario by each teacher.
type teacherAuthoringFixture struct {
	org                    uuid.UUID
	class                  uuid.UUID
	byTeacherA, byTeacherB *models.Scenario
}

func buildTeacherAuthoringFixture(t *testing.T, db *gorm.DB) teacherAuthoringFixture {
	t.Helper()
	f := teacherAuthoringFixture{org: createTestOrg(t, db, "ta-owner")}
	addOrgMember(t, db, f.org, "ta-owner", orgModels.OrgRoleOwner)
	addOrgMember(t, db, f.org, "ta-manager", orgModels.OrgRoleManager)
	addOrgMember(t, db, f.org, "ta-teacher-a", orgRoleTeacher)
	addOrgMember(t, db, f.org, "ta-teacher-b", orgRoleTeacher)
	addOrgMember(t, db, f.org, "ta-student", orgModels.OrgRoleMember)
	f.class = createTestGroupInOrg(t, db, f.org, "ta-teacher-a")
	addGroupMember(t, db, f.class, "ta-teacher-a", groupModels.GroupMemberRoleOwner)
	addGroupMember(t, db, f.class, "ta-student", groupModels.GroupMemberRoleMember)
	f.byTeacherA = createTeacherScenario(t, db, f.org, "lab-by-a", "ta-teacher-a")
	f.byTeacherB = createTeacherScenario(t, db, f.org, "lab-by-b", "ta-teacher-b")
	return f
}

func createTeacherScenario(t *testing.T, db *gorm.DB, orgID uuid.UUID, name, author string) *models.Scenario {
	t.Helper()
	s := createTestScenarioForOrg(t, db, orgID, name)
	require.NoError(t, db.Model(s).Update("created_by_id", author).Error)
	s.CreatedByID = author
	return s
}

// authoringRouter mounts every org and class authoring route behind the real
// Layer 2 middleware and the production route declarations.
func authoringRouter(t *testing.T, db *gorm.DB, userID string) *gin.Engine {
	t.Helper()
	access.RouteRegistry.Reset()
	access.ResetEnforcers()
	t.Cleanup(func() {
		access.RouteRegistry.Reset()
		access.ResetEnforcers()
	})
	scenarioController.RegisterScenarioPermissions(mocks.NewMockEnforcer())
	access.RegisterBuiltinEnforcers(nil, access.NewGormMembershipChecker(db))

	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	api.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Set("userRoles", []string{"member"})
		c.Next()
	})
	api.Use(access.Layer2Enforcement())

	mc := scenarioController.NewScenarioManagementController(db)
	org := api.Group("/organizations/:id/scenarios")
	org.GET("", mc.OrgListScenarios)
	org.POST("", mc.OrgCreateScenario)
	org.POST("/upload", mc.OrgUploadScenario)
	org.POST("/import-json", mc.OrgImportJSON)
	org.GET("/:scenarioId/export", mc.OrgExportScenario)
	org.DELETE("/:scenarioId", mc.OrgDeleteScenario)
	org.POST("/:scenarioId/duplicate", mc.OrgDuplicateScenario)
	group := api.Group("/groups/:groupId/scenarios")
	group.GET("", mc.ListGroupAvailableScenarios)
	group.POST("", mc.GroupCreateScenario)
	group.POST("/upload", mc.GroupUploadScenario)
	group.POST("/import-json", mc.GroupImportJSON)
	group.GET("/:scenarioId/export", mc.GroupExportScenario)
	group.POST("/:scenarioId/duplicate", mc.GroupDuplicateScenario)
	return r
}

func serve(router *gin.Engine, method, path string, body []byte, contentType string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func jsonBody(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	return raw
}

func seedScenarioJSON(t *testing.T, title string) []byte {
	return jsonBody(t, map[string]any{
		"title":         title,
		"instance_type": "ubuntu:22.04",
		"steps": []map[string]any{
			{"title": "Step 1", "text_content": "Do it", "verify_script": "#!/bin/bash\ntrue"},
		},
	})
}

// scenarioArchive is a one-step KillerCoda archive titled title, as the
// multipart body of an upload.
func scenarioArchive(t *testing.T, title string) ([]byte, string) {
	t.Helper()
	var zipped bytes.Buffer
	zw := zip.NewWriter(&zipped)
	files := map[string]string{
		"index.json": string(jsonBody(t, map[string]any{
			"title":   title,
			"details": map[string]any{"steps": []map[string]any{{"title": "Step 1", "text": "step1.md", "verify": "verify1.sh"}}},
		})),
		"step1.md":   "Do it",
		"verify1.sh": "#!/bin/bash\ntrue",
	}
	for name, content := range files {
		fw, err := zw.Create(name)
		require.NoError(t, err)
		_, err = fw.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", "lab.zip")
	require.NoError(t, err)
	_, err = fw.Write(zipped.Bytes())
	require.NoError(t, err)
	require.NoError(t, mw.Close())
	return body.Bytes(), mw.FormDataContentType()
}

func scenarioCreatedBy(t *testing.T, db *gorm.DB, orgID uuid.UUID, title, author string) bool {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&models.Scenario{}).
		Where("organization_id = ? AND title = ? AND created_by_id = ?", orgID, title, author).Count(&n).Error)
	return n == 1
}

// --- creating ---------------------------------------------------------------

func TestTeacherAuthoring_OrgCreate_TeacherCreatesStudentRefused(t *testing.T) {
	db := freshTestDB(t)
	f := buildTeacherAuthoringFixture(t, db)
	body := jsonBody(t, map[string]any{"name": "new-lab", "title": "New lab", "instance_type": "ubuntu:22.04"})

	w := serve(authoringRouter(t, db, "ta-teacher-a"), http.MethodPost, "/api/v1/organizations/"+f.org.String()+"/scenarios", body, "application/json")
	require.Equal(t, http.StatusCreated, w.Code, "body=%s", w.Body.String())
	assert.True(t, scenarioCreatedBy(t, db, f.org, "New lab", "ta-teacher-a"))

	w = serve(authoringRouter(t, db, "ta-student"), http.MethodPost, "/api/v1/organizations/"+f.org.String()+"/scenarios", body, "application/json")
	assert.Equal(t, http.StatusForbidden, w.Code, "body=%s", w.Body.String())
}

func TestTeacherAuthoring_GroupCreate_ClassOwnerCreatesStudentRefused(t *testing.T) {
	db := freshTestDB(t)
	f := buildTeacherAuthoringFixture(t, db)
	body := jsonBody(t, map[string]any{"name": "class-lab", "title": "Class lab", "instance_type": "ubuntu:22.04"})

	w := serve(authoringRouter(t, db, "ta-teacher-a"), http.MethodPost, "/api/v1/groups/"+f.class.String()+"/scenarios", body, "application/json")
	require.Equal(t, http.StatusCreated, w.Code, "body=%s", w.Body.String())
	assert.True(t, scenarioCreatedBy(t, db, f.org, "Class lab", "ta-teacher-a"))

	w = serve(authoringRouter(t, db, "ta-student"), http.MethodPost, "/api/v1/groups/"+f.class.String()+"/scenarios", body, "application/json")
	assert.Equal(t, http.StatusForbidden, w.Code, "body=%s", w.Body.String())
}

func TestTeacherAuthoring_OrgList_TeacherListsTheOrgCatalogue(t *testing.T) {
	db := freshTestDB(t)
	f := buildTeacherAuthoringFixture(t, db)

	w := serve(authoringRouter(t, db, "ta-teacher-a"), http.MethodGet, "/api/v1/organizations/"+f.org.String()+"/scenarios", nil, "")
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	var listed []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listed))
	assert.Len(t, listed, 2)

	w = serve(authoringRouter(t, db, "ta-student"), http.MethodGet, "/api/v1/organizations/"+f.org.String()+"/scenarios", nil, "")
	assert.Equal(t, http.StatusForbidden, w.Code)
}

// --- importing ----------------------------------------------------------------

func TestTeacherAuthoring_ImportJSON_TeacherImportsAtOrgAndClassLevel(t *testing.T) {
	db := freshTestDB(t)
	f := buildTeacherAuthoringFixture(t, db)
	router := authoringRouter(t, db, "ta-teacher-a")

	w := serve(router, http.MethodPost, "/api/v1/organizations/"+f.org.String()+"/scenarios/import-json", seedScenarioJSON(t, "Imported org lab"), "application/json")
	require.Equal(t, http.StatusCreated, w.Code, "body=%s", w.Body.String())
	assert.True(t, scenarioCreatedBy(t, db, f.org, "Imported org lab", "ta-teacher-a"))

	w = serve(router, http.MethodPost, "/api/v1/groups/"+f.class.String()+"/scenarios/import-json", seedScenarioJSON(t, "Imported class lab"), "application/json")
	require.Equal(t, http.StatusCreated, w.Code, "body=%s", w.Body.String())
	assert.True(t, scenarioCreatedBy(t, db, f.org, "Imported class lab", "ta-teacher-a"))
}

func TestTeacherAuthoring_Upload_TeacherUploadsAnArchive(t *testing.T) {
	db := freshTestDB(t)
	f := buildTeacherAuthoringFixture(t, db)
	body, contentType := scenarioArchive(t, "Uploaded lab")

	w := serve(authoringRouter(t, db, "ta-teacher-a"), http.MethodPost, "/api/v1/organizations/"+f.org.String()+"/scenarios/upload", body, contentType)
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.True(t, scenarioCreatedBy(t, db, f.org, "Uploaded lab", "ta-teacher-a"))
}

// Imports upsert by name inside the organisation: replacing a scenario that
// way is editing it, so a teacher cannot overwrite a colleague's lab with a
// file that bears its title.
func TestTeacherAuthoring_Import_CannotOverwriteAColleaguesScenario(t *testing.T) {
	db := freshTestDB(t)
	f := buildTeacherAuthoringFixture(t, db)
	require.NoError(t, db.Model(f.byTeacherB).Updates(map[string]any{"name": "colleague-lab", "title": "Colleague lab"}).Error)

	router := authoringRouter(t, db, "ta-teacher-a")
	w := serve(router, http.MethodPost, "/api/v1/organizations/"+f.org.String()+"/scenarios/import-json", seedScenarioJSON(t, "Colleague lab"), "application/json")
	assert.Equal(t, http.StatusForbidden, w.Code, "body=%s", w.Body.String())

	w = serve(router, http.MethodPost, "/api/v1/groups/"+f.class.String()+"/scenarios/import-json", seedScenarioJSON(t, "Colleague lab"), "application/json")
	assert.Equal(t, http.StatusForbidden, w.Code, "body=%s", w.Body.String())

	body, contentType := scenarioArchive(t, "Colleague lab")
	w = serve(router, http.MethodPost, "/api/v1/organizations/"+f.org.String()+"/scenarios/upload", body, contentType)
	assert.Equal(t, http.StatusForbidden, w.Code, "body=%s", w.Body.String())

	var kept models.Scenario
	require.NoError(t, db.Preload("Steps").First(&kept, "id = ?", f.byTeacherB.ID).Error)
	assert.Equal(t, "ta-teacher-b", kept.CreatedByID)
	require.Len(t, kept.Steps, 1)
	assert.Equal(t, "Do step 1", kept.Steps[0].TextContent, "the colleague's content must be untouched")

	// The author re-imports their own lab: an update.
	w = serve(authoringRouter(t, db, "ta-teacher-b"), http.MethodPost, "/api/v1/organizations/"+f.org.String()+"/scenarios/import-json", seedScenarioJSON(t, "Colleague lab"), "application/json")
	assert.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
}

// --- exporting and copying ------------------------------------------------------

// Exporting is running a lab, not editing it: a teacher takes away any lab of
// their school (they could copy it anyway); a student takes away none.
func TestTeacherAuthoring_Export_TeacherExportsOrgLabsStudentRefused(t *testing.T) {
	db := freshTestDB(t)
	f := buildTeacherAuthoringFixture(t, db)
	createScenarioAssignment(t, db, f.byTeacherB.ID, &f.class, nil, "group")
	orgExport := "/api/v1/organizations/" + f.org.String() + "/scenarios/" + f.byTeacherB.ID.String() + "/export"
	groupExport := "/api/v1/groups/" + f.class.String() + "/scenarios/" + f.byTeacherB.ID.String() + "/export"

	router := authoringRouter(t, db, "ta-teacher-a")
	w := serve(router, http.MethodGet, orgExport, nil, "")
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.Contains(t, w.Body.String(), f.byTeacherB.Title)
	w = serve(router, http.MethodGet, groupExport, nil, "")
	assert.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	w = serve(authoringRouter(t, db, "ta-student"), http.MethodGet, orgExport, nil, "")
	assert.Equal(t, http.StatusForbidden, w.Code, "body=%s", w.Body.String())
}

// A scenario of another organisation is not exported through this one's route,
// whatever the caller's rank here.
func TestTeacherAuthoring_OrgExport_OtherOrgsScenarioNotFound(t *testing.T) {
	db := freshTestDB(t)
	f := buildTeacherAuthoringFixture(t, db)
	otherOrg := createTestOrg(t, db, "other-owner")
	foreign := createTestScenarioForOrg(t, db, otherOrg, "foreign-lab")

	w := serve(authoringRouter(t, db, "ta-manager"), http.MethodGet, "/api/v1/organizations/"+f.org.String()+"/scenarios/"+foreign.ID.String()+"/export", nil, "")
	assert.Equal(t, http.StatusNotFound, w.Code, "body=%s", w.Body.String())
}

func TestTeacherAuthoring_Duplicate_TeacherCopiesAColleaguesLab(t *testing.T) {
	db := freshTestDB(t)
	f := buildTeacherAuthoringFixture(t, db)
	router := authoringRouter(t, db, "ta-teacher-a")

	w := serve(router, http.MethodPost, "/api/v1/organizations/"+f.org.String()+"/scenarios/"+f.byTeacherB.ID.String()+"/duplicate", nil, "")
	require.Equal(t, http.StatusCreated, w.Code, "body=%s", w.Body.String())
	w = serve(router, http.MethodPost, "/api/v1/groups/"+f.class.String()+"/scenarios/"+f.byTeacherB.ID.String()+"/duplicate", nil, "")
	require.Equal(t, http.StatusCreated, w.Code, "body=%s", w.Body.String())

	var copies int64
	require.NoError(t, db.Model(&models.Scenario{}).Where("organization_id = ? AND created_by_id = ? AND id NOT IN ?", f.org, "ta-teacher-a", []uuid.UUID{f.byTeacherA.ID}).Count(&copies).Error)
	assert.EqualValues(t, 2, copies, "each copy belongs to the teacher who made it")
}

// --- editing ----------------------------------------------------------------------

func TestTeacherAuthoring_Patch_OwnAllowedColleaguesRefusedManagerAllowed(t *testing.T) {
	db := freshTestDB(t)
	f := buildTeacherAuthoringFixture(t, db)
	hook := scenarioHooks.NewScenarioAuthorizationHook(db)
	patch := func(user string, s *models.Scenario) error {
		return hook.Execute(&hooks.HookContext{
			EntityName: "Scenario", HookType: hooks.BeforeUpdate, EntityID: s.ID,
			OldEntity: s, NewEntity: map[string]any{"title": "edited"},
			UserID: user, UserRoles: []string{"member"},
		})
	}

	assert.NoError(t, patch("ta-teacher-a", f.byTeacherA), "a teacher edits their own lab")
	assert.Error(t, patch("ta-teacher-a", f.byTeacherB), "a teacher does not edit a colleague's lab")
	assert.NoError(t, patch("ta-manager", f.byTeacherB), "an org manager edits any lab of the org")
}

// Owning a class is running it, not administering the organisation's labs.
func TestTeacherAuthoring_CanManageScenario_ClassManagerManagesOnlyTheirOwn(t *testing.T) {
	db := freshTestDB(t)
	f := buildTeacherAuthoringFixture(t, db)
	addGroupMember(t, db, f.class, "ta-co-teacher", groupModels.GroupMemberRoleManager)
	createScenarioAssignment(t, db, f.byTeacherB.ID, &f.class, nil, "group")

	for _, user := range []string{"ta-teacher-a", "ta-co-teacher"} {
		ok, err := scenarioHooks.CanManageScenario(db, nil, f.byTeacherB, user)
		require.NoError(t, err)
		assert.False(t, ok, "%s manages a class of the org, not the org's labs", user)
	}
}

// --- deleting ---------------------------------------------------------------------

func TestTeacherAuthoring_OrgDelete_OwnAllowedColleaguesRefused(t *testing.T) {
	db := freshTestDB(t)
	f := buildTeacherAuthoringFixture(t, db)
	router := authoringRouter(t, db, "ta-teacher-a")

	w := serve(router, http.MethodDelete, "/api/v1/organizations/"+f.org.String()+"/scenarios/"+f.byTeacherB.ID.String(), nil, "")
	assert.Equal(t, http.StatusForbidden, w.Code, "body=%s", w.Body.String())
	var n int64
	require.NoError(t, db.Model(&models.Scenario{}).Where("id = ?", f.byTeacherB.ID).Count(&n).Error)
	assert.EqualValues(t, 1, n, "the colleague's lab must survive")

	w = serve(router, http.MethodDelete, "/api/v1/organizations/"+f.org.String()+"/scenarios/"+f.byTeacherA.ID.String(), nil, "")
	assert.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	require.NoError(t, db.Model(&models.Scenario{}).Where("id = ?", f.byTeacherA.ID).Count(&n).Error)
	assert.EqualValues(t, 0, n)
}

func TestTeacherAuthoring_EntityDelete_OwnAllowedColleaguesRefused(t *testing.T) {
	db := freshTestDB(t)
	f := buildTeacherAuthoringFixture(t, db)
	hook := scenarioHooks.NewScenarioAuthorizationHook(db)
	del := func(user string, s *models.Scenario) error {
		return hook.Execute(&hooks.HookContext{
			EntityName: "Scenario", HookType: hooks.BeforeDelete, EntityID: s.ID,
			NewEntity: s, UserID: user, UserRoles: []string{"member"},
		})
	}
	assert.NoError(t, del("ta-teacher-a", f.byTeacherA))
	assert.Error(t, del("ta-teacher-a", f.byTeacherB))
}

// --- running ----------------------------------------------------------------------

func TestTeacherAuthoring_Assign_TeacherAssignsAColleaguesLabToTheirClass(t *testing.T) {
	db := freshTestDB(t)
	f := buildTeacherAuthoringFixture(t, db)
	hook := scenarioHooks.NewScenarioAssignmentAuthorizationHook(db)

	err := hook.Execute(&hooks.HookContext{
		EntityName: "ScenarioAssignment", HookType: hooks.BeforeCreate,
		NewEntity: &models.ScenarioAssignment{ScenarioID: f.byTeacherB.ID, GroupID: &f.class, Scope: "group"},
		UserID:    "ta-teacher-a", UserRoles: []string{"member"},
	})
	assert.NoError(t, err)
}

func TestTeacherAuthoring_Preview_TeacherPreviewsStudentRefused(t *testing.T) {
	db := freshTestDB(t)
	f := buildTeacherAuthoringFixture(t, db)

	for _, user := range []string{"ta-teacher-a", "ta-teacher-b"} {
		w := previewScenario(t, db, user, f.byTeacherB.ID, map[string]any{})
		assert.NotContains(t, w.Body.String(), "not authorized to preview",
			"%s teaches in the org and may preview its labs; body=%s", user, w.Body.String())
	}

	w := previewScenario(t, db, "ta-student", f.byTeacherB.ID, map[string]any{})
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "not authorized to preview")
}

// --- the can_manage verdict -------------------------------------------------------

// can_manage is CanManageScenario for the caller: the one rule behind PATCH,
// archive and delete. A teacher runs a colleague's lab (its steps come back)
// but may not retire it.
func TestTeacherAuthoring_CanManage_OwnTrueColleaguesFalse(t *testing.T) {
	db := freshTestDB(t)
	f := buildTeacherAuthoringFixture(t, db)

	type card struct {
		Title     string           `json:"title"`
		CanManage bool             `json:"can_manage"`
		Steps     []map[string]any `json:"steps"`
	}
	verdicts := func(t *testing.T, cards []card) map[string]bool {
		got := map[string]bool{}
		for _, c := range cards {
			got[c.Title] = c.CanManage
			assert.NotEmpty(t, c.Steps, "%s: a teacher of the org reads the full lab", c.Title)
		}
		return got
	}
	want := map[string]map[string]bool{
		"ta-teacher-a": {f.byTeacherA.Title: true, f.byTeacherB.Title: false},
		"ta-manager":   {f.byTeacherA.Title: true, f.byTeacherB.Title: true},
	}

	for user, expected := range want {
		t.Run("GET /scenarios as "+user, func(t *testing.T) {
			w := serve(setupScenarioReadAuthzTest(t, db, user, []string{"member"}, "/scenarios"), http.MethodGet, "/api/v1/scenarios", nil, "")
			require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
			var page struct {
				Data []card `json:"data"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
			assert.Equal(t, expected, verdicts(t, page.Data))
		})
		t.Run("GET /organizations/:id/scenarios as "+user, func(t *testing.T) {
			w := serve(authoringRouter(t, db, user), http.MethodGet, "/api/v1/organizations/"+f.org.String()+"/scenarios", nil, "")
			require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
			var cards []card
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &cards))
			assert.Equal(t, expected, verdicts(t, cards))
		})
	}
}
