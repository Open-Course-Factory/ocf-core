// tests/scenarios/permissionDeniedMapsTo403_test.go
//
// #493 / #483: the authorization hooks refused with a plain
// utils.PermissionDeniedError. The generic PATCH / DELETE / POST path did not
// recognise that shape, wrapped it as ENT007 and answered 500, while the
// archive hook's structured entityErrors.NewUnauthorizedError answered 403.
// Same rule, two error shapes. PermissionDeniedError is now the structured
// 403 itself, so every hook and service that refuses through it agrees with
// the framework.
package scenarios_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	orgModels "soli/formations/src/organizations/models"
	"soli/formations/src/scenarios/models"
)

func TestScenarioPatch_RefusedForSomeoneWhoCannotManageIt_Answers403(t *testing.T) {
	db := freshTestDB(t)
	orgID := createTestOrg(t, db, "org-owner")
	addOrgMember(t, db, orgID, "plain-member", orgModels.OrgRoleMember)
	scenario := createTestScenarioForOrg(t, db, orgID, "patch-authz-493")

	router := setupArchiveRouter(t, db, "plain-member", []string{"member"})

	body, _ := json.Marshal(map[string]any{"title": "renamed by a stranger"})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/scenarios/"+scenario.ID.String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code, "a refusal is the client's fault, never a hook failure. Body: %s", w.Body.String())

	var stored models.Scenario
	require.NoError(t, db.First(&stored, "id = ?", scenario.ID).Error)
	assert.Equal(t, scenario.Title, stored.Title, "the refused patch must not be written")
}
