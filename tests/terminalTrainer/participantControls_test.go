package terminalTrainer_tests

// Trainer controls on class participants (#580): one bulk read for the
// members list, the live wall and the supervisor session page, and three
// moderation actions (stop sessions, cut or restore internet, block session
// starts in the class).
//
// Routes (all declared GroupRole Param "id" MinRole "manager"; the handlers
// enforce the same rule themselves, like the supervision wall does):
//
//	GET  /class-groups/:id/participant-controls
//	POST /class-groups/:id/participants/:userId/stop-sessions   {session_ids?}
//	PUT  /class-groups/:id/participants/:userId/network         {enabled, session_ids?}
//	PUT  /class-groups/:id/participants/:userId/session-block   {blocked, reason?, stop_running?}
//
// The handlers are reached through participantControlsHandlers, an interface
// assertion on the TerminalController, so this package keeps compiling before
// they exist: each test then fails on its own, saying which handler is missing.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	auditModels "soli/formations/src/audit/models"
	access "soli/formations/src/auth/access"
	"soli/formations/src/auth/mocks"
	groupModels "soli/formations/src/groups/models"
	scenarioModels "soli/formations/src/scenarios/models"
	"soli/formations/src/terminalTrainer/models"
	terminalController "soli/formations/src/terminalTrainer/routes"
	"soli/formations/src/terminalTrainer/services"
	testTools "soli/formations/tests/testTools"
)

// Audit event types the actions write (src/audit/models/auditLog.go). Kept as
// literals so the package compiles before the constants exist.
const (
	auditStoppedByTrainer  = "terminal.stopped_by_trainer"
	auditNetworkDisabled   = "terminal.network.disabled"
	auditNetworkEnabled    = "terminal.network.enabled"
	auditSessionBlockSet   = "user.session_block.set"
	auditSessionBlockClear = "user.session_block.cleared"
)

// participantControlsHandlers are the four handlers the TerminalController
// must expose.
type participantControlsHandlers interface {
	GetParticipantControls(*gin.Context)
	StopParticipantSessions(*gin.Context)
	SetParticipantNetwork(*gin.Context)
	SetParticipantSessionBlock(*gin.Context)
}

// --- Fixture ------------------------------------------------------------------

// pcClass is a class in organization orgID: owner "pc-owner", manager
// "pc-manager", learners "pc-learner" and "pc-learner2". A second class in
// another organization is managed by "pc-outsider" and holds "pc-other-learner".
type pcClass struct {
	db         *gorm.DB
	group      *groupModels.ClassGroup
	orgID      uuid.UUID
	otherGroup *groupModels.ClassGroup
	// sessions by label: "owner", "learner", "learner2", "other-learner"
	// (one running, persistent, network-less session each, in their class's org).
	sessions map[string]string
}

const (
	pcOwner        = "pc-owner"
	pcManager      = "pc-manager"
	pcLearner      = "pc-learner"
	pcLearner2     = "pc-learner2"
	pcOutsider     = "pc-outsider"
	pcOtherLearner = "pc-other-learner"
	pcAdmin        = "pc-platform-admin"
)

func newPCClass(t *testing.T) *pcClass {
	t.Helper()
	db := freshTestDB(t)
	stubPCNames(t)

	orgID := uuid.New()
	group := &groupModels.ClassGroup{Name: "pc-class", DisplayName: "pc-class", OwnerUserID: pcOwner, MaxMembers: 50, OrganizationID: &orgID}
	require.NoError(t, db.Omit("Metadata").Create(group).Error)
	createTestGroupMember(t, db, group.ID, pcOwner, groupModels.GroupMemberRoleOwner)
	createTestGroupMember(t, db, group.ID, pcManager, groupModels.GroupMemberRoleManager)
	createTestGroupMember(t, db, group.ID, pcLearner, groupModels.GroupMemberRoleMember)
	createTestGroupMember(t, db, group.ID, pcLearner2, groupModels.GroupMemberRoleMember)

	otherOrg := uuid.New()
	other := &groupModels.ClassGroup{Name: "pc-other", DisplayName: "pc-other", OwnerUserID: pcOutsider, MaxMembers: 50, OrganizationID: &otherOrg}
	require.NoError(t, db.Omit("Metadata").Create(other).Error)
	createTestGroupMember(t, db, other.ID, pcOutsider, groupModels.GroupMemberRoleOwner)
	createTestGroupMember(t, db, other.ID, pcOtherLearner, groupModels.GroupMemberRoleMember)

	c := &pcClass{db: db, group: group, orgID: orgID, otherGroup: other, sessions: map[string]string{}}
	c.sessions["owner"] = pcTerminal(t, db, pcOwner, &orgID, models.StateRunning, "")
	c.sessions["learner"] = pcTerminal(t, db, pcLearner, &orgID, models.StateRunning, "")
	c.sessions["learner2"] = pcTerminal(t, db, pcLearner2, &orgID, models.StateRunning, "")
	c.sessions["other-learner"] = pcTerminal(t, db, pcOtherLearner, &otherOrg, models.StateRunning, "")
	return c
}

// pcTerminal creates a persistent terminal for userID in orgID (nil: personal)
// with the given state and composed features JSON, and returns its session id.
func pcTerminal(t *testing.T, db *gorm.DB, userID string, orgID *uuid.UUID, state models.TerminalState, features string) string {
	t.Helper()
	key, err := createTestUserKey(db, userID+"-"+uuid.NewString())
	require.NoError(t, err)
	term := &models.Terminal{
		SessionID:         "pc-session-" + uuid.NewString(),
		UserID:            userID,
		Name:              "Terminal of " + userID,
		State:             state,
		PersistenceMode:   models.PersistenceModePersistent,
		ExpiresAt:         time.Now().Add(time.Hour),
		InstanceType:      "test",
		MachineSize:       "S",
		OrganizationID:    orgID,
		ComposedFeatures:  features,
		UserTerminalKeyID: key.ID,
	}
	require.NoError(t, db.Create(term).Error)
	return term.SessionID
}

// stubPCNames answers the display-name seam the supervision wall uses.
func stubPCNames(t *testing.T) {
	t.Helper()
	original := services.LookupCasdoorUserForOrgUsage
	services.LookupCasdoorUserForOrgUsage = func(id string) (*casdoorsdk.User, error) {
		return &casdoorsdk.User{Id: id, DisplayName: "Name of " + id, Email: id + "@example.com"}, nil
	}
	t.Cleanup(func() { services.LookupCasdoorUserForOrgUsage = original })
}

// --- Fake tt-backend ---------------------------------------------------------

type pcNetworkCall struct {
	sessionID string
	apiKey    string
	enabled   *bool
}

// pcTT records the stops and network toggles ocf-core sends. networkReply maps
// a session id to the (status, code) tt-backend answers its toggle with; a
// missing entry succeeds.
type pcTT struct {
	mu           sync.Mutex
	stops        []string
	network      []pcNetworkCall
	unexpected   []string
	networkReply map[string]struct {
		status int
		code   string
	}
}

func (r *pcTT) stopped() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.stops...)
}

func (r *pcTT) toggles() []pcNetworkCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]pcNetworkCall(nil), r.network...)
}

func newPCTT(t *testing.T) *pcTT {
	t.Helper()
	rec := &pcTT{networkReply: map[string]struct {
		status int
		code   string
	}{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/stop") && len(parts) >= 2:
			id := parts[len(parts)-2]
			rec.mu.Lock()
			rec.stops = append(rec.stops, id)
			rec.mu.Unlock()
			_, _ = w.Write([]byte(ttStoppedBody(time.Now().Add(30 * time.Minute))))
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/1.0/admin/sessions/") && strings.HasSuffix(r.URL.Path, "/network"):
			id := parts[len(parts)-2]
			raw, _ := io.ReadAll(r.Body)
			var body struct {
				Enabled *bool `json:"enabled"`
			}
			_ = json.Unmarshal(raw, &body)
			rec.mu.Lock()
			rec.network = append(rec.network, pcNetworkCall{sessionID: id, apiKey: r.Header.Get("X-API-Key"), enabled: body.Enabled})
			reply, failing := rec.networkReply[id]
			rec.mu.Unlock()
			if failing {
				w.WriteHeader(reply.status)
				_ = json.NewEncoder(w).Encode(map[string]string{"code": reply.code, "error": "tt-backend says " + reply.code})
				return
			}
			disabled := body.Enabled != nil && !*body.Enabled
			_ = json.NewEncoder(w).Encode(map[string]any{"session_id": id, "network_disabled": disabled})
		default:
			rec.mu.Lock()
			rec.unexpected = append(rec.unexpected, r.Method+" "+r.URL.Path)
			rec.mu.Unlock()
			http.Error(w, "unexpected request: "+r.Method+" "+r.URL.Path, http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	configureTTServer(t, srv.URL)
	return rec
}

// --- Calling the routes -------------------------------------------------------

type pcCaller struct {
	userID       string
	roles        []string
	impersonator string
}

func member(userID string) pcCaller { return pcCaller{userID: userID, roles: []string{"member"}} }

func pcRouter(t *testing.T, db *gorm.DB, caller pcCaller) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	h, ok := any(terminalController.NewTerminalController(db)).(participantControlsHandlers)
	require.True(t, ok, "TerminalController must expose GetParticipantControls, StopParticipantSessions, SetParticipantNetwork and SetParticipantSessionBlock")

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("userId", caller.userID)
		c.Set("userRoles", caller.roles)
		if caller.impersonator != "" {
			c.Set("impersonatorId", caller.impersonator)
		}
		c.Next()
	})
	r.GET("/class-groups/:id/participant-controls", h.GetParticipantControls)
	r.POST("/class-groups/:id/participants/:userId/stop-sessions", h.StopParticipantSessions)
	r.PUT("/class-groups/:id/participants/:userId/network", h.SetParticipantNetwork)
	r.PUT("/class-groups/:id/participants/:userId/session-block", h.SetParticipantSessionBlock)
	return r
}

func pcDo(t *testing.T, db *gorm.DB, caller pcCaller, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	pcRouter(t, db, caller).ServeHTTP(w, req)
	return w
}

func controlsPath(groupID uuid.UUID) string {
	return "/class-groups/" + groupID.String() + "/participant-controls"
}

func participantPath(groupID uuid.UUID, userID, action string) string {
	return "/class-groups/" + groupID.String() + "/participants/" + userID + "/" + action
}

// --- Response shape -----------------------------------------------------------

type pcBlockView struct {
	Reason    string    `json:"reason"`
	BlockedBy string    `json:"blocked_by"`
	CreatedAt time.Time `json:"created_at"`
}

type pcSessionView struct {
	SessionID         string  `json:"session_id"`
	State             string  `json:"state"`
	NetworkFeature    bool    `json:"network_feature"`
	NetworkDisabled   bool    `json:"network_disabled"`
	ScenarioSessionID *string `json:"scenario_session_id"`
	ScenarioTitle     *string `json:"scenario_title"`
}

type pcParticipantView struct {
	UserID      string          `json:"user_id"`
	UserName    string          `json:"user_name"`
	ClassBlock  *pcBlockView    `json:"class_block"`
	OrgBlock    *pcBlockView    `json:"org_block"`
	GlobalBlock bool            `json:"global_block"`
	Sessions    []pcSessionView `json:"sessions"`
}

func readControls(t *testing.T, c *pcClass, caller pcCaller) map[string]pcParticipantView {
	t.Helper()
	w := pcDo(t, c.db, caller, http.MethodGet, controlsPath(c.group.ID), nil)
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	var rows []pcParticipantView
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rows), "body=%s", w.Body.String())
	byUser := make(map[string]pcParticipantView, len(rows))
	for _, row := range rows {
		byUser[row.UserID] = row
	}
	return byUser
}

type pcActionResult struct {
	SessionID string `json:"session_id"`
	OK        bool   `json:"ok"`
	Skipped   bool   `json:"skipped"`
	Code      string `json:"code"`
}

func readResults(t *testing.T, w *httptest.ResponseRecorder) map[string]pcActionResult {
	t.Helper()
	var body struct {
		Results []pcActionResult `json:"results"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), "body=%s", w.Body.String())
	out := make(map[string]pcActionResult, len(body.Results))
	for _, r := range body.Results {
		out[r.SessionID] = r
	}
	return out
}

// --- DB helpers ---------------------------------------------------------------

func pcState(t *testing.T, db *gorm.DB, sessionID string) models.TerminalState {
	t.Helper()
	var row models.Terminal
	require.NoError(t, db.First(&row, "session_id = ?", sessionID).Error)
	return row.State
}

// networkDisabledColumn reads terminals.network_disabled with raw SQL, so this
// file compiles before the Terminal.NetworkDisabled field exists.
func networkDisabledColumn(t *testing.T, db *gorm.DB, sessionID string) bool {
	t.Helper()
	var disabled bool
	require.NoError(t, db.Raw("SELECT network_disabled FROM terminals WHERE session_id = ?", sessionID).Row().Scan(&disabled),
		"terminals.network_disabled must exist (Terminal.NetworkDisabled, default false)")
	return disabled
}

func setNetworkDisabledColumn(t *testing.T, db *gorm.DB, sessionID string, disabled bool) {
	t.Helper()
	require.NoError(t, db.Exec("UPDATE terminals SET network_disabled = ? WHERE session_id = ?", disabled, sessionID).Error,
		"terminals.network_disabled must exist (Terminal.NetworkDisabled, default false)")
}

func seedExposedPort(t *testing.T, db *gorm.DB, sessionID string) {
	t.Helper()
	var term models.Terminal
	require.NoError(t, db.First(&term, "session_id = ?", sessionID).Error)
	require.NoError(t, db.Create(&models.ExposedPort{
		TerminalID: term.ID, SessionID: sessionID, UserID: term.UserID, ContainerPort: 8080,
		Slug: "pc" + strings.ReplaceAll(uuid.NewString(), "-", "")[:20], ContainerIP: "10.0.0.5", ExpiresAt: time.Now().Add(time.Hour),
	}).Error)
}

func exposedPortCount(t *testing.T, db *gorm.DB, sessionID string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&models.ExposedPort{}).Where("session_id = ?", sessionID).Count(&n).Error)
	return n
}

func auditRowsOf(t *testing.T, db *gorm.DB, eventType string) []auditModels.AuditLog {
	t.Helper()
	var rows []auditModels.AuditLog
	require.NoError(t, db.Where("event_type = ?", eventType).Find(&rows).Error)
	return rows
}

// failAuditWrites makes every insert into audit_logs fail until the test ends.
func failAuditWrites(t *testing.T, db *gorm.DB) {
	t.Helper()
	name := "pc:fail_audit_" + uuid.NewString()
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "audit_logs" {
			_ = tx.AddError(errors.New("audit store unavailable"))
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Create().Remove(name) })
}

func classBlockRows(t *testing.T, db *gorm.DB, userID string, groupID uuid.UUID) []models.SessionBlock {
	t.Helper()
	var rows []models.SessionBlock
	require.NoError(t, db.Where("user_id = ? AND scope_type = ? AND scope_id = ?", userID, models.SessionBlockScopeGroup, groupID).Find(&rows).Error)
	return rows
}

// =============================================================================
// Authorization
// =============================================================================

type pcRoute struct {
	name   string
	method string
	path   func(c *pcClass, target string) string
	body   any
}

func pcRoutes() []pcRoute {
	return []pcRoute{
		{"read", http.MethodGet, func(c *pcClass, _ string) string { return controlsPath(c.group.ID) }, nil},
		{"stop", http.MethodPost, func(c *pcClass, target string) string { return participantPath(c.group.ID, target, "stop-sessions") }, map[string]any{}},
		{"network", http.MethodPut, func(c *pcClass, target string) string { return participantPath(c.group.ID, target, "network") }, map[string]any{"enabled": false}},
		{"block", http.MethodPut, func(c *pcClass, target string) string { return participantPath(c.group.ID, target, "session-block") }, map[string]any{"blocked": true, "reason": "test", "stop_running": true}},
	}
}

// assertNothingDone checks a refused call left every session, block and
// toggle as it was.
func assertNothingDone(t *testing.T, c *pcClass, tt *pcTT) {
	t.Helper()
	assert.Empty(t, tt.stopped(), "a refused call must not stop any session")
	assert.Empty(t, tt.toggles(), "a refused call must not toggle any network")
	assert.Equal(t, int64(0), testTools.CountSessionBlocks(c.db, pcLearner), "a refused call must not block anyone")
	assert.Equal(t, models.StateRunning, pcState(t, c.db, c.sessions["learner"]))
}

func TestParticipantControlsRoutes_ManagerOfAnotherClass_403(t *testing.T) {
	c := newPCClass(t)
	tt := newPCTT(t)
	for _, route := range pcRoutes() {
		t.Run(route.name, func(t *testing.T) {
			w := pcDo(t, c.db, member(pcOutsider), route.method, route.path(c, pcLearner), route.body)
			assert.Equal(t, http.StatusForbidden, w.Code, "body=%s", w.Body.String())
			assert.NotContains(t, w.Body.String(), c.sessions["learner"], "a refusal must not leak a session id")
		})
	}
	assertNothingDone(t, c, tt)
}

func TestParticipantControlsRoutes_LearnerOfTheClass_403(t *testing.T) {
	c := newPCClass(t)
	tt := newPCTT(t)
	for _, route := range pcRoutes() {
		t.Run(route.name, func(t *testing.T) {
			w := pcDo(t, c.db, member(pcLearner2), route.method, route.path(c, pcLearner), route.body)
			assert.Equal(t, http.StatusForbidden, w.Code, "body=%s", w.Body.String())
		})
	}
	assertNothingDone(t, c, tt)
}

func TestParticipantControlsRoutes_ArchivedClass_403EvenForItsOwner(t *testing.T) {
	c := newPCClass(t)
	tt := newPCTT(t)
	require.NoError(t, c.db.Model(&groupModels.ClassGroup{}).Where("id = ?", c.group.ID).Update("archived_at", time.Now()).Error)
	for _, route := range pcRoutes() {
		t.Run(route.name, func(t *testing.T) {
			w := pcDo(t, c.db, member(pcOwner), route.method, route.path(c, pcLearner), route.body)
			assert.Equal(t, http.StatusForbidden, w.Code, "body=%s", w.Body.String())
		})
	}
	assertNothingDone(t, c, tt)
}

func TestParticipantControlsRoutes_ClassManager_Allowed(t *testing.T) {
	for _, route := range pcRoutes() {
		t.Run(route.name, func(t *testing.T) {
			c := newPCClass(t)
			newPCTT(t)
			w := pcDo(t, c.db, member(pcManager), route.method, route.path(c, pcLearner), route.body)
			assert.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
		})
	}
}

func TestParticipantControlsRoutes_PlatformAdmin_Allowed(t *testing.T) {
	for _, route := range pcRoutes() {
		t.Run(route.name, func(t *testing.T) {
			c := newPCClass(t)
			newPCTT(t)
			admin := pcCaller{userID: pcAdmin, roles: []string{"administrator"}}
			w := pcDo(t, c.db, admin, route.method, route.path(c, pcLearner), route.body)
			assert.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
		})
	}
}

// A trainer acts on participants only: never on themselves, on a manager or
// owner of the class, or on someone who is not an active member of it.
func TestParticipantControlsWrites_TargetNotAParticipant_403(t *testing.T) {
	targets := []struct {
		name   string
		caller string
		target string
	}{
		{"self", pcManager, pcManager},
		{"class owner", pcManager, pcOwner},
		{"class manager", pcOwner, pcManager},
		{"member of another class", pcManager, pcOtherLearner},
		{"unknown user", pcManager, "pc-nobody"},
	}
	for _, target := range targets {
		for _, route := range pcRoutes()[1:] {
			t.Run(target.name+"/"+route.name, func(t *testing.T) {
				c := newPCClass(t)
				tt := newPCTT(t)
				w := pcDo(t, c.db, member(target.caller), route.method, route.path(c, target.target), route.body)
				assert.Equal(t, http.StatusForbidden, w.Code, "body=%s", w.Body.String())
				assert.Empty(t, tt.stopped())
				assert.Empty(t, tt.toggles())
				assert.Equal(t, int64(0), testTools.CountSessionBlocks(c.db, target.target))
			})
		}
	}
}

func TestParticipantControlsWrites_InactiveMember_403(t *testing.T) {
	c := newPCClass(t)
	tt := newPCTT(t)
	require.NoError(t, c.db.Model(&groupModels.GroupMember{}).
		Where("group_id = ? AND user_id = ?", c.group.ID, pcLearner).Update("is_active", false).Error)
	for _, route := range pcRoutes()[1:] {
		t.Run(route.name, func(t *testing.T) {
			w := pcDo(t, c.db, member(pcManager), route.method, route.path(c, pcLearner), route.body)
			assert.Equal(t, http.StatusForbidden, w.Code, "body=%s", w.Body.String())
		})
	}
	assertNothingDone(t, c, tt)
}

func TestParticipantControlsRoutes_DeclaredGroupRoleManager(t *testing.T) {
	access.RouteRegistry.Reset()
	t.Cleanup(func() { access.RouteRegistry.Reset() })
	terminalController.RegisterTerminalPermissions(mocks.NewMockEnforcer())

	for _, route := range []struct{ method, path string }{
		{"GET", "/api/v1/class-groups/:id/participant-controls"},
		{"POST", "/api/v1/class-groups/:id/participants/:userId/stop-sessions"},
		{"PUT", "/api/v1/class-groups/:id/participants/:userId/network"},
		{"PUT", "/api/v1/class-groups/:id/participants/:userId/session-block"},
	} {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			perm, found := access.RouteRegistry.Lookup(route.method, route.path)
			require.True(t, found, "route must be declared in the RouteRegistry")
			assert.Equal(t, access.RoleMember, perm.Role)
			assert.Equal(t, access.GroupRole, perm.Access.Type)
			assert.Equal(t, "id", perm.Access.Param)
			assert.Equal(t, "manager", perm.Access.MinRole)
		})
	}
}

// =============================================================================
// Bulk read
// =============================================================================

func TestParticipantControls_ListsEveryActiveMemberWithTheirBlocks(t *testing.T) {
	c := newPCClass(t)
	createTestGroupMember(t, c.db, c.group.ID, "pc-gone", groupModels.GroupMemberRoleMember)
	require.NoError(t, c.db.Model(&groupModels.GroupMember{}).
		Where("group_id = ? AND user_id = ?", c.group.ID, "pc-gone").Update("is_active", false).Error)

	blockSessions(t, c.db, pcLearner, models.SessionBlockScopeGroup, &c.group.ID, "class reason")
	blockSessions(t, c.db, pcLearner, models.SessionBlockScopeOrg, &c.orgID, "org reason")
	blockSessions(t, c.db, pcLearner2, models.SessionBlockScopeGlobal, nil, "global reason")
	// Blocks elsewhere must not show: another class, another organization.
	blockSessions(t, c.db, pcLearner2, models.SessionBlockScopeGroup, &c.otherGroup.ID, "elsewhere")
	blockSessions(t, c.db, pcLearner2, models.SessionBlockScopeOrg, c.otherGroup.OrganizationID, "elsewhere")

	rows := readControls(t, c, member(pcManager))

	assert.ElementsMatch(t, []string{pcOwner, pcManager, pcLearner, pcLearner2}, keysOf(rows),
		"one row per ACTIVE member of the class, nobody else")

	learner := rows[pcLearner]
	require.NotNil(t, learner.ClassBlock, "class block must be reported")
	assert.Equal(t, "class reason", learner.ClassBlock.Reason)
	assert.Equal(t, "test-blocker", learner.ClassBlock.BlockedBy)
	assert.False(t, learner.ClassBlock.CreatedAt.IsZero())
	require.NotNil(t, learner.OrgBlock, "a block in the class's organization must be reported")
	assert.Equal(t, "org reason", learner.OrgBlock.Reason)
	assert.False(t, learner.GlobalBlock)

	learner2 := rows[pcLearner2]
	assert.Nil(t, learner2.ClassBlock, "a block in another class is not this class's")
	assert.Nil(t, learner2.OrgBlock, "a block in another organization is not this class's")
	assert.True(t, learner2.GlobalBlock)

	assert.Equal(t, "Name of "+pcLearner, learner.UserName)
	require.Len(t, learner.Sessions, 1)
	assert.Equal(t, c.sessions["learner"], learner.Sessions[0].SessionID)
	assert.Equal(t, string(models.StateRunning), learner.Sessions[0].State)
}

func TestParticipantControls_UnblockedParticipant_BlocksSerializeAsNull(t *testing.T) {
	c := newPCClass(t)
	w := pcDo(t, c.db, member(pcManager), http.MethodGet, controlsPath(c.group.ID), nil)
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	var rows []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rows))
	require.NotEmpty(t, rows)
	for _, row := range rows {
		for _, key := range []string{"user_id", "user_name", "class_block", "org_block", "global_block", "sessions"} {
			assert.Contains(t, row, key, "every participant row carries %q", key)
		}
		assert.Nil(t, row["class_block"])
		assert.Nil(t, row["org_block"])
		assert.Equal(t, false, row["global_block"])
	}
}

// The sessions listed are exactly those GET /terminals/user-sessions?group_id=
// returns for the class: running and stopped, in the class's organization only.
func TestParticipantControls_SessionsAreExactlyTheGroupFilterOfUserSessions(t *testing.T) {
	c := newPCClass(t)
	pcTerminal(t, c.db, pcLearner, &c.orgID, models.StateStopped, "")
	pcTerminal(t, c.db, pcLearner, nil, models.StateRunning, "")                         // personal
	pcTerminal(t, c.db, pcLearner, c.otherGroup.OrganizationID, models.StateRunning, "") // other organization
	pcTerminal(t, c.db, pcLearner2, &c.orgID, models.StateStopped, `{"network":true}`)

	rows := readControls(t, c, member(pcOwner))
	var listed []string
	for _, row := range rows {
		for _, s := range row.Sessions {
			listed = append(listed, s.SessionID)
		}
	}

	w := getUserSessionsAs(t, c.db, pcOwner, []string{"member"}, "?group_id="+c.group.ID.String())
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.ElementsMatch(t, sessionIDsIn(t, w), listed,
		"participant-controls must list the same sessions as user-sessions?group_id (groupMemberSessions)")
}

func TestParticipantControls_UserNameIsTheWallName(t *testing.T) {
	c := newPCClass(t)
	original := services.LookupCasdoorUserForOrgUsage
	services.LookupCasdoorUserForOrgUsage = func(id string) (*casdoorsdk.User, error) {
		if id == pcLearner {
			return &casdoorsdk.User{Id: id, DisplayName: "Ada Lovelace", Email: "ada@example.com"}, nil
		}
		return nil, fmt.Errorf("unknown user")
	}
	t.Cleanup(func() { services.LookupCasdoorUserForOrgUsage = original })

	wall, ok := terminalController.ListGroupSupervisionSessions(c.db, c.group.ID.String(), pcOwner, false)
	require.True(t, ok)
	wallNames := map[string]string{}
	for _, s := range wall {
		wallNames[s.UserID] = s.UserName
	}

	rows := readControls(t, c, member(pcOwner))
	assert.Equal(t, "Ada Lovelace", rows[pcLearner].UserName)
	assert.Equal(t, wallNames[pcLearner], rows[pcLearner].UserName, "user_name must be the wall's user_name")
	assert.Equal(t, wallNames[pcLearner2], rows[pcLearner2].UserName,
		"a name the resolver cannot find falls back the same way the wall's does")
}

func TestParticipantControls_SessionNetworkFlags(t *testing.T) {
	c := newPCClass(t)
	withNetwork := pcTerminal(t, c.db, pcLearner2, &c.orgID, models.StateRunning, `{"network":true}`)
	cut := pcTerminal(t, c.db, pcLearner2, &c.orgID, models.StateRunning, `{"network":true}`)
	setNetworkDisabledColumn(t, c.db, cut, true)

	rows := readControls(t, c, member(pcManager))
	sessions := map[string]pcSessionView{}
	for _, s := range rows[pcLearner2].Sessions {
		sessions[s.SessionID] = s
	}
	require.Contains(t, sessions, withNetwork)
	require.Contains(t, sessions, cut)
	assert.True(t, sessions[withNetwork].NetworkFeature, "composed with network")
	assert.False(t, sessions[withNetwork].NetworkDisabled)
	assert.True(t, sessions[cut].NetworkFeature)
	assert.True(t, sessions[cut].NetworkDisabled, "network_disabled mirrors Terminal.NetworkDisabled")
	assert.False(t, sessions[c.sessions["learner2"]].NetworkFeature, "composed without network")
}

func TestParticipantControls_ScenarioFieldsOnlyForAnOpenRun(t *testing.T) {
	c := newPCClass(t)
	finished := pcTerminal(t, c.db, pcLearner, &c.orgID, models.StateStopped, "")

	scenario := scenarioModels.Scenario{Name: "pc-scn-" + uuid.NewString()[:8], Title: "Linux basics", InstanceType: "S", OsType: "deb", CreatedByID: pcOwner}
	require.NoError(t, c.db.Create(&scenario).Error)
	openTerminal := c.sessions["learner"]
	openRun := scenarioModels.ScenarioSession{ScenarioID: scenario.ID, UserID: pcLearner, TerminalSessionID: &openTerminal, Status: "active", StartedAt: time.Now()}
	require.NoError(t, c.db.Create(&openRun).Error)
	require.NoError(t, c.db.Create(&scenarioModels.ScenarioSession{ScenarioID: scenario.ID, UserID: pcLearner, TerminalSessionID: &finished, Status: "completed", StartedAt: time.Now().Add(-time.Hour)}).Error)

	rows := readControls(t, c, member(pcManager))
	sessions := map[string]pcSessionView{}
	for _, s := range rows[pcLearner].Sessions {
		sessions[s.SessionID] = s
	}
	require.Contains(t, sessions, openTerminal)
	require.NotNil(t, sessions[openTerminal].ScenarioSessionID, "an open run on the terminal is reported")
	assert.Equal(t, openRun.ID.String(), *sessions[openTerminal].ScenarioSessionID)
	require.NotNil(t, sessions[openTerminal].ScenarioTitle)
	assert.Equal(t, "Linux basics", *sessions[openTerminal].ScenarioTitle)

	require.Contains(t, sessions, finished)
	assert.Nil(t, sessions[finished].ScenarioSessionID, "a finished run is not reported")
	assert.Nil(t, sessions[finished].ScenarioTitle)
}

// countQueries counts the SQL statements run on db until the returned stop is
// called.
func countQueries(t *testing.T, db *gorm.DB) (stop func() int) {
	t.Helper()
	var mu sync.Mutex
	n := 0
	inc := func(*gorm.DB) { mu.Lock(); n++; mu.Unlock() }
	name := "pc:count_" + uuid.NewString()
	require.NoError(t, db.Callback().Query().After("gorm:query").Register(name, inc))
	require.NoError(t, db.Callback().Row().After("gorm:row").Register(name, inc))
	require.NoError(t, db.Callback().Raw().After("gorm:raw").Register(name, inc))
	remove := func() {
		_ = db.Callback().Query().Remove(name)
		_ = db.Callback().Row().Remove(name)
		_ = db.Callback().Raw().Remove(name)
	}
	t.Cleanup(remove)
	return func() int {
		remove()
		mu.Lock()
		defer mu.Unlock()
		return n
	}
}

func queriesForControls(t *testing.T, c *pcClass) int {
	t.Helper()
	stop := countQueries(t, c.db)
	w := pcDo(t, c.db, member(pcOwner), http.MethodGet, controlsPath(c.group.ID), nil)
	n := stop()
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	return n
}

// The bulk read runs a fixed number of queries whatever the class size: no
// query per participant or per session.
func TestParticipantControls_QueryCountDoesNotGrowWithTheClass(t *testing.T) {
	c := newPCClass(t)
	small := queriesForControls(t, c)

	for i := range 8 {
		learner := fmt.Sprintf("pc-extra-%d", i)
		createTestGroupMember(t, c.db, c.group.ID, learner, groupModels.GroupMemberRoleMember)
		session := pcTerminal(t, c.db, learner, &c.orgID, models.StateRunning, `{"network":true}`)
		pcTerminal(t, c.db, learner, &c.orgID, models.StateStopped, "")
		blockSessions(t, c.db, learner, models.SessionBlockScopeGroup, &c.group.ID, "r")
		scenario := scenarioModels.Scenario{Name: "pc-q-" + uuid.NewString()[:8], Title: "T", InstanceType: "S", OsType: "deb", CreatedByID: pcOwner}
		require.NoError(t, c.db.Create(&scenario).Error)
		require.NoError(t, c.db.Create(&scenarioModels.ScenarioSession{ScenarioID: scenario.ID, UserID: learner, TerminalSessionID: &session, Status: "active", StartedAt: time.Now()}).Error)
	}
	large := queriesForControls(t, c)

	assert.Equal(t, small, large, "query count must not depend on the number of participants or sessions")
	assert.LessOrEqual(t, large, 10, "the bulk read is a handful of queries")
}

// =============================================================================
// Stop sessions
// =============================================================================

func TestStopParticipantSessions_StopsTheirRunningSessionsInTheClassScopeOnly(t *testing.T) {
	c := newPCClass(t)
	tt := newPCTT(t)
	stopped := pcTerminal(t, c.db, pcLearner, &c.orgID, models.StateStopped, "")
	personal := pcTerminal(t, c.db, pcLearner, nil, models.StateRunning, "")
	elsewhere := pcTerminal(t, c.db, pcLearner, c.otherGroup.OrganizationID, models.StateRunning, "")

	w := pcDo(t, c.db, member(pcManager), http.MethodPost, participantPath(c.group.ID, pcLearner, "stop-sessions"), map[string]any{})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	assert.Equal(t, []string{c.sessions["learner"]}, tt.stopped(), "only the running session of the class scope is stopped on tt-backend")
	assert.Equal(t, models.StateStopped, pcState(t, c.db, c.sessions["learner"]), "the stop goes through the real lifecycle")
	assert.Equal(t, models.StateRunning, pcState(t, c.db, personal), "a personal session is outside the class scope")
	assert.Equal(t, models.StateRunning, pcState(t, c.db, elsewhere), "a session of another organization is outside the class scope")
	assert.Equal(t, models.StateStopped, pcState(t, c.db, stopped))
	assert.Equal(t, models.StateRunning, pcState(t, c.db, c.sessions["learner2"]), "other participants are untouched")

	results := readResults(t, w)
	require.Contains(t, results, c.sessions["learner"])
	assert.True(t, results[c.sessions["learner"]].OK)
}

func TestStopParticipantSessions_GivenSessionIDs_OnlyThoseStopped(t *testing.T) {
	c := newPCClass(t)
	tt := newPCTT(t)
	second := pcTerminal(t, c.db, pcLearner, &c.orgID, models.StateRunning, "")

	w := pcDo(t, c.db, member(pcManager), http.MethodPost, participantPath(c.group.ID, pcLearner, "stop-sessions"),
		map[string]any{"session_ids": []string{second}})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.Equal(t, []string{second}, tt.stopped())
	assert.Equal(t, models.StateRunning, pcState(t, c.db, c.sessions["learner"]))
}

func TestStopParticipantSessions_SessionIDsOutsideTheScope_403NothingStopped(t *testing.T) {
	cases := map[string]func(c *pcClass) string{
		"another participant's session": func(c *pcClass) string { return c.sessions["learner2"] },
		"a session of another class":    func(c *pcClass) string { return c.sessions["other-learner"] },
		"the target's personal session": func(c *pcClass) string { return pcTerminal(t, c.db, pcLearner, nil, models.StateRunning, "") },
		"the target's other-org session": func(c *pcClass) string {
			return pcTerminal(t, c.db, pcLearner, c.otherGroup.OrganizationID, models.StateRunning, "")
		},
		"a session id that does not exist": func(*pcClass) string { return "pc-session-missing" },
	}
	for name, foreign := range cases {
		t.Run(name, func(t *testing.T) {
			c := newPCClass(t)
			tt := newPCTT(t)
			w := pcDo(t, c.db, member(pcManager), http.MethodPost, participantPath(c.group.ID, pcLearner, "stop-sessions"),
				map[string]any{"session_ids": []string{c.sessions["learner"], foreign(c)}})
			assert.Equal(t, http.StatusForbidden, w.Code, "body=%s", w.Body.String())
			assert.Empty(t, tt.stopped(), "one foreign id refuses the whole call: nothing is stopped")
			assert.Equal(t, models.StateRunning, pcState(t, c.db, c.sessions["learner"]))
		})
	}
}

func TestStopParticipantSessions_AuditsEachStop(t *testing.T) {
	c := newPCClass(t)
	newPCTT(t)
	second := pcTerminal(t, c.db, pcLearner, &c.orgID, models.StateRunning, "")

	w := pcDo(t, c.db, member(pcManager), http.MethodPost, participantPath(c.group.ID, pcLearner, "stop-sessions"), map[string]any{})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	rows := auditRowsOf(t, c.db, auditStoppedByTrainer)
	require.Len(t, rows, 2, "one audit row per stopped session")
	for _, sid := range []string{c.sessions["learner"], second} {
		assert.True(t, auditMentionsSession(rows, sid), "an audit row names session %s", sid)
	}
	for _, row := range rows {
		assert.Contains(t, row.Metadata, pcManager, "audit names the actor")
		assert.Contains(t, row.Metadata, pcLearner, "audit names the target")
		assert.Contains(t, row.Metadata, c.group.ID.String(), "audit names the class")
	}
}

func auditMentionsSession(rows []auditModels.AuditLog, sessionID string) bool {
	for _, row := range rows {
		if row.SessionID == sessionID || strings.Contains(row.Metadata, sessionID) {
			return true
		}
	}
	return false
}

func TestStopParticipantSessions_AuditWriteFails_NothingStopped(t *testing.T) {
	c := newPCClass(t)
	tt := newPCTT(t)
	failAuditWrites(t, c.db)

	w := pcDo(t, c.db, member(pcManager), http.MethodPost, participantPath(c.group.ID, pcLearner, "stop-sessions"), map[string]any{})
	assert.Equal(t, http.StatusInternalServerError, w.Code, "body=%s", w.Body.String())
	assert.Empty(t, tt.stopped(), "audit is written before acting: no audit, no stop")
	assert.Equal(t, models.StateRunning, pcState(t, c.db, c.sessions["learner"]))
}

func TestStopParticipantSessions_Impersonated_AuditNamesTheImpersonator(t *testing.T) {
	c := newPCClass(t)
	newPCTT(t)
	caller := pcCaller{userID: pcManager, roles: []string{"member"}, impersonator: "pc-support-admin"}

	w := pcDo(t, c.db, caller, http.MethodPost, participantPath(c.group.ID, pcLearner, "stop-sessions"), map[string]any{})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	rows := auditRowsOf(t, c.db, auditStoppedByTrainer)
	require.Len(t, rows, 1)
	assert.Contains(t, rows[0].Metadata, "pc-support-admin", "audit metadata carries impersonatorId")
}

// =============================================================================
// Internet access
// =============================================================================

func TestSetParticipantNetwork_Disable_CallsTTWithTheAdminKeyAndRecordsIt(t *testing.T) {
	c := newPCClass(t)
	tt := newPCTT(t)
	networked := pcTerminal(t, c.db, pcLearner, &c.orgID, models.StateRunning, `{"network":true}`)
	seedExposedPort(t, c.db, networked)
	seedExposedPort(t, c.db, networked)
	plain := c.sessions["learner"]

	w := pcDo(t, c.db, member(pcManager), http.MethodPut, participantPath(c.group.ID, pcLearner, "network"), map[string]any{"enabled": false})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	calls := tt.toggles()
	require.Len(t, calls, 1, "only the session composed with network is toggled")
	assert.Equal(t, networked, calls[0].sessionID)
	assert.Equal(t, "test-admin-key", calls[0].apiKey, "the toggle is an admin endpoint: TERMINAL_TRAINER_ADMIN_KEY")
	require.NotNil(t, calls[0].enabled)
	assert.False(t, *calls[0].enabled)

	assert.True(t, networkDisabledColumn(t, c.db, networked))
	assert.Equal(t, int64(0), exposedPortCount(t, c.db, networked), "cutting the network drops the session's exposed ports at once")

	results := readResults(t, w)
	require.Contains(t, results, networked)
	assert.True(t, results[networked].OK)
	require.Contains(t, results, plain, "a session without the network feature is reported")
	assert.True(t, results[plain].Skipped, "a session without the network feature is skipped, not an error")

	rows := auditRowsOf(t, c.db, auditNetworkDisabled)
	require.Len(t, rows, 1)
	assert.True(t, auditMentionsSession(rows, networked))
	assert.Contains(t, rows[0].Metadata, pcManager)
	assert.Contains(t, rows[0].Metadata, pcLearner)
	assert.Contains(t, rows[0].Metadata, c.group.ID.String())
}

func TestSetParticipantNetwork_Enable_ClearsTheFlagAndDropsPorts(t *testing.T) {
	c := newPCClass(t)
	tt := newPCTT(t)
	networked := pcTerminal(t, c.db, pcLearner, &c.orgID, models.StateRunning, `{"network":true}`)
	setNetworkDisabledColumn(t, c.db, networked, true)
	seedExposedPort(t, c.db, networked)

	w := pcDo(t, c.db, member(pcManager), http.MethodPut, participantPath(c.group.ID, pcLearner, "network"),
		map[string]any{"enabled": true, "session_ids": []string{networked}})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	calls := tt.toggles()
	require.Len(t, calls, 1)
	require.NotNil(t, calls[0].enabled)
	assert.True(t, *calls[0].enabled)
	assert.False(t, networkDisabledColumn(t, c.db, networked))
	assert.Equal(t, int64(0), exposedPortCount(t, c.db, networked), "the IP changes on re-attach: old exposures are dropped")
	assert.Len(t, auditRowsOf(t, c.db, auditNetworkEnabled), 1)
}

func TestSetParticipantNetwork_TTErrorCodes_MappedWithTheirCode(t *testing.T) {
	cases := []struct {
		code     string
		ttStatus int
		want     int
	}{
		{"feature_not_in_session", http.StatusConflict, http.StatusConflict},
		{"session_not_running", http.StatusConflict, http.StatusConflict},
		{"vm_not_supported", http.StatusConflict, http.StatusConflict},
		{"network_renew_failed", http.StatusBadGateway, http.StatusBadGateway},
		{"network_still_present", http.StatusInternalServerError, http.StatusInternalServerError},
		{"state_not_recorded", http.StatusInternalServerError, http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			c := newPCClass(t)
			tt := newPCTT(t)
			networked := pcTerminal(t, c.db, pcLearner, &c.orgID, models.StateRunning, `{"network":true}`)
			tt.networkReply[networked] = struct {
				status int
				code   string
			}{tc.ttStatus, tc.code}

			w := pcDo(t, c.db, member(pcManager), http.MethodPut, participantPath(c.group.ID, pcLearner, "network"),
				map[string]any{"enabled": false, "session_ids": []string{networked}})

			assert.Equal(t, tc.want, w.Code, "body=%s", w.Body.String())
			body := decodeBody(t, w)
			assert.Equal(t, tc.code, body["code"], "tt-backend's code is kept for the trainer's message")
			results := readResults(t, w)
			require.Contains(t, results, networked)
			assert.False(t, results[networked].OK)
			assert.Equal(t, tc.code, results[networked].Code)
			assert.False(t, networkDisabledColumn(t, c.db, networked), "a failed toggle records nothing")
		})
	}
}

func TestSetParticipantNetwork_SessionIDsOutsideTheScope_403NoToggle(t *testing.T) {
	c := newPCClass(t)
	tt := newPCTT(t)
	mine := pcTerminal(t, c.db, pcLearner, &c.orgID, models.StateRunning, `{"network":true}`)
	theirs := pcTerminal(t, c.db, pcLearner2, &c.orgID, models.StateRunning, `{"network":true}`)

	w := pcDo(t, c.db, member(pcManager), http.MethodPut, participantPath(c.group.ID, pcLearner, "network"),
		map[string]any{"enabled": false, "session_ids": []string{mine, theirs}})
	assert.Equal(t, http.StatusForbidden, w.Code, "body=%s", w.Body.String())
	assert.Empty(t, tt.toggles())
}

func TestSetParticipantNetwork_MissingEnabled_400(t *testing.T) {
	c := newPCClass(t)
	tt := newPCTT(t)
	pcTerminal(t, c.db, pcLearner, &c.orgID, models.StateRunning, `{"network":true}`)

	w := pcDo(t, c.db, member(pcManager), http.MethodPut, participantPath(c.group.ID, pcLearner, "network"), map[string]any{})
	assert.Equal(t, http.StatusBadRequest, w.Code, "a body without enabled must not read as \"off\"")
	assert.Empty(t, tt.toggles())
}

func TestSetParticipantNetwork_AuditWriteFails_NoToggle(t *testing.T) {
	c := newPCClass(t)
	tt := newPCTT(t)
	networked := pcTerminal(t, c.db, pcLearner, &c.orgID, models.StateRunning, `{"network":true}`)
	failAuditWrites(t, c.db)

	w := pcDo(t, c.db, member(pcManager), http.MethodPut, participantPath(c.group.ID, pcLearner, "network"),
		map[string]any{"enabled": false, "session_ids": []string{networked}})
	assert.Equal(t, http.StatusInternalServerError, w.Code, "body=%s", w.Body.String())
	assert.Empty(t, tt.toggles(), "audit is written before acting: no audit, no toggle")
	assert.False(t, networkDisabledColumn(t, c.db, networked))
}

// =============================================================================
// Session block in the class
// =============================================================================

func TestSetParticipantSessionBlock_BlockUpdateUnblock(t *testing.T) {
	c := newPCClass(t)
	newPCTT(t)
	path := participantPath(c.group.ID, pcLearner, "session-block")

	w := pcDo(t, c.db, member(pcManager), http.MethodPut, path, map[string]any{"blocked": true, "reason": "first"})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	rows := classBlockRows(t, c.db, pcLearner, c.group.ID)
	require.Len(t, rows, 1)
	assert.Equal(t, "first", rows[0].Reason)
	assert.Equal(t, pcManager, rows[0].BlockedBy)

	w = pcDo(t, c.db, member(pcManager), http.MethodPut, path, map[string]any{"blocked": true, "reason": "second"})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	rows = classBlockRows(t, c.db, pcLearner, c.group.ID)
	require.Len(t, rows, 1, "blocking again updates the block, it does not add one")
	assert.Equal(t, "second", rows[0].Reason)

	w = pcDo(t, c.db, member(pcManager), http.MethodPut, path, map[string]any{"blocked": false})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.Empty(t, classBlockRows(t, c.db, pcLearner, c.group.ID), "unblocking deletes the block")

	set := auditRowsOf(t, c.db, auditSessionBlockSet)
	require.Len(t, set, 2)
	assert.Contains(t, set[0].Metadata+set[1].Metadata, "second", "audit records the reason")
	for _, row := range set {
		assert.Contains(t, row.Metadata, pcManager)
		assert.Contains(t, row.Metadata, pcLearner)
		assert.Contains(t, row.Metadata, c.group.ID.String())
		assert.Contains(t, row.Metadata, models.SessionBlockScopeGroup, "audit records the scope")
	}
	assert.Len(t, auditRowsOf(t, c.db, auditSessionBlockClear), 1)
}

func TestSetParticipantSessionBlock_ReasonOver500Chars_400(t *testing.T) {
	c := newPCClass(t)
	newPCTT(t)
	w := pcDo(t, c.db, member(pcManager), http.MethodPut, participantPath(c.group.ID, pcLearner, "session-block"),
		map[string]any{"blocked": true, "reason": strings.Repeat("x", 501)})
	assert.Equal(t, http.StatusBadRequest, w.Code, "body=%s", w.Body.String())
	assert.Empty(t, classBlockRows(t, c.db, pcLearner, c.group.ID))

	w = pcDo(t, c.db, member(pcManager), http.MethodPut, participantPath(c.group.ID, pcLearner, "session-block"),
		map[string]any{"blocked": true, "reason": strings.Repeat("x", 500)})
	assert.Equal(t, http.StatusOK, w.Code, "500 characters is the limit, not over it")
}

func TestSetParticipantSessionBlock_WithoutStopRunning_SessionsKeepRunning(t *testing.T) {
	c := newPCClass(t)
	tt := newPCTT(t)
	w := pcDo(t, c.db, member(pcManager), http.MethodPut, participantPath(c.group.ID, pcLearner, "session-block"),
		map[string]any{"blocked": true, "reason": "r"})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.Empty(t, tt.stopped(), "stop_running defaults to false at API level")
	assert.Equal(t, models.StateRunning, pcState(t, c.db, c.sessions["learner"]))
}

func TestSetParticipantSessionBlock_StopRunning_StopsTheClassScopeOnly(t *testing.T) {
	c := newPCClass(t)
	tt := newPCTT(t)
	elsewhere := pcTerminal(t, c.db, pcLearner, c.otherGroup.OrganizationID, models.StateRunning, "")

	w := pcDo(t, c.db, member(pcManager), http.MethodPut, participantPath(c.group.ID, pcLearner, "session-block"),
		map[string]any{"blocked": true, "reason": "r", "stop_running": true})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.Equal(t, []string{c.sessions["learner"]}, tt.stopped())
	assert.Equal(t, models.StateStopped, pcState(t, c.db, c.sessions["learner"]))
	assert.Equal(t, models.StateRunning, pcState(t, c.db, elsewhere))
	assert.Equal(t, models.StateRunning, pcState(t, c.db, c.sessions["learner2"]))
}

func TestSetParticipantSessionBlock_AuditWriteFails_NoBlockNoStop(t *testing.T) {
	c := newPCClass(t)
	tt := newPCTT(t)
	failAuditWrites(t, c.db)

	w := pcDo(t, c.db, member(pcManager), http.MethodPut, participantPath(c.group.ID, pcLearner, "session-block"),
		map[string]any{"blocked": true, "reason": "r", "stop_running": true})
	assert.Equal(t, http.StatusInternalServerError, w.Code, "body=%s", w.Body.String())
	assert.Empty(t, classBlockRows(t, c.db, pcLearner, c.group.ID), "audit is written before acting: no audit, no block")
	assert.Empty(t, tt.stopped())
}

func TestSetParticipantSessionBlock_Impersonated_AuditNamesTheImpersonator(t *testing.T) {
	c := newPCClass(t)
	newPCTT(t)
	caller := pcCaller{userID: pcManager, roles: []string{"member"}, impersonator: "pc-support-admin"}
	w := pcDo(t, c.db, caller, http.MethodPut, participantPath(c.group.ID, pcLearner, "session-block"),
		map[string]any{"blocked": true, "reason": "r"})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	rows := auditRowsOf(t, c.db, auditSessionBlockSet)
	require.Len(t, rows, 1)
	assert.Contains(t, rows[0].Metadata, "pc-support-admin")
}

// =============================================================================
// Sync and schema
// =============================================================================

func TestSyncUserSessions_PropagatesNetworkDisabled(t *testing.T) {
	userID := "pc-sync-user"
	sessionID := "pc-sync-session"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/sessions") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sessions": []map[string]any{{
					"id": sessionID, "session_id": sessionID, "status": 0, "state": "running",
					"persistence_mode": "persistent", "expires_at": time.Now().Add(time.Hour).Unix(),
					"composed_features": `{"network":true}`, "network_disabled": true,
				}},
				"count": 1, "include_expired": true, "limit": 1000,
			})
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	configureTTServer(t, srv.URL)

	db := freshTestDB(t)
	key, err := createTestUserKey(db, userID)
	require.NoError(t, err)
	require.NoError(t, db.Create(&models.Terminal{
		SessionID: sessionID, UserID: userID, State: models.StateRunning, PersistenceMode: "persistent",
		ExpiresAt: time.Now().Add(time.Hour), MachineSize: "S", ComposedFeatures: `{"network":true}`, UserTerminalKeyID: key.ID,
	}).Error)
	require.False(t, networkDisabledColumn(t, db, sessionID), "Terminal.NetworkDisabled defaults to false")

	_, err = services.NewTerminalTrainerService(db).SyncUserSessions(userID)
	require.NoError(t, err)
	assert.True(t, networkDisabledColumn(t, db, sessionID), "a sync mirrors tt-backend's network_disabled")
}

func TestScenarioSessionTerminalSessionID_Indexed(t *testing.T) {
	assert.True(t, sharedTestDB.Migrator().HasIndex(&scenarioModels.ScenarioSession{}, "idx_scenario_sessions_terminal_session_id"),
		"the bulk read and the start path look runs up by terminal_session_id")
}

func keysOf[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
