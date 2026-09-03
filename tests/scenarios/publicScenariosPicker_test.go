// tests/scenarios/publicScenariosPicker_test.go
//
// The seeded catalogue (GameShell and friends) is public: is_public = true and
// no owning organisation. A teacher assigning work to a class must be able to
// pick it, so the group picker offers it under a third source, "public", next
// to the organisation's own scenarios ("org") and the ones already assigned to
// the group ("group"). The same public/not-archived rule lets an organisation
// copy a public scenario into its own catalogue.
package scenarios_test

import (
	"encoding/json"
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
)

func markScenarioPublic(t *testing.T, db *gorm.DB, scenarioID uuid.UUID) {
	t.Helper()
	require.NoError(t, db.Model(&models.Scenario{}).
		Where("id = ?", scenarioID).Update("is_public", true).Error)
}

func archiveScenarioRow(t *testing.T, db *gorm.DB, scenarioID uuid.UUID) {
	t.Helper()
	require.NoError(t, db.Model(&models.Scenario{}).
		Where("id = ?", scenarioID).Update("archived_at", time.Now()).Error)
}

// pickerSourcesByName returns the picker's answer for a group as name → source.
func pickerSourcesByName(t *testing.T, router *gin.Engine, groupID uuid.UUID) map[string]string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/groups/"+groupID.String()+"/scenarios", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var items []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &items))
	sources := make(map[string]string, len(items))
	for _, item := range items {
		name := item["name"].(string)
		_, seen := sources[name]
		require.False(t, seen, "scenario %q must appear once in the picker", name)
		sources[name] = item["source"].(string)
	}
	return sources
}

// pickerFixture is an org manager owning one class in that org.
func pickerFixture(t *testing.T, db *gorm.DB) (orgID, groupID uuid.UUID, router *gin.Engine) {
	t.Helper()
	orgID = createTestOrg(t, db, "org-owner")
	addOrgMember(t, db, orgID, "org-manager", orgModels.OrgRoleManager)
	groupID = createTestGroupInOrg(t, db, orgID, "org-manager")
	addGroupMember(t, db, groupID, "org-manager", groupModels.GroupMemberRoleOwner)
	router = setupOrgTestRouterWithUserAndRoles(t, db, "org-manager", []string{"member"})
	return orgID, groupID, router
}

func TestGroupPicker_OffersPublicScenariosUnderThePublicSource(t *testing.T) {
	db := freshTestDB(t)
	_, groupID, router := pickerFixture(t, db)

	public := createTestScenarioNoOrg(t, db, "catalogue-public")
	markScenarioPublic(t, db, public.ID)

	sources := pickerSourcesByName(t, router, groupID)
	assert.Equal(t, "public", sources["catalogue-public"],
		"a public scenario nobody owns and nobody assigned is offered as \"public\"")
}

func TestGroupPicker_PublicScenarioOwnedByTheOrgKeepsTheOrgSource(t *testing.T) {
	db := freshTestDB(t)
	orgID, groupID, router := pickerFixture(t, db)

	owned := createTestScenarioForOrg(t, db, orgID, "org-and-public")
	markScenarioPublic(t, db, owned.ID)

	sources := pickerSourcesByName(t, router, groupID)
	assert.Equal(t, "org", sources["org-and-public"],
		"the org section keeps precedence over the public one")
}

func TestGroupPicker_PublicScenarioAlreadyAssignedKeepsTheGroupSource(t *testing.T) {
	db := freshTestDB(t)
	_, groupID, router := pickerFixture(t, db)

	assigned := createTestScenarioNoOrg(t, db, "assigned-and-public")
	markScenarioPublic(t, db, assigned.ID)
	createScenarioAssignment(t, db, assigned.ID, &groupID, nil, "group")

	sources := pickerSourcesByName(t, router, groupID)
	assert.Equal(t, "group", sources["assigned-and-public"],
		"an existing assignment keeps precedence over the public section")
}

func TestGroupPicker_ArchivedPublicScenarioIsNotOffered(t *testing.T) {
	db := freshTestDB(t)
	_, groupID, router := pickerFixture(t, db)

	retired := createTestScenarioNoOrg(t, db, "public-but-archived")
	markScenarioPublic(t, db, retired.ID)
	archiveScenarioRow(t, db, retired.ID)

	sources := pickerSourcesByName(t, router, groupID)
	assert.NotContains(t, sources, "public-but-archived")
}

func TestGroupPicker_PrivateScenarioOfAnotherOrgStaysHidden(t *testing.T) {
	db := freshTestDB(t)
	_, groupID, router := pickerFixture(t, db)

	otherOrgID := createTestOrg(t, db, "other-org-owner")
	createTestScenarioForOrg(t, db, otherOrgID, "foreign-private")
	createTestScenarioNoOrg(t, db, "orphan-private")

	sources := pickerSourcesByName(t, router, groupID)
	assert.NotContains(t, sources, "foreign-private")
	assert.NotContains(t, sources, "orphan-private")
}

