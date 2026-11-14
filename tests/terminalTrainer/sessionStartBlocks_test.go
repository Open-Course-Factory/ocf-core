package terminalTrainer_tests

// Session start blocks (#579): a participant can be blocked from starting
// sessions in one class, in one organisation, or on the whole platform. These
// tests pin the lookup, every terminal-trainer entry point that starts or
// resumes a machine, and the HTTP answer the learner receives.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	groupModels "soli/formations/src/groups/models"
	paymentMiddleware "soli/formations/src/payment/middleware"
	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/terminalTrainer/models"
	terminalController "soli/formations/src/terminalTrainer/routes"
	"soli/formations/src/terminalTrainer/services"
	testTools "soli/formations/tests/testTools"
)

func blockSessions(t *testing.T, db *gorm.DB, userID, scopeType string, scopeID *uuid.UUID, reason string) {
	t.Helper()
	require.NoError(t, testTools.SeedSessionBlock(db, userID, scopeType, scopeID, reason))
}

func uuidPtr(id uuid.UUID) *uuid.UUID { return &id }

// requireSessionStartBlocked asserts err is the session-start refusal for scope.
func requireSessionStartBlocked(t *testing.T, err error, scope, reason string) {
	t.Helper()
	require.ErrorIs(t, err, models.ErrSessionStartBlocked)
	var blocked *models.SessionStartBlockedError
	require.ErrorAs(t, err, &blocked)
	assert.Equal(t, scope, blocked.Scope)
	assert.Equal(t, reason, blocked.Reason)
}

// ---------------------------------------------------------------------------
// Lookup
// ---------------------------------------------------------------------------

func TestSessionStartBlockedError_IsTheSentinelWithItsPublicCode(t *testing.T) {
	err := error(&models.SessionStartBlockedError{Scope: "org", Reason: "absences"})
	assert.ErrorIs(t, err, models.ErrSessionStartBlocked)
	assert.Equal(t, "session_start_blocked", err.Error())
	assert.Equal(t, "session_start_blocked", models.ErrSessionStartBlocked.Error())
}

func TestIsSessionStartBlocked_TruthTable(t *testing.T) {
	orgA, orgB, group := uuid.New(), uuid.New(), uuid.New()
	type block struct {
		scope  string
		id     *uuid.UUID
		reason string
	}
	type check struct {
		org         *uuid.UUID
		wantBlocked bool
		wantScope   string
		wantReason  string
	}
	cases := []struct {
		name   string
		blocks []block
		checks []check
	}{
		{"no block", nil, []check{
			{org: &orgA}, {org: nil},
		}},
		{"org block matches only its organisation", []block{{"org", &orgA, "absences"}}, []check{
			{org: &orgA, wantBlocked: true, wantScope: "org", wantReason: "absences"},
			{org: &orgB},
			{org: nil}, // the learner's personal context is not that organisation
		}},
		{"global block matches every context", []block{{"global", nil, "fraud"}}, []check{
			{org: &orgA, wantBlocked: true, wantScope: "global", wantReason: "fraud"},
			{org: nil, wantBlocked: true, wantScope: "global", wantReason: "fraud"},
		}},
		{"global wins over org", []block{{"org", &orgA, "absences"}, {"global", nil, "fraud"}}, []check{
			{org: &orgA, wantBlocked: true, wantScope: "global", wantReason: "fraud"},
		}},
		{"class block does not refuse a start", []block{{"group", &group, "late"}}, []check{
			{org: &orgA}, {org: nil},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := freshTestDB(t)
			userID := "blk-truth-" + uuid.NewString()
			for _, b := range tc.blocks {
				blockSessions(t, db, userID, b.scope, b.id, b.reason)
			}
			// Another user's block never leaks.
			blockSessions(t, db, "someone-else-"+uuid.NewString(), "global", nil, "other")

			for _, c := range tc.checks {
				blocked, scope, reason, err := models.IsSessionStartBlocked(db, userID, c.org)
				require.NoError(t, err)
				assert.Equal(t, c.wantBlocked, blocked, "org=%v", c.org)
				assert.Equal(t, c.wantScope, scope, "org=%v", c.org)
				assert.Equal(t, c.wantReason, reason, "org=%v", c.org)
			}
		})
	}
}

func TestIsClassBlocked_OnlyThatClass(t *testing.T) {
	db := freshTestDB(t)
	userID := "blk-class-" + uuid.NewString()
	classA, classB, org := uuid.New(), uuid.New(), uuid.New()
	blockSessions(t, db, userID, "group", &classA, "late")
	blockSessions(t, db, userID, "org", &org, "")

	blocked, err := models.IsClassBlocked(db, userID, classA)
	require.NoError(t, err)
	assert.True(t, blocked, "blocked in class A")

	blocked, err = models.IsClassBlocked(db, userID, classB)
	require.NoError(t, err)
	assert.False(t, blocked, "a block in class A says nothing about class B")

	blocked, err = models.IsClassBlocked(db, "other-"+uuid.NewString(), classA)
	require.NoError(t, err)
	assert.False(t, blocked, "another learner of class A is not blocked")
}

// ---------------------------------------------------------------------------
// StartComposedSession — composed start, and every path built on it
// ---------------------------------------------------------------------------

func composedStartFixture(t *testing.T) (*gorm.DB, string, *paymentModels.SubscriptionPlan) {
	t.Helper()
	ttServer := startComposedTTBackendStub(t)
	t.Cleanup(ttServer.Close)
	configureTTServer(t, ttServer.URL)
	db := freshTestDB(t)
	userID := "blk-composed-" + uuid.NewString()
	_, err := createTestUserKey(db, userID)
	require.NoError(t, err)
	plan := &paymentModels.SubscriptionPlan{Name: "BlockPlan", IsActive: true, MaxCPU: 8000, MaxMemoryMB: 4096, MaxSessionDurationMinutes: 60}
	require.NoError(t, db.Create(plan).Error)
	return db, userID, plan
}

func composedInput(orgID *uuid.UUID) dto.CreateComposedSessionInput {
	in := dto.CreateComposedSessionInput{Distribution: "ubuntu-24.04", Size: "XS", Terms: "accepted"}
	if orgID != nil {
		in.OrganizationID = orgID.String()
	}
	return in
}

func terminalCount(db *gorm.DB, userID string) int64 {
	var n int64
	db.Model(&models.Terminal{}).Where("user_id = ?", userID).Count(&n)
	return n
}

func TestStartComposedSession_GlobalBlock_RefusedAnywhere(t *testing.T) {
	db, userID, plan := composedStartFixture(t)
	blockSessions(t, db, userID, "global", nil, "fraud")
	svc := services.NewTerminalTrainerService(db)

	_, err := svc.StartComposedSession(userID, composedInput(nil), plan)
	requireSessionStartBlocked(t, err, "global", "fraud")

	_, err = svc.StartComposedSession(userID, composedInput(uuidPtr(uuid.New())), plan)
	requireSessionStartBlocked(t, err, "global", "fraud")

	assert.Zero(t, terminalCount(db, userID), "no terminal row for a refused start")
}

func TestStartComposedSession_OrgBlock_RefusesThatOrganisationOnly(t *testing.T) {
	db, userID, plan := composedStartFixture(t)
	orgA := uuid.New()
	blockSessions(t, db, userID, "org", &orgA, "absences")
	svc := services.NewTerminalTrainerService(db)

	_, err := svc.StartComposedSession(userID, composedInput(&orgA), plan)
	requireSessionStartBlocked(t, err, "org", "absences")
	assert.Zero(t, terminalCount(db, userID))

	_, err = svc.StartComposedSession(userID, composedInput(nil), plan)
	require.NoError(t, err, "an org block does not touch the learner's personal context")
	assert.EqualValues(t, 1, terminalCount(db, userID))
}

func TestStartComposedSession_OrgBlockElsewhere_DoesNotRefuse(t *testing.T) {
	db, userID, plan := composedStartFixture(t)
	orgA, orgB := uuid.New(), uuid.New()
	blockSessions(t, db, userID, "org", &orgA, "")

	_, err := services.NewTerminalTrainerService(db).StartComposedSession(userID, composedInput(&orgB), plan)
	assert.NotErrorIs(t, err, models.ErrSessionStartBlocked, "a block in org A says nothing about org B")
}

func TestStartComposedSession_ClassBlock_DoesNotRefuseAStart(t *testing.T) {
	db, userID, plan := composedStartFixture(t)
	class := uuid.New()
	blockSessions(t, db, userID, "group", &class, "")

	_, err := services.NewTerminalTrainerService(db).StartComposedSession(userID, composedInput(nil), plan)
	require.NoError(t, err, "a class block only acts on that class's assignments and bulk starts")
}

// ---------------------------------------------------------------------------
// StartSession — resume
// ---------------------------------------------------------------------------

func seedStoppedTerminal(t *testing.T, db *gorm.DB, userID string, orgID *uuid.UUID) string {
	t.Helper()
	userKey, err := createTestUserKey(db, userID)
	require.NoError(t, err)
	sessionID := "blk-resume-" + uuid.NewString()
	require.NoError(t, db.Create(&models.Terminal{
		SessionID:         sessionID,
		UserID:            userID,
		Name:              "blk-resume",
		State:             models.StateStopped,
		PersistenceMode:   models.PersistenceModePersistent,
		ExpiresAt:         time.Now().Add(time.Hour),
		InstanceType:      "test",
		MachineSize:       "S",
		OrganizationID:    orgID,
		UserTerminalKeyID: userKey.ID,
	}).Error)
	return sessionID
}

func TestStartSession_BlockOfTheTerminalScope_RefusesResume(t *testing.T) {
	orgA := uuid.New()
	cases := []struct {
		name        string
		terminalOrg *uuid.UUID
		blockScope  string
		blockID     *uuid.UUID
	}{
		{"org block, terminal in that org", &orgA, "org", &orgA},
		{"global block, personal terminal", nil, "global", nil},
		{"global block, org terminal", &orgA, "global", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, rec := startSessionTTServer(t)
			defer srv.Close()
			configureTTServer(t, srv.URL)
			db := freshTestDB(t)
			userID := "blk-resume-" + uuid.NewString()
			sessionID := seedStoppedTerminal(t, db, userID, tc.terminalOrg)
			blockSessions(t, db, userID, tc.blockScope, tc.blockID, "paused by trainer")

			err := services.NewTerminalTrainerService(db).StartSession(sessionID, roomyResumePlan())

			requireSessionStartBlocked(t, err, tc.blockScope, "paused by trainer")
			assert.Zero(t, rec.calls, "tt-backend is never asked to start the container")
			var row models.Terminal
			require.NoError(t, db.First(&row, "session_id = ?", sessionID).Error)
			assert.Equal(t, models.StateStopped, row.State, "the row stays stopped")
		})
	}
}

func TestStartSession_BlockOfAnotherScope_ResumeProceeds(t *testing.T) {
	orgA, orgB, class := uuid.New(), uuid.New(), uuid.New()
	cases := []struct {
		name        string
		terminalOrg *uuid.UUID
		blockScope  string
		blockID     *uuid.UUID
	}{
		{"org block, terminal in another org", &orgB, "org", &orgA},
		{"org block, personal terminal", nil, "org", &orgA},
		{"class block", &orgA, "group", &class},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, rec := startSessionTTServer(t)
			defer srv.Close()
			configureTTServer(t, srv.URL)
			db := freshTestDB(t)
			userID := "blk-resume-ok-" + uuid.NewString()
			sessionID := seedStoppedTerminal(t, db, userID, tc.terminalOrg)
			blockSessions(t, db, userID, tc.blockScope, tc.blockID, "")

			require.NoError(t, services.NewTerminalTrainerService(db).StartSession(sessionID, roomyResumePlan()))
			assert.Equal(t, 1, rec.calls)
		})
	}
}

// ---------------------------------------------------------------------------
// CreateUserKey — a platform block disables the learner's terminal key
// ---------------------------------------------------------------------------

func TestCreateUserKey_GlobalBlock_Refused(t *testing.T) {
	srv, creates := keyCountingTTServer(t)
	configureTTServer(t, srv.URL)
	db := freshTestDB(t)
	userID := "blk-key-" + uuid.NewString()
	blockSessions(t, db, userID, "global", nil, "fraud")

	err := services.NewTerminalTrainerService(db).CreateUserKey(userID, "auto-blocked")

	requireSessionStartBlocked(t, err, "global", "fraud")
	assert.Zero(t, creates.Load(), "tt-backend never asked for a key")
}

func TestCreateUserKey_OrgOrClassBlock_KeyStillCreated(t *testing.T) {
	srv, creates := keyCountingTTServer(t)
	configureTTServer(t, srv.URL)
	db := freshTestDB(t)
	userID := "blk-key-ok-" + uuid.NewString()
	blockSessions(t, db, userID, "org", uuidPtr(uuid.New()), "")
	blockSessions(t, db, userID, "group", uuidPtr(uuid.New()), "")

	require.NoError(t, services.NewTerminalTrainerService(db).CreateUserKey(userID, "auto-ok"),
		"the key serves every context; only a platform block disables it")
	assert.EqualValues(t, 1, creates.Load())
}

// ---------------------------------------------------------------------------
// Class bulk-create — skip and report the blocked members
// ---------------------------------------------------------------------------

func bulkCreateWithBlockedMember(t *testing.T, blockScope string, orgID *uuid.UUID) (blockedID, okID string, results map[string]dto.BulkTerminalCreationResult) {
	t.Helper()
	srv, _ := keyCountingTTServer(t)
	configureTTServer(t, srv.URL)
	casdoorsdk.InitConfig("http://localhost:0", "dummy-endpoint", "dummy-client", "dummy-secret", "dummy-org", "dummy-app")
	db := freshTestDB(t)
	ownerID := "blk-bulk-owner-" + uuid.NewString()
	group := createTestGroup(t, db, ownerID)
	blockedID, okID = "blk-bulk-ko-"+uuid.NewString(), "blk-bulk-ok-"+uuid.NewString()
	addActiveGroupMember(t, db, group.ID, blockedID, groupModels.GroupMemberRoleMember)
	addActiveGroupMember(t, db, group.ID, okID, groupModels.GroupMemberRoleMember)
	switch blockScope {
	case "group":
		blockSessions(t, db, blockedID, "group", &group.ID, "late")
	case "org":
		blockSessions(t, db, blockedID, "org", orgID, "late")
	case "global":
		blockSessions(t, db, blockedID, "global", nil, "fraud")
	}
	plan := &paymentModels.SubscriptionPlan{Name: "BulkBlock", IsActive: true, MaxCPU: 100000, MaxMemoryMB: 51200, MaxSessionDurationMinutes: 60}
	require.NoError(t, db.Create(plan).Error)

	req := dto.BulkCreateTerminalsRequest{Terms: "accepted", InstanceType: "ubuntu-24.04"}
	if orgID != nil {
		req.OrganizationID = orgID.String()
	}
	resp, err := services.NewTerminalTrainerService(db).BulkCreateTerminalsForGroup(
		group.ID.String(), ownerID, []string{"member"}, req, plan)
	require.NoError(t, err)

	results = map[string]dto.BulkTerminalCreationResult{}
	for _, r := range resp.Terminals {
		results[r.UserID] = r
	}
	require.Len(t, results, 2, "both members are reported")
	return blockedID, okID, results
}

func TestBulkCreateTerminals_ClassBlockedMember_SkippedAndReported(t *testing.T) {
	blockedID, okID, results := bulkCreateWithBlockedMember(t, "group", nil)

	assert.False(t, results[blockedID].Success)
	assert.Equal(t, "session_start_blocked", results[blockedID].Error)
	assert.NotEqual(t, "session_start_blocked", results[okID].Error, "the other member is still attempted")
}

func TestBulkCreateTerminals_OrgBlockedMember_SkippedAndReported(t *testing.T) {
	orgID := uuid.New()
	blockedID, okID, results := bulkCreateWithBlockedMember(t, "org", &orgID)

	assert.False(t, results[blockedID].Success)
	assert.Equal(t, "session_start_blocked", results[blockedID].Error)
	assert.NotEqual(t, "session_start_blocked", results[okID].Error, "the other member is still attempted")
}

func TestBulkCreateTerminals_GloballyBlockedMember_SkippedAndReported(t *testing.T) {
	blockedID, okID, results := bulkCreateWithBlockedMember(t, "global", nil)

	assert.False(t, results[blockedID].Success)
	assert.Equal(t, "session_start_blocked", results[blockedID].Error)
	assert.NotEqual(t, "session_start_blocked", results[okID].Error, "the other member is still attempted")
}

// ---------------------------------------------------------------------------
// Schema — one global block per user
// ---------------------------------------------------------------------------

// The production schema (model + MigrateSessionBlockGlobalIndex), not the test
// DDL: the composite unique index alone lets NULL scope_ids repeat.
func TestSessionBlockSchema_SecondGlobalBlockForSameUser_Refused(t *testing.T) {
	db := testTools.MemoryDB(&models.SessionBlock{})
	require.NoError(t, models.MigrateSessionBlockGlobalIndex(db))
	userID := "blk-global-" + uuid.NewString()

	require.NoError(t, db.Create(&models.SessionBlock{UserID: userID, ScopeType: models.SessionBlockScopeGlobal, Reason: "fraud"}).Error)
	assert.Error(t, db.Create(&models.SessionBlock{UserID: userID, ScopeType: models.SessionBlockScopeGlobal, Reason: "again"}).Error,
		"a second global block for the same user is refused")

	assert.NoError(t, db.Create(&models.SessionBlock{UserID: userID, ScopeType: models.SessionBlockScopeOrg, ScopeID: uuidPtr(uuid.New())}).Error,
		"an organisation block alongside the global one is allowed")
	assert.NoError(t, db.Create(&models.SessionBlock{UserID: "blk-global-other-" + uuid.NewString(), ScopeType: models.SessionBlockScopeGlobal}).Error,
		"another user's global block is allowed")
}

// ---------------------------------------------------------------------------
// HTTP — 403 session_start_blocked, with scope and reason
// ---------------------------------------------------------------------------

func decodeBody(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var payload map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload), w.Body.String())
	return payload
}

func TestStartComposedSessionRoute_Blocked_403WithScopeAndReason(t *testing.T) {
	svc := &mockTerminalTrainerService{}
	svc.On("StartComposedSession", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, &models.SessionStartBlockedError{Scope: "org", Reason: "absences répétées"})
	router := setupComposedSessionRouter(svc, defaultTestPlan())

	req := httptest.NewRequest(http.MethodPost, "/terminals/start-composed-session", bytes.NewReader(validComposedSessionBody()))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	payload := decodeBody(t, w)
	assert.Equal(t, "session_start_blocked", payload["error_message"])
	assert.Equal(t, "org", payload["scope"])
	assert.Equal(t, "absences répétées", payload["reason"], "the learner sees why")
}

func TestStartSessionRoute_GlobalBlock_403SessionStartBlocked(t *testing.T) {
	db := freshTestDB(t)
	userID := "blk-resume-route-" + uuid.NewString()
	seedPersonalPlanFor(t, userID, 64000, 65536, 60, "Roomy")
	sessionID := seedStoppedTerminal(t, db, userID, nil)
	blockSessions(t, db, userID, "global", nil, "fraud")

	var starts atomic.Int32
	ttSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/metrics":
			_ = json.NewEncoder(w).Encode(map[string]any{"ram_available_gb": 32.0, "ram_percent": 10.0})
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/sessions":
			_ = json.NewEncoder(w).Encode(map[string]any{"sessions": []map[string]any{
				{"id": sessionID, "session_id": sessionID, "status": 1, "state": "stopped", "persistence_mode": "persistent", "expires_at": time.Now().Add(time.Hour).Unix()},
			}})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/start"):
			starts.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"state": "running", "expires_at": time.Now().Add(time.Hour).Unix()})
		default:
			http.Error(w, "unexpected: "+r.Method+" "+r.URL.Path, http.StatusNotFound)
		}
	}))
	t.Cleanup(ttSrv.Close)
	configureTTServer(t, ttSrv.URL)

	router := setupResumeRouterWithProdMiddleware(t, userID, services.NewTerminalTrainerService(sharedTestDB))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/terminals/"+sessionID+"/start", nil))

	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	payload := decodeBody(t, w)
	assert.Equal(t, "session_start_blocked", payload["error_message"])
	assert.Equal(t, "global", payload["scope"])
	assert.Equal(t, "fraud", payload["reason"])
	assert.Zero(t, starts.Load())
}

// ---------------------------------------------------------------------------
// Organization context — the query string and the body name the same org
// ---------------------------------------------------------------------------

// setupComposedRouteWithOrgContext wires POST /terminals/start-composed-session
// with the production InjectOrgContext and the real service; the plan is
// injected directly, as InjectEffectivePlan + RequirePlan would.
func setupComposedRouteWithOrgContext(db *gorm.DB, userID string, plan *paymentModels.SubscriptionPlan) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Set("userRoles", []string{"member"})
		c.Next()
	})
	ctrl := terminalController.NewTerminalControllerWithService(db, services.NewTerminalTrainerService(db))
	router.POST("/terminals/start-composed-session",
		paymentMiddleware.InjectOrgContext(),
		func(c *gin.Context) {
			c.Set("subscription_plan", plan)
			c.Set("has_active_subscription", true)
			c.Next()
		},
		ctrl.StartComposedSession,
	)
	return router
}

func postComposedWithQueryOrg(router *gin.Engine, queryOrg string, bodyOrg *uuid.UUID) *httptest.ResponseRecorder {
	body, _ := json.Marshal(composedInput(bodyOrg))
	req := httptest.NewRequest(http.MethodPost, "/terminals/start-composed-session?organization_id="+queryOrg, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// The plan chain takes the organization from ?organization_id= first; the
// start must run in that same organization, so its block applies.
func TestStartComposedSessionRoute_OrgBlockedInQueryOrg_NoBodyOrg_403(t *testing.T) {
	db, userID, plan := composedStartFixture(t)
	orgX := uuid.New()
	blockSessions(t, db, userID, "org", &orgX, "absences")

	w := postComposedWithQueryOrg(setupComposedRouteWithOrgContext(db, userID, plan), orgX.String(), nil)

	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	payload := decodeBody(t, w)
	assert.Equal(t, "session_start_blocked", payload["error_message"])
	assert.Equal(t, "org", payload["scope"])
	assert.Equal(t, "absences", payload["reason"])
	assert.Zero(t, terminalCount(db, userID), "no terminal row for a refused start")
}

func TestStartComposedSessionRoute_QueryOrgDiffersFromBodyOrg_400Mismatch(t *testing.T) {
	db, userID, plan := composedStartFixture(t)
	orgX, orgY := uuid.New(), uuid.New()
	blockSessions(t, db, userID, "org", &orgX, "absences")

	w := postComposedWithQueryOrg(setupComposedRouteWithOrgContext(db, userID, plan), orgX.String(), &orgY)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, strings.ToLower(w.Body.String()), "organization mismatch")
	assert.Zero(t, terminalCount(db, userID), "no terminal row for a refused start")
}

// ---------------------------------------------------------------------------
// Class bulk-create — the class's organization when the request names none
// ---------------------------------------------------------------------------

func TestBulkCreateTerminals_OrgClassWithoutRequestOrg_OrgBlockedMemberSkipped(t *testing.T) {
	srv, _ := keyCountingTTServer(t)
	configureTTServer(t, srv.URL)
	casdoorsdk.InitConfig("http://localhost:0", "dummy-endpoint", "dummy-client", "dummy-secret", "dummy-org", "dummy-app")
	db := freshTestDB(t)
	ownerID := "blk-bulk-orgclass-owner-" + uuid.NewString()
	orgX := uuid.New()
	group := createTestGroupForHistory(t, db, ownerID, &orgX)
	blockedID, okID := "blk-bulk-orgclass-ko-"+uuid.NewString(), "blk-bulk-orgclass-ok-"+uuid.NewString()
	addActiveGroupMember(t, db, group.ID, blockedID, groupModels.GroupMemberRoleMember)
	addActiveGroupMember(t, db, group.ID, okID, groupModels.GroupMemberRoleMember)
	blockSessions(t, db, blockedID, "org", &orgX, "absences")
	plan := &paymentModels.SubscriptionPlan{Name: "BulkOrgClass", IsActive: true, MaxCPU: 100000, MaxMemoryMB: 51200, MaxSessionDurationMinutes: 60}
	require.NoError(t, db.Create(plan).Error)

	// No organization_id in the request: the class's own organization applies.
	resp, err := services.NewTerminalTrainerService(db).BulkCreateTerminalsForGroup(
		group.ID.String(), ownerID, []string{"member"},
		dto.BulkCreateTerminalsRequest{Terms: "accepted", InstanceType: "ubuntu-24.04"}, plan)
	require.NoError(t, err)

	results := map[string]dto.BulkTerminalCreationResult{}
	for _, r := range resp.Terminals {
		results[r.UserID] = r
	}
	require.Len(t, results, 2)
	assert.False(t, results[blockedID].Success)
	assert.Equal(t, "session_start_blocked", results[blockedID].Error,
		"a member blocked in the class's organization is skipped even when the request names no organization")
	assert.NotEqual(t, "session_start_blocked", results[okID].Error)
}

// ---------------------------------------------------------------------------
// Bulk RAM fit — counts only the members that will be started
// ---------------------------------------------------------------------------

func TestBulkCreateTerminals_RAMFitCountsOnlyUnblockedMembers(t *testing.T) {
	// 10 GB host, 1.2 GB free, 5% reserve: 0.7 GB spare — one S (512 MiB)
	// fits, three do not.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/metrics"):
			_ = json.NewEncoder(w).Encode(dto.ServerMetricsResponse{RAMTotalGB: 10, RAMAvailableGB: 1.2, RAMPercent: 88, Timestamp: time.Now().Unix()})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/admin/api-keys"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data":    map[string]any{"id": 1, "key_value": "tt-key", "name": "learner", "is_active": true},
			})
		default:
			http.Error(w, "unexpected request", http.StatusServiceUnavailable)
		}
	}))
	t.Cleanup(srv.Close)
	configureTTServer(t, srv.URL)
	casdoorsdk.InitConfig("http://localhost:0", "dummy-endpoint", "dummy-client", "dummy-secret", "dummy-org", "dummy-app")
	db := freshTestDB(t)
	ownerID := "blk-bulk-ram-owner-" + uuid.NewString()
	group := createTestGroup(t, db, ownerID)
	okID := "blk-bulk-ram-ok-" + uuid.NewString()
	addActiveGroupMember(t, db, group.ID, okID, groupModels.GroupMemberRoleMember)
	var blockedIDs []string
	for range 2 {
		id := "blk-bulk-ram-ko-" + uuid.NewString()
		addActiveGroupMember(t, db, group.ID, id, groupModels.GroupMemberRoleMember)
		blockSessions(t, db, id, "group", &group.ID, "late")
		blockedIDs = append(blockedIDs, id)
	}
	plan := &paymentModels.SubscriptionPlan{Name: "BulkRAMBlocked", IsActive: true, MaxCPU: 100000, MaxMemoryMB: 51200, MaxSessionDurationMinutes: 60}
	require.NoError(t, db.Create(plan).Error)

	resp, err := services.NewTerminalTrainerService(db).BulkCreateTerminalsForGroup(
		group.ID.String(), ownerID, []string{"member"},
		dto.BulkCreateTerminalsRequest{Terms: "accepted", InstanceType: "ubuntu-24.04", Size: "S"}, plan)

	require.NotErrorIs(t, err, services.ErrBulkInsufficientRAM,
		"the one unblocked member fits; the blocked ones are never started and need no room")
	require.NoError(t, err)
	results := map[string]dto.BulkTerminalCreationResult{}
	for _, r := range resp.Terminals {
		results[r.UserID] = r
	}
	for _, id := range blockedIDs {
		assert.Equal(t, "session_start_blocked", results[id].Error)
	}
	assert.Contains(t, results, okID, "the unblocked member is attempted")
}

// ---------------------------------------------------------------------------
// Key regeneration — refused under a platform block, old key kept
// ---------------------------------------------------------------------------

func TestRegenerateKey_GlobalBlock_403AndOldKeyKept(t *testing.T) {
	srv := newKeyAdminTTServer(t, 42, false)
	configureTTServer(t, srv.URL)
	db := freshTestDB(t)
	userID := "blk-key-regen-" + uuid.NewString()
	seedProvisionedKey(t, db, userID, 42)
	blockSessions(t, db, userID, "global", nil, "fraud")

	w := regenerateKey(db, userID)

	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	payload := decodeBody(t, w)
	assert.Equal(t, "session_start_blocked", payload["error_message"])
	assert.Equal(t, "global", payload["scope"])
	assert.Equal(t, "fraud", payload["reason"])
	assert.Empty(t, srv.recordedUpdates(), "the old key is not disabled on tt-backend")
	assert.True(t, localKeyIsActive(t, db, userID), "the old key row stays active")
}
