// tests/scenarios/scenarioStaysInItsOrg_test.go
//
// #520: a scenario never leaves its organisation. The generic PATCH mapped
// organization_id straight into the update, and the authorization hook only
// refused it when combined with is_public — so the scenario's creator or a
// manager of its org could move it (and its files) into any other
// organisation, or turn it into a platform scenario. Only administrators may
// change organization_id; the same value, or no value, is a normal edit.
//
// Other write paths checked, none of which rewrites organization_id on an
// existing row: org/group create and duplicate always build a NEW row whose
// org comes from the route; the JSON and archive imports upsert by name
// scoped to the route's org and never write organization_id on update; step,
// hint and question edit DTOs carry no parent id.
package scenarios_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	orgModels "soli/formations/src/organizations/models"
	"soli/formations/src/scenarios/models"
)

func patchScenarioJSON(t *testing.T, router *gin.Engine, scenarioID uuid.UUID, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/scenarios/"+scenarioID.String(), bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func storedScenario(t *testing.T, db *gorm.DB, id uuid.UUID) models.Scenario {
	t.Helper()
	var s models.Scenario
	require.NoError(t, db.First(&s, "id = ?", id).Error)
	return s
}

func assertStillInOrg(t *testing.T, db *gorm.DB, id, orgID uuid.UUID) {
	t.Helper()
	s := storedScenario(t, db, id)
	require.NotNil(t, s.OrganizationID, "the scenario must not have become a platform scenario")
	assert.Equal(t, orgID, *s.OrganizationID, "the scenario must still belong to its organisation")
}

func orgIDBody(t *testing.T, fields map[string]any) string {
	t.Helper()
	b, err := json.Marshal(fields)
	require.NoError(t, err)
	return string(b)
}

func TestScenarioPatch_OrgManagerCannotMoveScenarioToAnotherOrg(t *testing.T) {
	db := freshTestDB(t)
	orgA := createTestOrg(t, db, "org-a-owner")
	orgB := createTestOrg(t, db, "org-b-owner")
	addOrgMember(t, db, orgA, "org-a-manager", orgModels.OrgRoleManager)
	// Managing both ends must not help: the rule is "never leaves", not "only to an org you manage".
	addOrgMember(t, db, orgB, "org-a-manager", orgModels.OrgRoleManager)
	scenario := createTestScenarioForOrg(t, db, orgA, "move-by-manager")

	router := setupArchiveRouter(t, db, "org-a-manager", []string{"member"})

	w := patchScenarioJSON(t, router, scenario.ID, orgIDBody(t, map[string]any{"organization_id": orgB}))
	assert.Equal(t, http.StatusForbidden, w.Code, "a scenario never leaves its organisation. Body: %s", w.Body.String())
	assertStillInOrg(t, db, scenario.ID, orgA)
}

func TestScenarioPatch_CreatorCannotMoveScenarioToAnotherOrg(t *testing.T) {
	db := freshTestDB(t)
	orgA := createTestOrg(t, db, "org-a-owner")
	orgB := createTestOrg(t, db, "org-b-owner")
	// createTestScenarioForOrg stamps CreatedByID "test-creator", who is in neither org.
	scenario := createTestScenarioForOrg(t, db, orgA, "move-by-creator")

	router := setupArchiveRouter(t, db, "test-creator", []string{"member"})

	w := patchScenarioJSON(t, router, scenario.ID, orgIDBody(t, map[string]any{"organization_id": orgB}))
	assert.Equal(t, http.StatusForbidden, w.Code, "being the creator does not let a scenario leave its organisation. Body: %s", w.Body.String())
	assertStillInOrg(t, db, scenario.ID, orgA)
}

func TestScenarioPatch_NonAdminCannotTurnOrgScenarioIntoPlatformScenario(t *testing.T) {
	db := freshTestDB(t)
	orgA := createTestOrg(t, db, "org-a-owner")
	addOrgMember(t, db, orgA, "org-a-manager", orgModels.OrgRoleManager)

	t.Run("nil UUID", func(t *testing.T) {
		scenario := createTestScenarioForOrg(t, db, orgA, "unorg-nil-uuid")
		router := setupArchiveRouter(t, db, "org-a-manager", []string{"member"})

		w := patchScenarioJSON(t, router, scenario.ID, orgIDBody(t, map[string]any{"organization_id": uuid.Nil}))
		assert.Equal(t, http.StatusForbidden, w.Code, "an org scenario cannot be made a platform one by a non-admin. Body: %s", w.Body.String())
		assertStillInOrg(t, db, scenario.ID, orgA)
	})

	t.Run("null", func(t *testing.T) {
		scenario := createTestScenarioForOrg(t, db, orgA, "unorg-null")
		router := setupArchiveRouter(t, db, "org-a-manager", []string{"member"})

		// JSON null decodes to an absent field in the edit DTO: whatever the
		// status, the scenario must stay in its organisation.
		w := patchScenarioJSON(t, router, scenario.ID, `{"organization_id": null}`)
		assert.Contains(t, []int{http.StatusNoContent, http.StatusForbidden}, w.Code, "body: %s", w.Body.String())
		assertStillInOrg(t, db, scenario.ID, orgA)
	})
}

func TestScenarioPatch_SameOrgOrNoOrgIsANormalEdit(t *testing.T) {
	db := freshTestDB(t)
	orgA := createTestOrg(t, db, "org-a-owner")
	addOrgMember(t, db, orgA, "org-a-manager", orgModels.OrgRoleManager)

	t.Run("unchanged organization_id", func(t *testing.T) {
		scenario := createTestScenarioForOrg(t, db, orgA, "same-org")
		router := setupArchiveRouter(t, db, "org-a-manager", []string{"member"})

		w := patchScenarioJSON(t, router, scenario.ID, orgIDBody(t, map[string]any{"organization_id": orgA, "title": "renamed"}))
		require.Equal(t, http.StatusNoContent, w.Code, "the front sends the org back unchanged on every save. Body: %s", w.Body.String())
		s := storedScenario(t, db, scenario.ID)
		assert.Equal(t, "renamed", s.Title)
		assertStillInOrg(t, db, scenario.ID, orgA)
	})

	t.Run("no organization_id", func(t *testing.T) {
		scenario := createTestScenarioForOrg(t, db, orgA, "no-org-field")
		router := setupArchiveRouter(t, db, "org-a-manager", []string{"member"})

		w := patchScenarioJSON(t, router, scenario.ID, `{"title": "renamed"}`)
		require.Equal(t, http.StatusNoContent, w.Code, "body: %s", w.Body.String())
		assert.Equal(t, "renamed", storedScenario(t, db, scenario.ID).Title)
		assertStillInOrg(t, db, scenario.ID, orgA)
	})
}

func TestScenarioPatch_AdministratorCanMoveScenarioBetweenOrgs(t *testing.T) {
	db := freshTestDB(t)
	orgA := createTestOrg(t, db, "org-a-owner")
	orgB := createTestOrg(t, db, "org-b-owner")
	scenario := createTestScenarioForOrg(t, db, orgA, "move-by-admin")

	router := setupArchiveRouter(t, db, "platform-admin", []string{"administrator"})

	w := patchScenarioJSON(t, router, scenario.ID, orgIDBody(t, map[string]any{"organization_id": orgB}))
	require.Equal(t, http.StatusNoContent, w.Code, "administrators bypass, as in every other scenario hook. Body: %s", w.Body.String())
	assertStillInOrg(t, db, scenario.ID, orgB)
}

func TestScenarioPatch_PublicFlagStillRefusedOnOrgScenario(t *testing.T) {
	db := freshTestDB(t)
	orgA := createTestOrg(t, db, "org-a-owner")
	addOrgMember(t, db, orgA, "org-a-manager", orgModels.OrgRoleManager)
	scenario := createTestScenarioForOrg(t, db, orgA, "public-org")

	router := setupArchiveRouter(t, db, "org-a-manager", []string{"member"})

	w := patchScenarioJSON(t, router, scenario.ID, orgIDBody(t, map[string]any{"is_public": true, "organization_id": orgA}))
	assert.GreaterOrEqual(t, w.Code, 400, "an org scenario cannot be made public. Body: %s", w.Body.String())
	s := storedScenario(t, db, scenario.ID)
	assert.False(t, s.IsPublic)
	assertStillInOrg(t, db, scenario.ID, orgA)
}
