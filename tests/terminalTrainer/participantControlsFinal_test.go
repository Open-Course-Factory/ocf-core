package terminalTrainer_tests

// Trainer controls on class participants (#580), final round:
//
//	A. a sticky internet cut per participant and class: PUT .../network without
//	   session_ids records it in participant_network_cuts, and every session
//	   composed for that participant in the class's organization while it
//	   stands is created without internet (tt-backend body network_disabled).
//	B. a narrower bulk read: persistence_mode per session, running and stopped
//	   sessions only, the actions' scope for an administrator too, no author or
//	   reason of the organization block, no scenario title.
//	C. a class manager may not act on a manager or owner of the class's
//	   organization.
//	D. exposures: dropped before the tt-backend network call, a live instance
//	   without address is stale, no exposure on a session whose internet is cut.
//	E. a class bulk-create always runs in the class's organization.
//	F/G. audit rows: identities in typed columns only, has_reason instead of
//	   the reason, status "requested"; an unblock that removes nothing writes
//	   no "cleared" row.

import (
	"bytes"
	"encoding/json"
	"io"
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
	"gorm.io/gorm"

	auditModels "soli/formations/src/audit/models"
	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/mocks"
	configModels "soli/formations/src/configuration/models"
	entityManagementModels "soli/formations/src/entityManagement/models"
	groupModels "soli/formations/src/groups/models"
	orgModels "soli/formations/src/organizations/models"
	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/terminalTrainer/models"
	terminalController "soli/formations/src/terminalTrainer/routes"
	"soli/formations/src/terminalTrainer/services"
	testTools "soli/formations/tests/testTools"
)

// =============================================================================
// Shared helpers
// =============================================================================

// auditMeta decodes an audit row's Metadata.
func auditMeta(t *testing.T, row auditModels.AuditLog) map[string]any {
	t.Helper()
	meta := map[string]any{}
	if row.Metadata != "" {
		require.NoError(t, json.Unmarshal([]byte(row.Metadata), &meta), "metadata=%s", row.Metadata)
	}
	return meta
}

// assertNoIdentityInMetadata checks a participant-control audit row keeps
// identities in its typed columns only (ActorID, TargetID, OnBehalfOfID).
func assertNoIdentityInMetadata(t *testing.T, row auditModels.AuditLog) {
	t.Helper()
	meta := auditMeta(t, row)
	for _, key := range []string{"actor_user_id", "target_user_id", "impersonator_id", "reason"} {
		assert.NotContains(t, meta, key, "audit Metadata must not carry %q (typed columns only)", key)
	}
}

// pcSeedCut records a sticky internet cut on userID in the pc class.
func pcSeedCut(t *testing.T, c *pcClass, userID string) {
	t.Helper()
	require.NoError(t, testTools.SeedNetworkCut(c.db, userID, c.group.ID, &c.orgID, pcManager))
}

func pcCuts(t *testing.T, db *gorm.DB, userID string) []testTools.NetworkCutRow {
	t.Helper()
	rows, err := testTools.NetworkCutsOf(db, userID)
	require.NoError(t, err, "participant_network_cuts must exist")
	return rows
}

// readControlsRaw is the bulk read as generic JSON, by user id.
func readControlsRaw(t *testing.T, c *pcClass, caller pcCaller) map[string]map[string]any {
	t.Helper()
	w := pcDo(t, c.db, caller, http.MethodGet, controlsPath(c.group.ID), nil)
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	var rows []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rows), "body=%s", w.Body.String())
	out := map[string]map[string]any{}
	for _, row := range rows {
		id, _ := row["user_id"].(string)
		out[id] = row
	}
	return out
}

func rawSessions(row map[string]any) map[string]map[string]any {
	out := map[string]map[string]any{}
	list, _ := row["sessions"].([]any)
	for _, item := range list {
		s, _ := item.(map[string]any)
		id, _ := s["session_id"].(string)
		out[id] = s
	}
	return out
}

// =============================================================================
// Fake tt-backend for composes: records every POST /1.0/sessions body by the
// API key it was sent with (createTestUserKey: "test-api-key-<userID>").
// =============================================================================

type composeTT struct {
	mu     sync.Mutex
	bodies map[string][]map[string]any // by X-API-Key
}

func (c *composeTT) bodiesOf(userID string) []map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]map[string]any(nil), c.bodies["test-api-key-"+userID]...)
}

func newComposeTT(t *testing.T) *composeTT {
	t.Helper()
	rec := &composeTT{bodies: map[string][]map[string]any{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/distributions":
			_ = json.NewEncoder(w).Encode([]dto.TTDistribution{{Name: "ubuntu-24.04", Prefix: "ubuntu", Description: "Ubuntu", SupportedFeatures: []string{"network"}}})
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/sizes":
			_ = json.NewEncoder(w).Encode([]dto.TTSize{{Key: "S", Name: "Small", SortOrder: 20, CPU: 1, CPUAllowance: "25%", Memory: "512MB", Disk: "2GB", Processes: 100}})
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/features":
			_ = json.NewEncoder(w).Encode([]dto.TTFeature{{Key: "network", Name: "Network", ProfileName: "network"}})
		case strings.HasSuffix(r.URL.Path, "/metrics"):
			_ = json.NewEncoder(w).Encode(dto.ServerMetricsResponse{RAMTotalGB: 100, RAMAvailableGB: 90, RAMPercent: 10, Timestamp: time.Now().Unix()})
		case r.Method == http.MethodPost && r.URL.Path == "/1.0/sessions":
			raw, _ := io.ReadAll(r.Body)
			body := map[string]any{}
			_ = json.Unmarshal(raw, &body)
			rec.mu.Lock()
			key := r.Header.Get("X-API-Key")
			rec.bodies[key] = append(rec.bodies[key], body)
			rec.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "cut-sess-" + uuid.NewString(), "expires_at": time.Now().Add(time.Hour).Unix(), "backend": "local", "status": 0,
			})
		default:
			http.Error(w, "unexpected request: "+r.Method+" "+r.URL.Path, http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	configureTTServer(t, srv.URL)
	return rec
}

// cutOrg creates an organization row (team) and returns its id.
func cutOrg(t *testing.T, db *gorm.DB, ownerID string) uuid.UUID {
	t.Helper()
	org := &orgModels.Organization{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Name:      "cut-org-" + uuid.NewString()[:8], DisplayName: "Cut org", OwnerUserID: ownerID,
		OrganizationType: orgModels.OrgTypeTeam, IsActive: true, MaxGroups: 10, MaxMembers: 50,
	}
	require.NoError(t, db.Omit("Metadata").Create(org).Error)
	return org.ID
}

func composeIn(orgID *uuid.UUID) dto.CreateComposedSessionInput {
	in := dto.CreateComposedSessionInput{Distribution: "ubuntu-24.04", Size: "S", Terms: "accepted", PersistenceMode: "ephemeral"}
	if orgID != nil {
		in.OrganizationID = orgID.String()
	}
	return in
}

// =============================================================================
// A. Sticky internet cut
// =============================================================================

func TestStickyNetworkCut_DisableWithoutSessionIDs_RecordsTheCutAndCutsCurrentSessions(t *testing.T) {
	c := newPCClass(t)
	tt := newPCTT(t)
	networked := pcTerminal(t, c.db, pcLearner, &c.orgID, models.StateRunning, `{"network":true}`)

	w := pcDo(t, c.db, member(pcManager), http.MethodPut, participantPath(c.group.ID, pcLearner, "network"), map[string]any{"enabled": false})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	cuts := pcCuts(t, c.db, pcLearner)
	require.Len(t, cuts, 1, "a cut without session_ids is sticky: one participant_network_cuts row")
	assert.Equal(t, c.group.ID.String(), cuts[0].GroupID)
	require.NotNil(t, cuts[0].OrganizationID, "the cut carries the class's organization")
	assert.Equal(t, c.orgID.String(), *cuts[0].OrganizationID)
	assert.Equal(t, pcManager, cuts[0].CutBy)
	require.Len(t, tt.toggles(), 1, "the current network sessions are cut too")
	assert.Equal(t, networked, tt.toggles()[0].sessionID)
	assert.Zero(t, testTools.CountNetworkCuts(c.db, pcLearner2), "nobody else is cut")
}

func TestStickyNetworkCut_NoSessionAtAll_StillRecordsTheCut(t *testing.T) {
	c := newPCClass(t)
	tt := newPCTT(t)

	w := pcDo(t, c.db, member(pcManager), http.MethodPut, participantPath(c.group.ID, pcLearner, "network"), map[string]any{"enabled": false})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.Empty(t, tt.toggles())
	assert.Len(t, pcCuts(t, c.db, pcLearner), 1, "the cut applies to the sessions to come, even with none running")
}

func TestStickyNetworkCut_DisableTwice_OneRow(t *testing.T) {
	c := newPCClass(t)
	newPCTT(t)
	for range 2 {
		w := pcDo(t, c.db, member(pcManager), http.MethodPut, participantPath(c.group.ID, pcLearner, "network"), map[string]any{"enabled": false})
		require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	}
	assert.Len(t, pcCuts(t, c.db, pcLearner), 1, "unique (user_id, group_id): a second cut keeps one row")
}

func TestStickyNetworkCut_EnableWithoutSessionIDs_DeletesTheCutAndRestores(t *testing.T) {
	c := newPCClass(t)
	tt := newPCTT(t)
	networked := pcTerminal(t, c.db, pcLearner, &c.orgID, models.StateRunning, `{"network":true}`)
	setNetworkDisabledColumn(t, c.db, networked, true)
	pcSeedCut(t, c, pcLearner)

	w := pcDo(t, c.db, member(pcManager), http.MethodPut, participantPath(c.group.ID, pcLearner, "network"), map[string]any{"enabled": true})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.Empty(t, pcCuts(t, c.db, pcLearner), "restoring without session_ids lifts the sticky cut")
	require.Len(t, tt.toggles(), 1)
	assert.True(t, *tt.toggles()[0].enabled)
	assert.False(t, networkDisabledColumn(t, c.db, networked))
}

func TestStickyNetworkCut_WithSessionIDs_CutRowUntouched(t *testing.T) {
	c := newPCClass(t)
	newPCTT(t)
	networked := pcTerminal(t, c.db, pcLearner, &c.orgID, models.StateRunning, `{"network":true}`)
	path := participantPath(c.group.ID, pcLearner, "network")

	w := pcDo(t, c.db, member(pcManager), http.MethodPut, path, map[string]any{"enabled": false, "session_ids": []string{networked}})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.Empty(t, pcCuts(t, c.db, pcLearner), "a cut of named sessions is not sticky")

	pcSeedCut(t, c, pcLearner)
	w = pcDo(t, c.db, member(pcManager), http.MethodPut, path, map[string]any{"enabled": true, "session_ids": []string{networked}})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.Len(t, pcCuts(t, c.db, pcLearner), 1, "restoring named sessions leaves the sticky cut in place")
}

func TestStickyNetworkCut_Audited(t *testing.T) {
	c := newPCClass(t)
	newPCTT(t)
	path := participantPath(c.group.ID, pcLearner, "network")

	w := pcDo(t, c.db, member(pcManager), http.MethodPut, path, map[string]any{"enabled": false})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	w = pcDo(t, c.db, member(pcManager), http.MethodPut, path, map[string]any{"enabled": true})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	for _, event := range []string{auditNetworkDisabled, auditNetworkEnabled} {
		rows := auditRowsOf(t, c.db, event)
		require.Len(t, rows, 1, "the sticky %s is audited once, with no session (none is running)", event)
		row := rows[0]
		assert.Empty(t, row.SessionID, "a sticky cut row names no session")
		require.NotNil(t, row.GroupID)
		assert.Equal(t, c.group.ID, *row.GroupID)
		assert.Equal(t, models.SessionBlockScopeGroup, auditMeta(t, row)["scope"])
		assertNoIdentityInMetadata(t, row)
	}
}

// cutInLiveClass is a learner with a terminal key, an active member of a live
// class of a new organization, cut in that class. It returns the database,
// the learner, the organization and the class.
func cutInLiveClass(t *testing.T) (*gorm.DB, string, uuid.UUID, uuid.UUID) {
	t.Helper()
	db := freshTestDB(t)
	userID := "cut-learner-" + uuid.NewString()
	_, err := createTestUserKey(db, userID)
	require.NoError(t, err)
	orgID := cutOrg(t, db, "cut-owner")
	group := createTestGroupForHistory(t, db, "cut-owner", &orgID)
	addActiveGroupMember(t, db, group.ID, userID, groupModels.GroupMemberRoleMember)
	require.NoError(t, testTools.SeedNetworkCut(db, userID, group.ID, &orgID, "cut-manager"))
	return db, userID, orgID, group.ID
}

// networkPlan allows the network feature.
func networkPlan() *paymentModels.SubscriptionPlan {
	plan := makePlan(false)
	plan.NetworkAccessEnabled = true
	return plan
}

// composeWithNetwork is composeIn with the network feature asked for.
func composeWithNetwork(orgID *uuid.UUID) dto.CreateComposedSessionInput {
	in := composeIn(orgID)
	in.Features = map[string]bool{"network": true}
	return in
}

// Contract (final round, N1/N3): the cut that applies is one of a live class
// with an active membership, and a Terminal is recorded cut only when it was
// composed with the network feature.
func TestStickyNetworkCut_ComposedStartInThatOrg_CreatedWithoutNetwork(t *testing.T) {
	tt := newComposeTT(t)
	db, userID, orgID, _ := cutInLiveClass(t)

	resp, err := services.NewTerminalTrainerService(db).StartComposedSession(userID, composeWithNetwork(&orgID), networkPlan())
	require.NoError(t, err)

	bodies := tt.bodiesOf(userID)
	require.Len(t, bodies, 1)
	assert.Equal(t, true, bodies[0]["network_disabled"], "a participant cut in this organization starts without internet; body=%v", bodies[0])
	assert.True(t, networkDisabledColumn(t, db, resp.SessionID), "the Terminal row records the cut")
}

// N1: a cut nobody can lift any more does not cut.
func TestStickyNetworkCut_ClassArchivedOrDeleted_NoLongerCuts(t *testing.T) {
	for name, retire := range map[string]string{
		"archived":     "UPDATE class_groups SET archived_at = ? WHERE id = ?",
		"soft-deleted": "UPDATE class_groups SET deleted_at = ? WHERE id = ?",
	} {
		t.Run(name, func(t *testing.T) {
			tt := newComposeTT(t)
			db, userID, orgID, groupID := cutInLiveClass(t)
			require.NoError(t, db.Exec(retire, time.Now(), groupID.String()).Error)

			resp, err := services.NewTerminalTrainerService(db).StartComposedSession(userID, composeWithNetwork(&orgID), networkPlan())
			require.NoError(t, err)
			bodies := tt.bodiesOf(userID)
			require.Len(t, bodies, 1)
			assert.NotEqual(t, true, bodies[0]["network_disabled"], "a cut of a %s class no longer applies; body=%v", name, bodies[0])
			assert.False(t, networkDisabledColumn(t, db, resp.SessionID))
		})
	}
}

func TestStickyNetworkCut_MembershipDeactivated_NoLongerCuts(t *testing.T) {
	tt := newComposeTT(t)
	db, userID, orgID, groupID := cutInLiveClass(t)
	require.NoError(t, db.Exec("UPDATE group_members SET is_active = ? WHERE group_id = ? AND user_id = ?", false, groupID.String(), userID).Error)

	resp, err := services.NewTerminalTrainerService(db).StartComposedSession(userID, composeWithNetwork(&orgID), networkPlan())
	require.NoError(t, err)
	bodies := tt.bodiesOf(userID)
	require.Len(t, bodies, 1)
	assert.NotEqual(t, true, bodies[0]["network_disabled"], "a cut of a membership no longer active does not apply; body=%v", bodies[0])
	assert.False(t, networkDisabledColumn(t, db, resp.SessionID))
}

// N3: tt-backend ignores the flag on a session without the network feature;
// the Terminal row must agree. (The request may carry the field or not.)
func TestStickyNetworkCut_ComposeWithoutNetworkFeature_TerminalNotMarkedCut(t *testing.T) {
	newComposeTT(t)
	db, userID, orgID, _ := cutInLiveClass(t)

	resp, err := services.NewTerminalTrainerService(db).StartComposedSession(userID, composeIn(&orgID), networkPlan())
	require.NoError(t, err)
	assert.False(t, networkDisabledColumn(t, db, resp.SessionID),
		"a session composed without network has no internet to cut: NetworkDisabled stays false")
}

func TestStickyNetworkCut_NoCut_FieldAbsent(t *testing.T) {
	tt := newComposeTT(t)
	db := freshTestDB(t)
	userID := "cut-free-" + uuid.NewString()
	_, err := createTestUserKey(db, userID)
	require.NoError(t, err)
	orgID := cutOrg(t, db, "cut-owner")

	resp, err := services.NewTerminalTrainerService(db).StartComposedSession(userID, composeIn(&orgID), makePlan(false))
	require.NoError(t, err)
	bodies := tt.bodiesOf(userID)
	require.Len(t, bodies, 1)
	assert.NotEqual(t, true, bodies[0]["network_disabled"])
	assert.False(t, networkDisabledColumn(t, db, resp.SessionID))
}

func TestStickyNetworkCut_PersonalOrOtherOrgCompose_Unaffected(t *testing.T) {
	tt := newComposeTT(t)
	db := freshTestDB(t)
	userID := "cut-elsewhere-" + uuid.NewString()
	_, err := createTestUserKey(db, userID)
	require.NoError(t, err)
	cutIn := cutOrg(t, db, "cut-owner")
	other := cutOrg(t, db, "cut-owner-2")
	require.NoError(t, testTools.SeedNetworkCut(db, userID, uuid.New(), &cutIn, "cut-manager"))

	svc := services.NewTerminalTrainerService(db)
	personal, err := svc.StartComposedSession(userID, composeIn(nil), makePlan(false))
	require.NoError(t, err)
	elsewhere, err := svc.StartComposedSession(userID, composeIn(&other), makePlan(false))
	require.NoError(t, err)

	bodies := tt.bodiesOf(userID)
	require.Len(t, bodies, 2)
	for _, body := range bodies {
		assert.NotEqual(t, true, body["network_disabled"], "a cut in another organization does not reach this compose; body=%v", body)
	}
	assert.False(t, networkDisabledColumn(t, db, personal.SessionID))
	assert.False(t, networkDisabledColumn(t, db, elsewhere.SessionID))
}

func TestStickyNetworkCut_ClassBulkCreate_CutMemberCreatedWithoutNetwork(t *testing.T) {
	tt := newComposeTT(t)
	db := freshTestDB(t)
	ownerID := "cut-bulk-owner-" + uuid.NewString()
	orgID := cutOrg(t, db, ownerID)
	group := createTestGroupForHistory(t, db, ownerID, &orgID)
	cutID, okID := "cut-bulk-ko-"+uuid.NewString(), "cut-bulk-ok-"+uuid.NewString()
	for _, id := range []string{cutID, okID} {
		addActiveGroupMember(t, db, group.ID, id, groupModels.GroupMemberRoleMember)
		_, err := createTestUserKey(db, id)
		require.NoError(t, err)
	}
	require.NoError(t, testTools.SeedNetworkCut(db, cutID, group.ID, &orgID, ownerID))
	plan := makePlan(false)
	require.NoError(t, db.Create(plan).Error)

	resp, err := services.NewTerminalTrainerService(db).BulkCreateTerminalsForGroup(
		group.ID.String(), ownerID, []string{"member"}, dto.BulkCreateTerminalsRequest{Terms: "accepted", InstanceType: "ubuntu-24.04", Size: "S"}, plan)
	require.NoError(t, err)
	require.Equal(t, 2, resp.CreatedCount, "both members are started; resp=%+v", resp)

	cut, ok := tt.bodiesOf(cutID), tt.bodiesOf(okID)
	require.Len(t, cut, 1)
	require.Len(t, ok, 1)
	assert.Equal(t, true, cut[0]["network_disabled"], "the cut member's session starts without internet")
	assert.NotEqual(t, true, ok[0]["network_disabled"])
}

func TestStickyNetworkCut_BulkReadCarriesNetworkCut(t *testing.T) {
	c := newPCClass(t)
	pcSeedCut(t, c, pcLearner)

	rows := readControlsRaw(t, c, member(pcManager))
	assert.Equal(t, true, rows[pcLearner]["network_cut"], "a participant cut in this class reads network_cut=true")
	assert.Equal(t, false, rows[pcLearner2]["network_cut"], "network_cut is always present, false when not cut")
}

// =============================================================================
// B. Bulk read
// =============================================================================

func TestParticipantControls_SessionCarriesPersistenceMode(t *testing.T) {
	c := newPCClass(t)
	ephemeral := pcTerminal(t, c.db, pcLearner, &c.orgID, models.StateRunning, "")
	require.NoError(t, c.db.Model(&models.Terminal{}).Where("session_id = ?", ephemeral).Update("persistence_mode", "ephemeral").Error)

	sessions := rawSessions(readControlsRaw(t, c, member(pcManager))[pcLearner])
	require.Contains(t, sessions, c.sessions["learner"])
	require.Contains(t, sessions, ephemeral)
	assert.Equal(t, "persistent", sessions[c.sessions["learner"]]["persistence_mode"])
	assert.Equal(t, "ephemeral", sessions[ephemeral]["persistence_mode"])
}

func TestParticipantControls_OnlyRunningAndStoppedSessions(t *testing.T) {
	c := newPCClass(t)
	stopped := pcTerminal(t, c.db, pcLearner, &c.orgID, models.StateStopped, "")
	deleted := pcTerminal(t, c.db, pcLearner, &c.orgID, models.StateDeleted, "")
	revoked := pcTerminal(t, c.db, pcLearner, &c.orgID, models.StateRevoked, "")

	sessions := rawSessions(readControlsRaw(t, c, member(pcManager))[pcLearner])
	assert.Contains(t, sessions, c.sessions["learner"])
	assert.Contains(t, sessions, stopped)
	assert.NotContains(t, sessions, deleted, "a deleted session is not listed")
	assert.NotContains(t, sessions, revoked, "a revoked session is not listed")
}

func TestParticipantControls_AdminReadsTheActionsScopeOnly(t *testing.T) {
	c := newPCClass(t)
	personal := pcTerminal(t, c.db, pcLearner, nil, models.StateRunning, "")
	elsewhere := pcTerminal(t, c.db, pcLearner, c.otherGroup.OrganizationID, models.StateRunning, "")

	sessions := rawSessions(readControlsRaw(t, c, pcCaller{userID: pcAdmin, roles: []string{"administrator"}})[pcLearner])
	assert.Contains(t, sessions, c.sessions["learner"])
	assert.NotContains(t, sessions, personal, "an administrator reads the class's organization sessions, as the actions reach")
	assert.NotContains(t, sessions, elsewhere)
}

func TestParticipantControls_ShapeHidesOrgAndGlobalBlockDetails(t *testing.T) {
	c := newPCClass(t)
	blockSessions(t, c.db, pcLearner, models.SessionBlockScopeGroup, &c.group.ID, "class reason")
	blockSessions(t, c.db, pcLearner, models.SessionBlockScopeOrg, &c.orgID, "org secret reason")
	blockSessions(t, c.db, pcLearner2, models.SessionBlockScopeGlobal, nil, "global secret reason")

	w := pcDo(t, c.db, member(pcManager), http.MethodGet, controlsPath(c.group.ID), nil)
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	body := w.Body.String()
	assert.NotContains(t, body, "org secret reason", "the organization block's reason is not shown to class managers")
	assert.NotContains(t, body, "global secret reason")
	assert.NotContains(t, body, "test-blocker", "no block author is shown")
	assert.NotContains(t, body, "scenario_title")

	rows := readControlsRaw(t, c, member(pcManager))
	learner := rows[pcLearner]
	assert.NotContains(t, learner, "org_block", "org_block is replaced by org_blocked")
	assert.Equal(t, true, learner["org_blocked"])
	assert.Equal(t, true, rows[pcLearner2]["global_block"])
	classBlock, ok := learner["class_block"].(map[string]any)
	require.True(t, ok, "class_block is an object")
	assert.Equal(t, map[string]any{"reason": "class reason"}, classBlock, "class_block carries its reason only")
}

// =============================================================================
// C. Target protection: managers and owner of the class's organization
// =============================================================================

const (
	pcOrgOwner   = "pc-org-owner"
	pcOrgManager = "pc-org-manager"
)

// withClassOrg creates the class's organization, owned by pcOrgOwner, with
// pcOrgManager as its manager; both are plain members of the class.
func withClassOrg(t *testing.T, c *pcClass) {
	t.Helper()
	org := &orgModels.Organization{
		BaseModel: entityManagementModels.BaseModel{ID: c.orgID},
		Name:      "pc-org-" + uuid.NewString()[:8], DisplayName: "pc org", OwnerUserID: pcOrgOwner,
		OrganizationType: orgModels.OrgTypeTeam, IsActive: true, MaxGroups: 10, MaxMembers: 50,
	}
	require.NoError(t, c.db.Omit("Metadata").Create(org).Error)
	require.NoError(t, c.db.Omit("Metadata").Create(&orgModels.OrganizationMember{
		OrganizationID: c.orgID, UserID: pcOrgManager, Role: orgModels.OrgRoleManager, JoinedAt: time.Now(), IsActive: true,
	}).Error)
	for _, id := range []string{pcOrgOwner, pcOrgManager} {
		createTestGroupMember(t, c.db, c.group.ID, id, groupModels.GroupMemberRoleMember)
	}
}

func TestParticipantControlsWrites_TargetManagesTheClassOrganization_403(t *testing.T) {
	for _, target := range []string{pcOrgManager, pcOrgOwner} {
		for _, route := range pcRoutes()[1:] {
			t.Run(target+"/"+route.name, func(t *testing.T) {
				c := newPCClass(t)
				withClassOrg(t, c)
				tt := newPCTT(t)
				session := pcTerminal(t, c.db, target, &c.orgID, models.StateRunning, `{"network":true}`)

				w := pcDo(t, c.db, member(pcManager), route.method, route.path(c, target), route.body)
				assert.Equal(t, http.StatusForbidden, w.Code, "a class manager may not act on a manager or owner of the class's organization; body=%s", w.Body.String())
				assert.Empty(t, tt.stopped())
				assert.Empty(t, tt.toggles())
				assert.Zero(t, testTools.CountSessionBlocks(c.db, target))
				assert.Zero(t, testTools.CountNetworkCuts(c.db, target))
				assert.Equal(t, models.StateRunning, pcState(t, c.db, session))
			})
		}
	}
}

func TestParticipantControlsWrites_AdminOnClassOrganizationManager_Allowed(t *testing.T) {
	c := newPCClass(t)
	withClassOrg(t, c)
	newPCTT(t)
	w := pcDo(t, c.db, pcCaller{userID: pcAdmin, roles: []string{"administrator"}}, http.MethodPut,
		participantPath(c.group.ID, pcOrgManager, "session-block"), map[string]any{"blocked": true, "reason": "r"})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.Len(t, classBlockRows(t, c.db, pcOrgManager, c.group.ID), 1)
}

// =============================================================================
// D. Exposures
// =============================================================================

func TestSetParticipantNetwork_Disable_ExposuresDroppedBeforeTheTTCall(t *testing.T) {
	c := newPCClass(t)
	tt := newPCTT(t)
	networked := pcTerminal(t, c.db, pcLearner, &c.orgID, models.StateRunning, `{"network":true}`)
	seedExposedPort(t, c.db, networked)
	var atCall []int64
	var mu sync.Mutex
	tt.onNetwork = func(sessionID string) {
		var n int64
		c.db.Model(&models.ExposedPort{}).Where("session_id = ?", sessionID).Count(&n)
		mu.Lock()
		atCall = append(atCall, n)
		mu.Unlock()
	}

	w := pcDo(t, c.db, member(pcManager), http.MethodPut, participantPath(c.group.ID, pcLearner, "network"),
		map[string]any{"enabled": false, "session_ids": []string{networked}})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []int64{0}, atCall, "the exposures are gone before tt-backend is asked to cut")
}

func TestSetParticipantNetwork_DisableFailsOnTT_ExposuresStillDropped(t *testing.T) {
	c := newPCClass(t)
	tt := newPCTT(t)
	networked := pcTerminal(t, c.db, pcLearner, &c.orgID, models.StateRunning, `{"network":true}`)
	seedExposedPort(t, c.db, networked)
	tt.networkReply[networked] = struct {
		status int
		code   string
	}{http.StatusBadGateway, "network_renew_failed"}

	w := pcDo(t, c.db, member(pcManager), http.MethodPut, participantPath(c.group.ID, pcLearner, "network"),
		map[string]any{"enabled": false, "session_ids": []string{networked}})
	assert.Equal(t, http.StatusBadGateway, w.Code, "body=%s", w.Body.String())
	assert.Zero(t, exposedPortCount(t, c.db, networked), "a cut asked for drops the public routes whatever tt-backend answers")
}

// exposeFixture is a running, network-composed session whose plan allows
// exposure, the platform flag on, and a tt-backend /info answering an IP.
func exposeFixture(t *testing.T, networkDisabled bool) (*gorm.DB, string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"stub","status":0,"ip":"10.0.0.5","instance_running":true}`))
	}))
	t.Cleanup(srv.Close)
	configureTTServer(t, srv.URL)

	db := freshTestDB(t)
	require.NoError(t, db.Create(&configModels.Feature{Key: services.PortExposureFeatureKey, Name: "x", Enabled: true}).Error)
	plan := &paymentModels.SubscriptionPlan{Name: "expose", PortExposureEnabled: true, IsActive: true}
	require.NoError(t, db.Create(plan).Error)
	userID := "expose-" + uuid.NewString()
	key, err := createTestUserKey(db, userID)
	require.NoError(t, err)
	term := &models.Terminal{
		SessionID: "expose-sess-" + uuid.NewString(), UserID: userID, State: models.StateRunning,
		ExpiresAt: time.Now().Add(time.Hour), MachineSize: "S", ComposedFeatures: `{"network":true}`,
		UserTerminalKeyID: key.ID, SubscriptionPlanID: &plan.ID,
	}
	require.NoError(t, db.Create(term).Error)
	setNetworkDisabledColumn(t, db, term.SessionID, networkDisabled)
	return db, term.SessionID
}

func postExposure(t *testing.T, db *gorm.DB, sessionID string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/terminals/:id/exposed-ports", terminalController.NewTerminalController(db).CreateExposedPort)
	req := httptest.NewRequest(http.MethodPost, "/terminals/"+sessionID+"/exposed-ports", bytes.NewReader([]byte(`{"port":8080}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestCreateExposedPort_NetworkCut_409(t *testing.T) {
	db, sessionID := exposeFixture(t, true)
	w := postExposure(t, db, sessionID)
	assert.Equal(t, http.StatusConflict, w.Code, "no public route to a session whose internet is cut; body=%s", w.Body.String())
	assert.Zero(t, exposedPortCount(t, db, sessionID))
}

func TestCreateExposedPort_NetworkOn_Created(t *testing.T) {
	db, sessionID := exposeFixture(t, false)
	w := postExposure(t, db, sessionID)
	assert.Equal(t, http.StatusCreated, w.Code, "control: the same session with internet is exposed; body=%s", w.Body.String())
}

// =============================================================================
// E. Class bulk-create runs in the class's organization
// =============================================================================

func TestBulkCreateTerminals_RequestOrgDiffers_ClassOrgForced(t *testing.T) {
	tt := newComposeTT(t)
	db := freshTestDB(t)
	ownerID := "bulk-org-owner-" + uuid.NewString()
	classOrg := cutOrg(t, db, ownerID)
	requestOrg := cutOrg(t, db, ownerID)
	group := createTestGroupForHistory(t, db, ownerID, &classOrg)
	blockedID, okID := "bulk-org-ko-"+uuid.NewString(), "bulk-org-ok-"+uuid.NewString()
	for _, id := range []string{blockedID, okID} {
		addActiveGroupMember(t, db, group.ID, id, groupModels.GroupMemberRoleMember)
		_, err := createTestUserKey(db, id)
		require.NoError(t, err)
	}
	blockSessions(t, db, blockedID, models.SessionBlockScopeOrg, &classOrg, "absences")
	plan := makePlan(false)
	require.NoError(t, db.Create(plan).Error)

	resp, err := services.NewTerminalTrainerService(db).BulkCreateTerminalsForGroup(
		group.ID.String(), ownerID, []string{"member"},
		dto.BulkCreateTerminalsRequest{Terms: "accepted", InstanceType: "ubuntu-24.04", Size: "S", OrganizationID: requestOrg.String()}, plan)
	require.NoError(t, err)

	results := map[string]dto.BulkTerminalCreationResult{}
	for _, r := range resp.Terminals {
		results[r.UserID] = r
	}
	assert.Equal(t, "session_start_blocked", results[blockedID].Error,
		"another request organization does not get round a block in the class's organization")
	assert.Empty(t, tt.bodiesOf(blockedID))

	require.True(t, results[okID].Success, "result=%+v", results[okID])
	var term models.Terminal
	require.NoError(t, db.First(&term, "session_id = ?", *results[okID].SessionID).Error)
	require.NotNil(t, term.OrganizationID)
	assert.Equal(t, classOrg, *term.OrganizationID, "the session runs in the class's organization, whatever the request says")
}

// =============================================================================
// F/G. Audit rows
// =============================================================================

// uuidParticipants adds a manager and a learner with UUID ids (Casdoor ids
// are UUIDs: the typed audit columns hold them) to the pc class.
func uuidParticipants(t *testing.T, c *pcClass) (manager, learner uuid.UUID) {
	t.Helper()
	manager, learner = uuid.New(), uuid.New()
	createTestGroupMember(t, c.db, c.group.ID, manager.String(), groupModels.GroupMemberRoleManager)
	createTestGroupMember(t, c.db, c.group.ID, learner.String(), groupModels.GroupMemberRoleMember)
	return manager, learner
}

func TestParticipantControlsAudit_IdentitiesInTypedColumnsOnly(t *testing.T) {
	c := newPCClass(t)
	newPCTT(t)
	manager, learner := uuidParticipants(t, c)
	pcTerminal(t, c.db, learner.String(), &c.orgID, models.StateRunning, `{"network":true}`)

	w := pcDo(t, c.db, member(manager.String()), http.MethodPut, participantPath(c.group.ID, learner.String(), "network"), map[string]any{"enabled": false})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	w = pcDo(t, c.db, member(manager.String()), http.MethodPost, participantPath(c.group.ID, learner.String(), "stop-sessions"), map[string]any{})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	w = pcDo(t, c.db, member(manager.String()), http.MethodPut, participantPath(c.group.ID, learner.String(), "session-block"),
		map[string]any{"blocked": true, "reason": "private reason text"})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	var rows []auditModels.AuditLog
	require.NoError(t, c.db.Where("event_type IN ?", []string{auditNetworkDisabled, auditStoppedByTrainer, auditSessionBlockSet}).Find(&rows).Error)
	require.NotEmpty(t, rows)
	seen := map[string]bool{}
	for _, row := range rows {
		seen[string(row.EventType)] = true
		assertNoIdentityInMetadata(t, row)
		assert.NotContains(t, row.Metadata, manager.String(), "%s: the actor is not in Metadata", row.EventType)
		assert.NotContains(t, row.Metadata, learner.String(), "%s: the target is not in Metadata", row.EventType)
		require.NotNil(t, row.ActorID, "%s: typed actor", row.EventType)
		assert.Equal(t, manager, *row.ActorID)
		require.NotNil(t, row.TargetID, "%s: typed target", row.EventType)
		assert.Equal(t, learner, *row.TargetID)
		assert.NotEqual(t, learner.String(), row.TargetName, "%s: TargetName is not the user id", row.EventType)
		assert.Equal(t, "requested", row.Status, "%s: a row written before acting says requested, not success", row.EventType)
	}
	assert.True(t, seen[auditNetworkDisabled] && seen[auditStoppedByTrainer] && seen[auditSessionBlockSet], "seen=%v", seen)
}

func TestParticipantControlsAudit_BlockRecordsHasReasonNotTheReason(t *testing.T) {
	c := newPCClass(t)
	newPCTT(t)
	path := participantPath(c.group.ID, pcLearner, "session-block")

	w := pcDo(t, c.db, member(pcManager), http.MethodPut, path, map[string]any{"blocked": true, "reason": "private reason text"})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	w = pcDo(t, c.db, member(pcManager), http.MethodPut, path, map[string]any{"blocked": true, "reason": ""})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	rows := auditRowsOf(t, c.db, auditSessionBlockSet)
	require.Len(t, rows, 2)
	var hasReason []any
	for _, row := range rows {
		assert.NotContains(t, row.Metadata, "private reason text")
		hasReason = append(hasReason, auditMeta(t, row)["has_reason"])
	}
	assert.ElementsMatch(t, []any{true, false}, hasReason, "has_reason is a boolean per row")
}

func TestParticipantControlsAudit_ImpersonatorIsTheTypedActor(t *testing.T) {
	c := newPCClass(t)
	newPCTT(t)
	manager, learner := uuidParticipants(t, c)
	impersonator := uuid.New()
	caller := pcCaller{userID: manager.String(), roles: []string{"member"}, impersonator: impersonator.String()}

	w := pcDo(t, c.db, caller, http.MethodPut, participantPath(c.group.ID, learner.String(), "session-block"), map[string]any{"blocked": true, "reason": "r"})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	rows := auditRowsOf(t, c.db, auditSessionBlockSet)
	require.Len(t, rows, 1)
	require.NotNil(t, rows[0].ActorID)
	assert.Equal(t, impersonator, *rows[0].ActorID)
	require.NotNil(t, rows[0].OnBehalfOfID)
	assert.Equal(t, manager, *rows[0].OnBehalfOfID)
	assert.NotContains(t, rows[0].Metadata, impersonator.String())
}

func TestSetParticipantSessionBlock_UnblockWithoutBlock_200NoClearedAudit(t *testing.T) {
	c := newPCClass(t)
	newPCTT(t)
	w := pcDo(t, c.db, member(pcManager), http.MethodPut, participantPath(c.group.ID, pcLearner, "session-block"), map[string]any{"blocked": false})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.Empty(t, auditRowsOf(t, c.db, auditSessionBlockClear), "nothing was cleared: no cleared row")
}

// pcPlatformAdminMember is a plain member of the class whom the platform
// role lookup answers as an administrator.
const pcPlatformAdminMember = "pc-platform-admin-member"

func withPlatformAdminMember(t *testing.T, c *pcClass) string {
	t.Helper()
	orig := casdoor.Enforcer
	enforcer := mocks.NewMockEnforcer()
	enforcer.GetRolesForUserFunc = func(name string) ([]string, error) {
		if name == pcPlatformAdminMember {
			return []string{"member", "administrator"}, nil
		}
		return []string{"member"}, nil
	}
	casdoor.Enforcer = enforcer
	t.Cleanup(func() { casdoor.Enforcer = orig })
	createTestGroupMember(t, c.db, c.group.ID, pcPlatformAdminMember, groupModels.GroupMemberRoleMember)
	return pcTerminal(t, c.db, pcPlatformAdminMember, &c.orgID, models.StateRunning, `{"network":true}`)
}

// A platform administrator who is a plain member of the class is out of a
// class manager's and owner's reach, as in the organization scope: no stop,
// no network cut, no session block.
func TestParticipantControlsWrites_TargetIsPlatformAdministrator_403(t *testing.T) {
	for _, caller := range []string{pcManager, pcOwner} {
		for _, route := range pcRoutes()[1:] {
			t.Run(caller+"/"+route.name, func(t *testing.T) {
				c := newPCClass(t)
				tt := newPCTT(t)
				session := withPlatformAdminMember(t, c)

				w := pcDo(t, c.db, member(caller), route.method, route.path(c, pcPlatformAdminMember), route.body)

				assert.Equal(t, http.StatusForbidden, w.Code, "body=%s", w.Body.String())
				assert.Empty(t, tt.stopped())
				assert.Empty(t, tt.toggles())
				assert.Zero(t, testTools.CountSessionBlocks(c.db, pcPlatformAdminMember))
				assert.Zero(t, testTools.CountNetworkCuts(c.db, pcPlatformAdminMember))
				assert.Equal(t, models.StateRunning, pcState(t, c.db, session))
			})
		}
	}
}

func TestParticipantControlsWrites_AdminOnPlatformAdministratorMember_Allowed(t *testing.T) {
	for _, route := range pcRoutes()[1:] {
		t.Run(route.name, func(t *testing.T) {
			c := newPCClass(t)
			newPCTT(t)
			withPlatformAdminMember(t, c)

			w := pcDo(t, c.db, pcCaller{userID: pcAdmin, roles: []string{"administrator"}}, route.method, route.path(c, pcPlatformAdminMember), route.body)

			assert.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
		})
	}
}
