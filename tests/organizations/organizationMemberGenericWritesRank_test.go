// tests/organizations/organizationMemberGenericWritesRank_test.go
//
// The generic organization-member PATCH and DELETE follow the rank rule of
// offboarding and reinstating: a caller acts only on a member they outrank (the
// owner on anyone but a platform administrator, a manager on plain members).
// is_active belongs to offboard / reinstate, and an offboarded row belongs to
// its offboarding: reinstate or erase are the ways out, not a generic delete.
package organizations_tests

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	ems "soli/formations/src/entityManagement/entityManagementService"
	"soli/formations/src/entityManagement/hooks"
	controller "soli/formations/src/entityManagement/routes"
	orgRegistration "soli/formations/src/organizations/entityRegistration"
	"soli/formations/src/organizations/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/mitchellh/mapstructure"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// seedGenericWritesOrg seeds owner-1 (owner), manager-1 and manager-2
// (managers), student-1 (member) and admin-1 (a member who is a platform
// administrator), with the real organization hooks registered.
func seedGenericWritesOrg(t *testing.T) (*gorm.DB, uuid.UUID) {
	t.Helper()
	installMockEnforcer(t).GetRolesForUserFunc = adminRoleFor("admin-1")
	db := newOrgRoleCapDB(t)
	orgID := seedOrgWithMembers(t, db, "owner-1", map[string]models.OrganizationMemberRole{
		"owner-1":   models.OrgRoleOwner,
		"manager-1": models.OrgRoleManager,
		"manager-2": models.OrgRoleManager,
		"student-1": models.OrgRoleMember,
		"admin-1":   models.OrgRoleMember,
	})
	registerOrgHooks(t, db)
	return db, orgID
}

func markOffboarded(t *testing.T, db *gorm.DB, orgID uuid.UUID, userID string) {
	t.Helper()
	now := time.Now()
	require.NoError(t, db.Model(&models.OrganizationMember{}).
		Where("organization_id = ? AND user_id = ?", orgID, userID).
		Updates(map[string]any{"is_active": false, "left_at": now, "scheduled_erasure_at": now.AddDate(0, 0, 10)}).Error)
}

// genericPatch runs body through the generic PATCH path: decoded and mapped by
// the real registration, authorized by the real BeforeUpdate hooks against the
// stored row, then written when authorized. Returns the hook error.
func genericPatch(t *testing.T, db *gorm.DB, orgID uuid.UUID, actor, target string, body map[string]any) error {
	t.Helper()
	svc := ems.NewEntityRegistrationService()
	orgRegistration.RegisterOrganizationMember(svc)
	ops, ok := svc.GetEntityOps("OrganizationMember")
	require.True(t, ok)
	edit := ops.NewEditDto()
	require.NoError(t, mapstructure.Decode(body, &edit))
	updates, err := ops.ConvertEditDtoToMap(edit)
	require.NoError(t, err)

	current := loadMember(t, db, orgID, target)
	err = hooks.GlobalHookRegistry.ExecuteHooks(&hooks.HookContext{
		EntityName: "OrganizationMember",
		HookType:   hooks.BeforeUpdate,
		EntityID:   current.ID,
		OldEntity:  &current,
		NewEntity:  updates,
		UserID:     actor,
		UserRoles:  platformMember,
	})
	delete(updates, "metadata") // SQLite cannot bind the jsonb map; no test reads it back
	if err == nil && len(updates) > 0 {
		require.NoError(t, db.Model(&models.OrganizationMember{}).Where("id = ?", current.ID).Updates(updates).Error)
	}
	return err
}

// genericDelete runs the real BeforeDelete hooks on the stored row, as the
// generic DELETE does. Returns the hook error.
func genericDelete(t *testing.T, db *gorm.DB, orgID uuid.UUID, actor string, actorRoles []string, target string) error {
	t.Helper()
	current := loadMember(t, db, orgID, target)
	return hooks.GlobalHookRegistry.ExecuteHooks(&hooks.HookContext{
		EntityName: "OrganizationMember",
		HookType:   hooks.BeforeDelete,
		EntityID:   current.ID,
		NewEntity:  &current,
		UserID:     actor,
		UserRoles:  actorRoles,
	})
}

func TestOrgMemberGenericPatch_IsActive_NeitherReactivatesNorDeactivates(t *testing.T) {
	for _, actor := range []string{"manager-1", "owner-1"} {
		t.Run(actor, func(t *testing.T) {
			db, orgID := seedGenericWritesOrg(t)
			markOffboarded(t, db, orgID, "student-1")

			_ = genericPatch(t, db, orgID, actor, "student-1", map[string]any{"is_active": true})
			_ = genericPatch(t, db, orgID, actor, "admin-1", map[string]any{"is_active": false})

			offboarded := loadMember(t, db, orgID, "student-1")
			assert.False(t, offboarded.IsActive, "only reinstate brings an offboarded member back")
			assert.NotNil(t, offboarded.LeftAt)
			assert.True(t, loadMember(t, db, orgID, "admin-1").IsActive, "only offboard deactivates a member")
		})
	}
}

func TestOrgMemberGenericPatch_RoleChange_FollowsTheRankRule(t *testing.T) {
	for _, c := range []struct {
		name, actor, target string
		role                models.OrganizationMemberRole
		allowed             bool
	}{
		{"manager demotes a peer manager", "manager-1", "manager-2", models.OrgRoleMember, false},
		{"manager promotes a member who is a platform administrator", "manager-1", "admin-1", models.OrgRoleManager, false},
		{"owner demotes a manager", "owner-1", "manager-2", models.OrgRoleMember, true},
		{"manager promotes a plain member", "manager-1", "student-1", models.OrgRoleManager, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			db, orgID := seedGenericWritesOrg(t)
			before := loadMember(t, db, orgID, c.target).Role

			err := genericPatch(t, db, orgID, c.actor, c.target, map[string]any{"role": string(c.role)})

			if c.allowed {
				require.NoError(t, err)
				assert.Equal(t, c.role, loadMember(t, db, orgID, c.target).Role)
				return
			}
			require.Error(t, err)
			assert.Equal(t, before, loadMember(t, db, orgID, c.target).Role)
		})
	}
}

// A patch without a role still writes the member: the caller must outrank them.
func TestOrgMemberGenericPatch_MetadataOnly_FollowsTheRankRule(t *testing.T) {
	db, orgID := seedGenericWritesOrg(t)
	body := map[string]any{"metadata": map[string]any{"note": "x"}}

	assert.Error(t, genericPatch(t, db, orgID, "manager-1", "manager-2", body), "a peer manager is out of reach")
	assert.Error(t, genericPatch(t, db, orgID, "manager-1", "admin-1", body), "a platform administrator is out of reach")
}

func TestOrgMemberGenericDelete_FollowsTheRankRule(t *testing.T) {
	for _, c := range []struct {
		name, actor, target string
		allowed             bool
	}{
		{"manager removes a peer manager", "manager-1", "manager-2", false},
		{"manager removes a member who is a platform administrator", "manager-1", "admin-1", false},
		{"owner removes a manager", "owner-1", "manager-2", true},
		{"manager removes a plain member", "manager-1", "student-1", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			db, orgID := seedGenericWritesOrg(t)

			err := genericDelete(t, db, orgID, c.actor, platformMember, c.target)

			if c.allowed {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestOrgMemberGenericDelete_OffboardedRow_Refused(t *testing.T) {
	for _, c := range []struct {
		actor string
		roles []string
	}{
		{"owner-1", platformMember},
		{"platform-admin", platformAdministrator},
	} {
		t.Run(c.actor, func(t *testing.T) {
			db, orgID := seedGenericWritesOrg(t)
			markOffboarded(t, db, orgID, "student-1")

			err := genericDelete(t, db, orgID, c.actor, c.roles, "student-1")

			assert.Error(t, err, "the offboarding owns this row: reinstate or erase it")
		})
	}
}

// genericMemberRoute serves the real generic PATCH and DELETE of
// /organization-members for actor, over the real registration and hooks.
func genericMemberRoute(t *testing.T, db *gorm.DB, actor string, roles []string) func(method, target string, body string) *httptest.ResponseRecorder {
	t.Helper()
	prev := ems.GlobalEntityRegistrationService
	ems.GlobalEntityRegistrationService = ems.NewEntityRegistrationService()
	orgRegistration.RegisterOrganizationMember(ems.GlobalEntityRegistrationService)
	t.Cleanup(func() { ems.GlobalEntityRegistrationService = prev })

	gc := controller.NewGenericController(db, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("userId", actor); c.Set("userRoles", roles); c.Next() })
	router.PATCH("/api/v1/organization-members/:id", gc.EditEntity)
	router.DELETE("/api/v1/organization-members/:id", func(c *gin.Context) { gc.DeleteEntity(c, true) })
	return func(method, target, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, "/api/v1/organization-members/"+target, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(rec, req)
		return rec
	}
}

func memberRowID(t *testing.T, db *gorm.DB, orgID uuid.UUID, userID string) string {
	t.Helper()
	return loadMember(t, db, orgID, userID).ID.String()
}

// A body carrying only is_active is accepted and changes nothing: the field is
// no longer part of the edit DTO (ocf-front never sends it).
func TestOrgMemberGenericPatchRoute_OnlyIsActive_NoContentAndRowUnchanged(t *testing.T) {
	db, orgID := seedGenericWritesOrg(t)
	markOffboarded(t, db, orgID, "student-1")
	patch := genericMemberRoute(t, db, "manager-1", platformMember)

	rec := patch(http.MethodPatch, memberRowID(t, db, orgID, "student-1"), `{"is_active":true}`)
	assert.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	assert.False(t, loadMember(t, db, orgID, "student-1").IsActive)
	assert.NotNil(t, loadMember(t, db, orgID, "student-1").LeftAt)

	seedMember(t, db, orgID, "student-2", models.OrgRoleMember)
	rec = patch(http.MethodPatch, memberRowID(t, db, orgID, "student-2"), `{"is_active":false}`)
	assert.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	assert.True(t, loadMember(t, db, orgID, "student-2").IsActive)
}

func TestOrgMemberGenericRoutes_ActorKinds_StatusAndRow(t *testing.T) {
	for _, c := range []struct {
		name, actor    string
		roles          []string
		method, target string
		body           string
		offboard       bool
		status         int
		rowStays       bool
	}{
		{"actor holds no membership: patch", "stranger", platformMember, http.MethodPatch, "student-1", `{"role":"manager"}`, false, http.StatusForbidden, true},
		{"actor holds no membership: delete", "stranger", platformMember, http.MethodDelete, "student-1", ``, false, http.StatusForbidden, true},
		{"manager patches own metadata", "manager-1", platformMember, http.MethodPatch, "manager-1", `{"metadata":{"a":"b"}}`, false, http.StatusForbidden, true},
		{"manager demotes self", "manager-1", platformMember, http.MethodPatch, "manager-1", `{"role":"member"}`, false, http.StatusForbidden, true},
		{"manager removes self", "manager-1", platformMember, http.MethodDelete, "manager-1", ``, false, http.StatusForbidden, true},
		{"plain member removes self", "student-1", platformMember, http.MethodDelete, "student-1", ``, false, http.StatusForbidden, true},
		{"platform administrator demotes a manager", "platform-admin", platformAdministrator, http.MethodPatch, "manager-2", `{"role":"member"}`, false, http.StatusNoContent, true},
		{"platform administrator promotes a platform administrator member", "platform-admin", platformAdministrator, http.MethodPatch, "admin-1", `{"role":"manager"}`, false, http.StatusNoContent, true},
		{"platform administrator removes a manager", "platform-admin", platformAdministrator, http.MethodDelete, "manager-2", ``, false, http.StatusNoContent, false},
		{"platform administrator removes an offboarded member", "platform-admin", platformAdministrator, http.MethodDelete, "student-1", ``, true, http.StatusConflict, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			db, orgID := seedGenericWritesOrg(t)
			if c.offboard {
				markOffboarded(t, db, orgID, c.target)
			}

			rec := genericMemberRoute(t, db, c.actor, c.roles)(c.method, memberRowID(t, db, orgID, c.target), c.body)

			assert.Equal(t, c.status, rec.Code, rec.Body.String())
			want := int64(0)
			if c.rowStays {
				want = 1
			}
			assert.Equal(t, want, countMemberRows(t, db, orgID, c.target))
		})
	}
}
