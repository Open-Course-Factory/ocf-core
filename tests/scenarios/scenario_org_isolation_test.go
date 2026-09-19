package scenarios_test

// Organisation isolation for scenarios (decided 2026-09-19):
//
//   - An org scenario is visible, editable and assignable only inside its
//     organisation: org managers and the managers of that org's classes.
//     Nothing of org B ever reaches a user of org A — not even when an
//     assignment row says otherwise, and not through `is_public`, which is
//     refused on an org scenario.
//   - A platform scenario (no org) is managed by admins, its creator, and the
//     managers of the classes it is assigned to — the catalogue is shared on
//     purpose; a teacher who wants to stop depending on it copies it into
//     their organisation. When public, everybody sees it read-only and any
//     teacher may assign it. When private, only admins assign it.
//   - Assigning requires being allowed to see the scenario — a UUID is not
//     a permission.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/entityManagement/hooks"
	groupModels "soli/formations/src/groups/models"
	groupServices "soli/formations/src/groups/services"
	orgModels "soli/formations/src/organizations/models"
	scenarioHooks "soli/formations/src/scenarios/hooks"
	"soli/formations/src/scenarios/models"

	"gorm.io/gorm"
)

// Two organisations, one teacher in A who is NOT an org member (a class
// manager only), and every kind of scenario the rule has to sort.
type orgIsolationFixture struct {
	orgA, orgB           uuid.UUID
	classA               uuid.UUID
	teacherA             string
	orgAPrivate          *models.Scenario
	orgBPrivate          *models.Scenario
	orgBPublic           *models.Scenario // data anomaly: public flag on an org scenario
	orgBAssignedToClassA *models.Scenario // data anomaly: cross-org assignment row
	platformPublic       *models.Scenario
	platformPrivate      *models.Scenario
	platformAssignedToA  *models.Scenario // private platform scenario the admin assigned to class A
}

func buildOrgIsolationFixture(t *testing.T, db *gorm.DB) orgIsolationFixture {
	t.Helper()
	f := orgIsolationFixture{teacherA: "teacher-a"}
	f.orgA = createTestOrg(t, db, "org-a-owner")
	f.orgB = createTestOrg(t, db, "org-b-owner")
	f.classA = createTestGroupInOrg(t, db, f.orgA, "org-a-owner")
	addGroupMember(t, db, f.classA, f.teacherA, groupModels.GroupMemberRoleManager)

	f.orgAPrivate = createTestScenarioForOrg(t, db, f.orgA, "org-a-private")
	f.orgBPrivate = createTestScenarioForOrg(t, db, f.orgB, "org-b-private")
	f.orgBPublic = createTestScenarioForOrg(t, db, f.orgB, "org-b-public")
	markScenarioPublic(t, db, f.orgBPublic.ID)
	f.orgBAssignedToClassA = createTestScenarioForOrg(t, db, f.orgB, "org-b-assigned-to-class-a")
	createScenarioAssignment(t, db, f.orgBAssignedToClassA.ID, &f.classA, nil, "group")
	f.platformPublic = createTestScenarioNoOrg(t, db, "platform-public")
	markScenarioPublic(t, db, f.platformPublic.ID)
	f.platformPrivate = createTestScenarioNoOrg(t, db, "platform-private")
	f.platformAssignedToA = createTestScenarioNoOrg(t, db, "platform-assigned-to-class-a")
	createScenarioAssignment(t, db, f.platformAssignedToA.ID, &f.classA, nil, "group")
	return f
}

// --- what a teacher lists in the editor ------------------------------------

func TestOrgIsolation_TeacherListsOwnOrgAndPlatformPublicOnly(t *testing.T) {
	db := freshTestDB(t)
	f := buildOrgIsolationFixture(t, db)

	names := listScenarioNames(t, db, f.teacherA, []string{"member"}, "")
	require.ElementsMatch(t, []string{"org-a-private", "platform-public", "platform-assigned-to-class-a"}, names)
}

func TestOrgIsolation_OrgManagerListsOwnOrgAndPlatformPublicOnly(t *testing.T) {
	db := freshTestDB(t)
	f := buildOrgIsolationFixture(t, db)
	addOrgMember(t, db, f.orgA, "org-a-manager", orgModels.OrgRoleManager)

	// The org manager manages class A too, hence the platform scenario assigned to it.
	names := listScenarioNames(t, db, "org-a-manager", []string{"member"}, "")
	require.ElementsMatch(t, []string{"org-a-private", "platform-public", "platform-assigned-to-class-a"}, names)
}

// --- the manage predicate itself -------------------------------------------

func TestOrgIsolation_CanManageScenario(t *testing.T) {
	db := freshTestDB(t)
	f := buildOrgIsolationFixture(t, db)
	groupSvc := groupServices.NewGroupService(db)

	cases := []struct {
		name     string
		user     string
		scenario *models.Scenario
		want     bool
	}{
		{"teacher manages own org scenario", f.teacherA, f.orgAPrivate, true},
		{"teacher does not manage other org scenario", f.teacherA, f.orgBPrivate, false},
		{"cross-org assignment grants nothing", f.teacherA, f.orgBAssignedToClassA, false},
		{"public platform scenario is read-only", f.teacherA, f.platformPublic, false},
		{"platform scenario assigned to my class is mine to edit", f.teacherA, f.platformAssignedToA, true},
		{"platform scenario assigned elsewhere is not", "org-b-owner", f.platformAssignedToA, false},
		{"creator manages their scenario", "test-creator", f.orgBPrivate, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := scenarioHooks.CanManageScenario(db, groupSvc, c.scenario, c.user)
			require.NoError(t, err)
			assert.Equal(t, c.want, got)
		})
	}
}

// --- public is a platform notion -------------------------------------------

func TestOrgIsolation_PublicFlagIsRefusedOnOrgScenario(t *testing.T) {
	db := freshTestDB(t)
	f := buildOrgIsolationFixture(t, db)
	hook := scenarioHooks.NewScenarioAuthorizationHook(db)

	t.Run("update", func(t *testing.T) {
		err := hook.Execute(&hooks.HookContext{
			EntityName: "Scenario", HookType: hooks.BeforeUpdate, EntityID: f.orgAPrivate.ID,
			OldEntity: f.orgAPrivate, NewEntity: map[string]any{"is_public": true},
			UserID: "org-a-owner", UserRoles: []string{"member"},
		})
		require.Error(t, err, "an org scenario cannot be made public")
	})
	t.Run("update on platform scenario", func(t *testing.T) {
		err := hook.Execute(&hooks.HookContext{
			EntityName: "Scenario", HookType: hooks.BeforeUpdate, EntityID: f.platformPrivate.ID,
			OldEntity: f.platformPrivate, NewEntity: map[string]any{"is_public": true},
			UserID: "test-creator", UserRoles: []string{"member"},
		})
		require.NoError(t, err)
	})
	t.Run("create", func(t *testing.T) {
		orgID := f.orgA
		err := hook.Execute(&hooks.HookContext{
			EntityName: "Scenario", HookType: hooks.BeforeCreate,
			NewEntity: &models.Scenario{Name: "x", OrganizationID: &orgID, IsPublic: true},
			UserID:    "org-a-owner", UserRoles: []string{"administrator"},
		})
		require.Error(t, err, "not even an admin: the flag has no meaning on an org scenario")
	})
}

func TestOrgIsolation_LearnerCatalogueIgnoresOrgPublicFlag(t *testing.T) {
	db := freshTestDB(t)
	f := buildOrgIsolationFixture(t, db)

	var names []string
	require.NoError(t, db.Model(&models.Scenario{}).Scopes(models.PublicCatalogue).Pluck("name", &names).Error)
	require.ElementsMatch(t, []string{"platform-public"}, names, "org-b-public must not be in the public catalogue")
	_ = f
}

// --- assigning requires seeing --------------------------------------------

func TestOrgIsolation_AssignmentCreateRequiresVisibleScenario(t *testing.T) {
	db := freshTestDB(t)
	f := buildOrgIsolationFixture(t, db)
	hook := scenarioHooks.NewScenarioAssignmentAuthorizationHook(db)

	assign := func(user string, roles []string, scenario *models.Scenario) error {
		return hook.Execute(&hooks.HookContext{
			EntityName: "ScenarioAssignment", HookType: hooks.BeforeCreate,
			NewEntity: &models.ScenarioAssignment{ScenarioID: scenario.ID, GroupID: &f.classA, Scope: "group"},
			UserID:    user, UserRoles: roles,
		})
	}
	member := []string{"member"}
	require.NoError(t, assign(f.teacherA, member, f.orgAPrivate), "own org scenario")
	require.NoError(t, assign(f.teacherA, member, f.platformPublic), "public platform scenario")
	require.Error(t, assign(f.teacherA, member, f.orgBPrivate), "another org's scenario")
	require.Error(t, assign(f.teacherA, member, f.orgBPublic), "another org's scenario, whatever its flag says")
	require.Error(t, assign(f.teacherA, member, f.platformPrivate), "private platform scenario")
	require.NoError(t, assign("ops", []string{"administrator"}, f.platformPrivate), "admins curate the platform catalogue")
	require.Error(t, assign("ops", []string{"administrator"}, f.orgBPrivate), "an org scenario never leaves its org, even by admin hand")
}

// --- copying into a class -------------------------------------------------

func TestOrgIsolation_TeacherCopiesPublicScenarioIntoTheirClass(t *testing.T) {
	db := freshTestDB(t)
	f := buildOrgIsolationFixture(t, db)
	router := setupOrgTestRouterWithUserAndRoles(t, db, f.teacherA, []string{"member"})

	post := func(scenarioID uuid.UUID) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/groups/"+f.classA.String()+"/scenarios/"+scenarioID.String()+"/duplicate", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	w := post(f.platformPublic.ID)
	require.Equal(t, http.StatusCreated, w.Code, "body: %s", w.Body.String())
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, f.orgA.String(), resp["organization_id"], "the copy belongs to the class's organisation")
	assert.Equal(t, false, resp["is_public"], "a copy into an organisation is that organisation's, not the catalogue's")
	var assignments int64
	require.NoError(t, db.Model(&models.ScenarioAssignment{}).Where("scenario_id = ? AND group_id = ?", resp["id"], f.classA).Count(&assignments).Error)
	assert.EqualValues(t, 1, assignments, "the copy is assigned to the class it was copied for")

	assert.Equal(t, http.StatusCreated, post(f.orgAPrivate.ID).Code, "own org scenarios can be copied too")
	assert.Equal(t, http.StatusNotFound, post(f.orgBPrivate.ID).Code, "another org's scenario is not copyable")
	assert.Equal(t, http.StatusNotFound, post(f.orgBPublic.ID).Code)
	assert.Equal(t, http.StatusNotFound, post(f.platformPrivate.ID).Code)

	stranger := setupOrgTestRouterWithUserAndRoles(t, db, "stranger", []string{"member"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/groups/"+f.classA.String()+"/scenarios/"+f.platformPublic.ID.String()+"/duplicate", nil)
	w = httptest.NewRecorder()
	stranger.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code, "only the class's managers copy into it")
}
