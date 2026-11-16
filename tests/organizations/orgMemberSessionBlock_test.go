package organizations_tests

// PUT /organizations/:id/members/:userId/session-block (#580): an organization
// manager blocks a member from starting sessions in the organization.
// Declared OrgRole Param "id" MinRole "manager"; the handler enforces the same
// rule and refuses a target that is the caller, a manager or owner of the
// organization, or not an active member of it.
//
// The handler is reached through an interface assertion on the
// OrganizationController so this package keeps compiling before it exists.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	auditModels "soli/formations/src/audit/models"
	access "soli/formations/src/auth/access"
	"soli/formations/src/auth/mocks"
	"soli/formations/src/entityManagement/hooks"
	groupModels "soli/formations/src/groups/models"
	"soli/formations/src/organizations/controller"
	organizationHooks "soli/formations/src/organizations/hooks"
	"soli/formations/src/organizations/models"
	organizationRoutes "soli/formations/src/organizations/routes"
	organizationServices "soli/formations/src/organizations/services"
	terminalModels "soli/formations/src/terminalTrainer/models"
	testTools "soli/formations/tests/testTools"
)

const (
	obOwner    = "ob-owner"
	obManager  = "ob-manager"
	obLearner  = "ob-learner"
	obLearner2 = "ob-learner2"
	obOutsider = "ob-outsider" // manager of another organization
)

type orgSessionBlockHandler interface {
	SetMemberSessionBlock(*gin.Context)
}

var obDBSeq int

type obFixture struct {
	db       *gorm.DB
	orgID    uuid.UUID
	otherOrg uuid.UUID
	stops    *[]string
	mu       *sync.Mutex
	// running persistent sessions of obLearner: in the org, in another org.
	inOrg, elsewhere string
}

func newOBFixture(t *testing.T) *obFixture {
	t.Helper()
	installMockEnforcer(t) // the target's platform role is read and fails closed without an enforcer
	obDBSeq++
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:org_session_block_%d?mode=memory&cache=shared", obDBSeq)),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.Organization{}, &models.OrganizationMember{},
		&groupModels.ClassGroup{}, &groupModels.GroupMember{},
		&terminalModels.UserTerminalKey{}, &terminalModels.Terminal{}, &terminalModels.ExposedPort{},
		&auditModels.AuditLog{},
	))
	require.NoError(t, testTools.CreateSessionBlocksTable(db))

	f := &obFixture{db: db, orgID: uuid.New(), otherOrg: uuid.New(), stops: &[]string{}, mu: &sync.Mutex{}}
	for _, org := range []struct {
		id    uuid.UUID
		owner string
	}{{f.orgID, obOwner}, {f.otherOrg, obOutsider}} {
		o := &models.Organization{Name: "ob-" + org.id.String()[:8], DisplayName: "ob", OwnerUserID: org.owner, OrganizationType: models.OrgTypeTeam, IsActive: true}
		o.ID = org.id
		require.NoError(t, db.Omit("Metadata").Create(o).Error)
	}
	obMember(t, db, f.orgID, obOwner, models.OrgRoleOwner)
	obMember(t, db, f.orgID, obManager, models.OrgRoleManager)
	obMember(t, db, f.orgID, obLearner, models.OrgRoleMember)
	obMember(t, db, f.orgID, obLearner2, models.OrgRoleMember)
	obMember(t, db, f.otherOrg, obOutsider, models.OrgRoleManager)
	obMember(t, db, f.otherOrg, obLearner, models.OrgRoleMember)

	f.inOrg = obTerminal(t, db, obLearner, &f.orgID)
	f.elsewhere = obTerminal(t, db, obLearner, &f.otherOrg)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/stop") {
			parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
			f.mu.Lock()
			*f.stops = append(*f.stops, parts[len(parts)-2])
			f.mu.Unlock()
			_, _ = fmt.Fprintf(w, `{"state":"stopped","idle_until":%d}`, time.Now().Add(30*time.Minute).Unix())
			return
		}
		http.Error(w, "unexpected request: "+r.Method+" "+r.URL.Path, http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("TERMINAL_TRAINER_URL", srv.URL)
	t.Setenv("TERMINAL_TRAINER_ADMIN_KEY", "test-admin-key")
	t.Setenv("TERMINAL_TRAINER_API_VERSION", "1.0")
	return f
}

func (f *obFixture) stopped() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), *f.stops...)
}

func obMember(t *testing.T, db *gorm.DB, orgID uuid.UUID, userID string, role models.OrganizationMemberRole) {
	t.Helper()
	require.NoError(t, db.Omit("Metadata").Create(&models.OrganizationMember{
		OrganizationID: orgID, UserID: userID, Role: role, JoinedAt: time.Now(), IsActive: true,
	}).Error)
}

func obTerminal(t *testing.T, db *gorm.DB, userID string, orgID *uuid.UUID) string {
	t.Helper()
	key := &terminalModels.UserTerminalKey{UserID: userID, APIKey: "key-" + uuid.NewString(), KeyName: "k", IsActive: true}
	require.NoError(t, db.Create(key).Error)
	term := &terminalModels.Terminal{
		SessionID: "ob-session-" + uuid.NewString(), UserID: userID, State: terminalModels.StateRunning,
		PersistenceMode: terminalModels.PersistenceModePersistent, ExpiresAt: time.Now().Add(time.Hour),
		MachineSize: "S", OrganizationID: orgID, UserTerminalKeyID: key.ID,
	}
	require.NoError(t, db.Create(term).Error)
	return term.SessionID
}

func obState(t *testing.T, db *gorm.DB, sessionID string) terminalModels.TerminalState {
	t.Helper()
	var row terminalModels.Terminal
	require.NoError(t, db.First(&row, "session_id = ?", sessionID).Error)
	return row.State
}

func obPut(t *testing.T, f *obFixture, callerID string, roles []string, orgID uuid.UUID, target string, body any) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	h, ok := any(controller.NewOrganizationController(nil, nil, nil, nil, f.db)).(orgSessionBlockHandler)
	require.True(t, ok, "OrganizationController must expose SetMemberSessionBlock")
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("userId", callerID)
		c.Set("userRoles", roles)
		c.Next()
	})
	r.PUT("/organizations/:id/members/:userId/session-block", h.SetMemberSessionBlock)
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPut, "/organizations/"+orgID.String()+"/members/"+target+"/session-block", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func obOrgBlocks(t *testing.T, db *gorm.DB, userID string, orgID uuid.UUID) []terminalModels.SessionBlock {
	t.Helper()
	var rows []terminalModels.SessionBlock
	require.NoError(t, db.Where("user_id = ? AND scope_type = ? AND scope_id = ?", userID, terminalModels.SessionBlockScopeOrg, orgID).Find(&rows).Error)
	return rows
}

var memberRoles = []string{"member"}

func TestOrgMemberSessionBlock_ManagerBlocksUpdatesAndUnblocks(t *testing.T) {
	f := newOBFixture(t)

	w := obPut(t, f, obManager, memberRoles, f.orgID, obLearner, map[string]any{"blocked": true, "reason": "first"})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	rows := obOrgBlocks(t, f.db, obLearner, f.orgID)
	require.Len(t, rows, 1)
	assert.Equal(t, "first", rows[0].Reason)
	assert.Equal(t, obManager, rows[0].BlockedBy)

	w = obPut(t, f, obManager, memberRoles, f.orgID, obLearner, map[string]any{"blocked": true, "reason": "second"})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	rows = obOrgBlocks(t, f.db, obLearner, f.orgID)
	require.Len(t, rows, 1, "re-blocking updates the block")
	assert.Equal(t, "second", rows[0].Reason)

	w = obPut(t, f, obManager, memberRoles, f.orgID, obLearner, map[string]any{"blocked": false})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.Empty(t, obOrgBlocks(t, f.db, obLearner, f.orgID))

	var set []auditModels.AuditLog
	require.NoError(t, f.db.Where("event_type = ?", "user.session_block.set").Find(&set).Error)
	require.Len(t, set, 2)
	// Contract change (final round, F): identities live in the typed columns
	// only, the reason is recorded as has_reason, and a pre-action row says
	// "requested".
	for _, row := range set {
		var meta map[string]any
		require.NoError(t, json.Unmarshal([]byte(row.Metadata), &meta))
		for _, key := range []string{"actor_user_id", "target_user_id", "impersonator_id", "reason"} {
			assert.NotContains(t, meta, key)
		}
		assert.Equal(t, true, meta["has_reason"])
		assert.Equal(t, "requested", row.Status)
		assert.Contains(t, row.Metadata, f.orgID.String())
		assert.Contains(t, row.Metadata, terminalModels.SessionBlockScopeOrg)
	}
	var cleared int64
	require.NoError(t, f.db.Model(&auditModels.AuditLog{}).Where("event_type = ?", "user.session_block.cleared").Count(&cleared).Error)
	assert.Equal(t, int64(1), cleared)
	assert.Empty(t, f.stopped(), "stop_running defaults to false")
}

func TestOrgMemberSessionBlock_StopRunning_StopsTheirSessionsInThatOrgOnly(t *testing.T) {
	f := newOBFixture(t)
	w := obPut(t, f, obManager, memberRoles, f.orgID, obLearner, map[string]any{"blocked": true, "reason": "r", "stop_running": true})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.Equal(t, []string{f.inOrg}, f.stopped())
	assert.Equal(t, terminalModels.StateStopped, obState(t, f.db, f.inOrg))
	assert.Equal(t, terminalModels.StateRunning, obState(t, f.db, f.elsewhere))
}

func TestOrgMemberSessionBlock_PlatformAdmin_Allowed(t *testing.T) {
	f := newOBFixture(t)
	w := obPut(t, f, "ob-admin", []string{"administrator"}, f.orgID, obLearner, map[string]any{"blocked": true, "reason": "r"})
	assert.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.Len(t, obOrgBlocks(t, f.db, obLearner, f.orgID), 1)
}

func TestOrgMemberSessionBlock_Refusals_403(t *testing.T) {
	cases := []struct {
		name, caller, target string
	}{
		{"plain member", obLearner2, obLearner},
		{"manager of another organization", obOutsider, obLearner},
		{"target is the caller", obManager, obManager},
		{"target is an org manager", obOwner, obManager},
		{"target is the org owner", obManager, obOwner},
		{"target is not a member", obManager, "ob-nobody"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newOBFixture(t)
			w := obPut(t, f, tc.caller, memberRoles, f.orgID, tc.target, map[string]any{"blocked": true, "reason": "r", "stop_running": true})
			assert.Equal(t, http.StatusForbidden, w.Code, "body=%s", w.Body.String())
			assert.Equal(t, int64(0), testTools.CountSessionBlocks(f.db, tc.target))
			assert.Empty(t, f.stopped())
		})
	}
}

func TestOrgMemberSessionBlock_InactiveMember_403(t *testing.T) {
	f := newOBFixture(t)
	require.NoError(t, f.db.Model(&models.OrganizationMember{}).
		Where("organization_id = ? AND user_id = ?", f.orgID, obLearner).Update("is_active", false).Error)
	w := obPut(t, f, obManager, memberRoles, f.orgID, obLearner, map[string]any{"blocked": true, "reason": "r"})
	assert.Equal(t, http.StatusForbidden, w.Code, "body=%s", w.Body.String())
	assert.Empty(t, obOrgBlocks(t, f.db, obLearner, f.orgID))
}

func TestOrgMemberSessionBlock_ReasonOver500Chars_400(t *testing.T) {
	f := newOBFixture(t)
	w := obPut(t, f, obManager, memberRoles, f.orgID, obLearner, map[string]any{"blocked": true, "reason": strings.Repeat("x", 501)})
	assert.Equal(t, http.StatusBadRequest, w.Code, "body=%s", w.Body.String())
	assert.Empty(t, obOrgBlocks(t, f.db, obLearner, f.orgID))
}

func TestOrgMemberSessionBlock_AuditWriteFails_NoBlockNoStop(t *testing.T) {
	f := newOBFixture(t)
	require.NoError(t, f.db.Callback().Create().Before("gorm:create").Register("ob:fail_audit", func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "audit_logs" {
			_ = tx.AddError(errors.New("audit store unavailable"))
		}
	}))
	w := obPut(t, f, obManager, memberRoles, f.orgID, obLearner, map[string]any{"blocked": true, "reason": "r", "stop_running": true})
	assert.Equal(t, http.StatusInternalServerError, w.Code, "body=%s", w.Body.String())
	assert.Empty(t, obOrgBlocks(t, f.db, obLearner, f.orgID), "audit before acting: no audit, no block")
	assert.Empty(t, f.stopped())
}

func TestOrgMemberSessionBlockRoute_DeclaredOrgRoleManager(t *testing.T) {
	access.RouteRegistry.Reset()
	t.Cleanup(func() { access.RouteRegistry.Reset() })
	organizationRoutes.RegisterOrganizationPermissions(mocks.NewMockEnforcer())

	perm, found := access.RouteRegistry.Lookup("PUT", "/api/v1/organizations/:id/members/:userId/session-block")
	require.True(t, found, "route must be declared in the RouteRegistry")
	assert.Equal(t, access.RoleMember, perm.Role)
	assert.Equal(t, access.OrgRole, perm.Access.Type)
	assert.Equal(t, "id", perm.Access.Param)
	assert.Equal(t, "manager", perm.Access.MinRole)
}

// =============================================================================
// Final round (#580)
// =============================================================================

func TestOrgMemberSessionBlock_UnblockWithoutBlock_200NoClearedAudit(t *testing.T) {
	f := newOBFixture(t)
	w := obPut(t, f, obManager, memberRoles, f.orgID, obLearner, map[string]any{"blocked": false})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	var cleared int64
	require.NoError(t, f.db.Model(&auditModels.AuditLog{}).Where("event_type = ?", "user.session_block.cleared").Count(&cleared).Error)
	assert.Zero(t, cleared, "nothing was cleared: no cleared row")
}

// seedLeaverBlocks gives obLearner a block in f.orgID, one in f.otherOrg, a
// class block and a global block.
func seedLeaverBlocks(t *testing.T, f *obFixture) {
	t.Helper()
	class := uuid.New()
	require.NoError(t, testTools.SeedSessionBlock(f.db, obLearner, "org", &f.orgID, "r"))
	require.NoError(t, testTools.SeedSessionBlock(f.db, obLearner, "org", &f.otherOrg, "r"))
	require.NoError(t, testTools.SeedSessionBlock(f.db, obLearner, "group", &class, "r"))
	require.NoError(t, testTools.SeedSessionBlock(f.db, obLearner, "global", nil, "r"))
}

func assertOnlyThatOrgBlockRemoved(t *testing.T, f *obFixture) {
	t.Helper()
	assert.Empty(t, obOrgBlocks(t, f.db, obLearner, f.orgID), "the block in the organization the member left is deleted")
	assert.Len(t, obOrgBlocks(t, f.db, obLearner, f.otherOrg), 1, "the block in another organization stays")
	assert.EqualValues(t, 3, testTools.CountSessionBlocks(f.db, obLearner), "class and platform blocks stay")
}

func TestOrgMemberDeletionHook_DeletesThatOrgSessionBlock(t *testing.T) {
	useMockEnforcer(t)
	f := newOBFixture(t)
	seedLeaverBlocks(t, f)

	hook := organizationHooks.NewOrganizationMemberDeletionHook(f.db)
	require.NoError(t, hook.Execute(&hooks.HookContext{
		EntityName: "OrganizationMember",
		HookType:   hooks.BeforeDelete,
		NewEntity:  &models.OrganizationMember{OrganizationID: f.orgID, UserID: obLearner, Role: models.OrgRoleMember},
		UserID:     obManager,
		UserRoles:  memberRoles,
	}))
	assertOnlyThatOrgBlockRemoved(t, f)
}

func TestRemoveMemberFromOrganization_DeletesThatOrgSessionBlock(t *testing.T) {
	useMockEnforcer(t)
	f := newOBFixture(t)
	seedLeaverBlocks(t, f)

	require.NoError(t, organizationServices.NewOrganizationService(f.db).RemoveMemberFromOrganization(f.orgID, obManager, obLearner))
	assertOnlyThatOrgBlockRemoved(t, f)
}

// A platform administrator who is a plain member of the organization is out of
// a manager's and the owner's reach, as under the shared member rank rule.
func TestOrgMemberSessionBlock_PlatformAdministratorTarget_403(t *testing.T) {
	for _, caller := range []string{obManager, obOwner} {
		t.Run(caller, func(t *testing.T) {
			f := newOBFixture(t)
			installMockEnforcer(t).GetRolesForUserFunc = adminRoleFor("ob-platform-admin")
			obMember(t, f.db, f.orgID, "ob-platform-admin", models.OrgRoleMember)

			w := obPut(t, f, caller, memberRoles, f.orgID, "ob-platform-admin", map[string]any{"blocked": true, "reason": "r"})

			assert.Equal(t, http.StatusForbidden, w.Code, "body=%s", w.Body.String())
			assert.Empty(t, obOrgBlocks(t, f.db, "ob-platform-admin", f.orgID))
		})
	}
}

// Pins the inline rule's reach today: below manager is in reach, so a manager
// blocks a teacher; the owner blocking a manager is refused (Refusals_403).
func TestOrgMemberSessionBlock_ManagerBlocksTeacher_200(t *testing.T) {
	f := newOBFixture(t)
	installMockEnforcer(t)
	obMember(t, f.db, f.orgID, "ob-teacher", models.OrganizationMemberRole(access.RoleTeacher))

	w := obPut(t, f, obManager, memberRoles, f.orgID, "ob-teacher", map[string]any{"blocked": true, "reason": "r"})

	assert.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.Len(t, obOrgBlocks(t, f.db, "ob-teacher", f.orgID), 1)
}
