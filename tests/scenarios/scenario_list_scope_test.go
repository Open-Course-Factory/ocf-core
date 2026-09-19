package scenarios_test

// Issue #294: GET /api/v1/scenarios listed every scenario on the platform to
// every authenticated Member. The redactor stripped step content per row but
// never the row itself, so an org manager (or a student typing the editor URL)
// saw every other organisation's scenario titles, descriptions and objectives.
//
// Contract: a non-admin lists exactly the scenarios they can manage
// (scenarioHooks.CanManageScenario — the same predicate the write hooks use)
// plus public ones. Admins keep the platform-wide list.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	groupModels "soli/formations/src/groups/models"
	orgModels "soli/formations/src/organizations/models"
	"soli/formations/src/scenarios/models"

	"gorm.io/gorm"
)

func listScenarioNames(t *testing.T, db *gorm.DB, userID string, roles []string, query string) []string {
	t.Helper()
	router := setupScenarioReadAuthzTest(t, db, userID, roles, "/scenarios")
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/scenarios"+query, nil)
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	var page struct {
		Data  []struct{ Name string `json:"name"` } `json:"data"`
		Total int64                                  `json:"total"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
	names := make([]string, 0, len(page.Data))
	for _, s := range page.Data {
		names = append(names, s.Name)
	}
	require.EqualValues(t, len(names), page.Total, "total must count only the scoped rows")
	return names
}

func makeScenario(t *testing.T, db *gorm.DB, name, creatorID string, orgID *uuid.UUID, public bool) *models.Scenario {
	t.Helper()
	s := &models.Scenario{Name: name, Title: name, InstanceType: "s", SourceType: "seed",
		CreatedByID: creatorID, OrganizationID: orgID, IsPublic: public}
	require.NoError(t, db.Create(s).Error)
	return s
}

// One fixture, several callers: what each of them is allowed to list.
type listScopeFixture struct {
	orgA, orgB     uuid.UUID
	groupG         uuid.UUID
	orgAManager    string
	groupGManager  string
	orgAPlainMember string
}

func buildListScopeFixture(t *testing.T, db *gorm.DB) listScopeFixture {
	t.Helper()
	f := listScopeFixture{
		orgAManager:     "org-a-manager",
		groupGManager:   "group-g-manager",
		orgAPlainMember: "org-a-plain-member",
	}
	f.orgA = makeOrgWithMember(t, db, "org-a-owner", f.orgAManager, orgModels.OrgRoleManager)
	require.NoError(t, db.Omit("Metadata").Create(&orgModels.OrganizationMember{
		OrganizationID: f.orgA, UserID: f.orgAPlainMember, Role: orgModels.OrgRoleMember,
		JoinedAt: time.Now(), IsActive: true,
	}).Error)
	f.orgB = makeOrgWithMember(t, db, "org-b-owner", "", orgModels.OrgRoleMember)
	// A class of org A whose manager is not an org member: a teacher.
	f.groupG = makeGroupWithOwner(t, db, f.groupGManager)
	require.NoError(t, db.Model(&groupModels.ClassGroup{}).Where("id = ?", f.groupG).Update("organization_id", f.orgA).Error)

	makeScenario(t, db, "org-a-private", "org-a-owner", &f.orgA, false)
	makeScenario(t, db, "org-b-private", "org-b-owner", &f.orgB, false)
	makeScenario(t, db, "platform-private", "platform-admin", nil, false)
	makeScenario(t, db, "platform-public", "platform-admin", nil, true)
	// Two data anomalies the rule must ignore: a public flag on an org
	// scenario, and an assignment of an org-B scenario to a class of org A.
	makeScenario(t, db, "org-b-public", "org-b-owner", &f.orgB, true)
	assigned := makeScenario(t, db, "org-b-assigned-to-g", "org-b-owner", &f.orgB, false)
	require.NoError(t, db.Create(&models.ScenarioAssignment{
		ScenarioID: assigned.ID, Scope: "group", GroupID: &f.groupG, IsActive: true,
	}).Error)
	return f
}

func TestListScenarios_OrgManager_SeesOwnOrgAndPublicOnly(t *testing.T) {
	db := freshTestDB(t)
	f := buildListScopeFixture(t, db)

	names := listScenarioNames(t, db, f.orgAManager, []string{"member"}, "")
	require.ElementsMatch(t, []string{"org-a-private", "platform-public"}, names)
}

func TestListScenarios_ClassManager_SeesOwnOrgAndPublicOnly(t *testing.T) {
	db := freshTestDB(t)
	f := buildListScopeFixture(t, db)

	names := listScenarioNames(t, db, f.groupGManager, []string{"member"}, "")
	require.ElementsMatch(t, []string{"org-a-private", "platform-public"}, names)
}

func TestListScenarios_PlainOrgMember_SeesPublicOnly(t *testing.T) {
	db := freshTestDB(t)
	f := buildListScopeFixture(t, db)

	names := listScenarioNames(t, db, f.orgAPlainMember, []string{"member"}, "")
	require.ElementsMatch(t, []string{"platform-public"}, names)
}

func TestListScenarios_Creator_SeesOwnPrivateScenario(t *testing.T) {
	db := freshTestDB(t)
	buildListScopeFixture(t, db)

	names := listScenarioNames(t, db, "platform-admin", []string{"member"}, "")
	require.ElementsMatch(t, []string{"platform-private", "platform-public"}, names)
}

func TestListScenarios_Admin_SeesEverything(t *testing.T) {
	db := freshTestDB(t)
	buildListScopeFixture(t, db)

	names := listScenarioNames(t, db, "someone", []string{"administrator"}, "")
	require.Len(t, names, 6)
}

// The cursor branch shares the filter map with the offset branch; pin it so a
// refactor of one cannot silently unscope the other.
func TestListScenarios_CursorPagination_IsScopedToo(t *testing.T) {
	db := freshTestDB(t)
	f := buildListScopeFixture(t, db)

	router := setupScenarioReadAuthzTest(t, db, f.orgAPlainMember, []string{"member"}, "/scenarios")
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/scenarios?cursor=&limit=10", nil)
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	var page struct {
		Data  []struct{ Name string `json:"name"` } `json:"data"`
		Total int64                                  `json:"total"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
	require.Len(t, page.Data, 1)
	require.EqualValues(t, 1, page.Total)
}

// A member with nothing to manage and no public scenario around gets an empty
// page, not the whole table — the failure mode this issue is about.
func TestListScenarios_NothingManageable_EmptyPage(t *testing.T) {
	db := freshTestDB(t)
	makeScenario(t, db, "org-x-private", "org-x-owner", nil, false)

	names := listScenarioNames(t, db, "stranger", []string{"member"}, "")
	require.Empty(t, names)
}


// can_manage is the backend verdict the editor relies on for its edit
// controls: true exactly where a write would pass the hooks.
func TestListScenarios_CanManage_ReflectsTheWriteVerdict(t *testing.T) {
	db := freshTestDB(t)
	f := buildListScopeFixture(t, db)
	router := setupScenarioReadAuthzTest(t, db, f.orgAManager, []string{"member"}, "/scenarios")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/scenarios", nil)
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var page struct {
		Data []struct {
			Name      string `json:"name"`
			CanManage bool   `json:"can_manage"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
	verdicts := map[string]bool{}
	for _, s := range page.Data {
		verdicts[s.Name] = s.CanManage
	}
	require.Equal(t, map[string]bool{"org-a-private": true, "platform-public": false}, verdicts)
}
