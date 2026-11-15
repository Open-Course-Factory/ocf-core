package terminalController

// participantControls.go — a trainer's controls on the participants of a class
// (#580): one bulk read for the members list, the live wall and the
// supervisor's session page, and three actions (stop sessions, cut or restore
// internet, block session starts in the class). The caller must manage the
// class (MayListGroupSessions, shared with the wall); the actions themselves
// live in services.ParticipantControlsService, shared with the organization
// and platform-admin blocks.

import (
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"soli/formations/src/auth/access"
	apiErrors "soli/formations/src/auth/errors"
	groupModels "soli/formations/src/groups/models"
	scenarioModels "soli/formations/src/scenarios/models"
	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/terminalTrainer/httperrors"
	terminalModels "soli/formations/src/terminalTrainer/models"
	services "soli/formations/src/terminalTrainer/services"
)

// ListParticipantControls returns one row per ACTIVE member of groupID (owner
// and managers included, each row carrying its class role): their block in
// this class (with its reason), whether they are blocked in its organization
// or on the platform (no reason, no author: those are not the class's), whether
// their internet is cut in this class, and their running and stopped sessions
// in the class's organization — the sessions the actions reach, for an
// administrator too — each with its network flags, its persistence mode and
// the scenario run open on it. user_name is the supervision wall's.
// allowed=false refuses a caller who does not manage the class.
//
// A fixed number of queries whatever the class size: no query per member or
// per session.
func ListParticipantControls(db *gorm.DB, groupID, callerUserID string, isAdmin bool) (rows []dto.ParticipantControlsRow, allowed bool, err error) {
	gid, allowed := MayListGroupSessions(db, groupID, callerUserID, isAdmin)
	if !allowed {
		return nil, false, nil
	}

	var group groupModels.ClassGroup
	if err := db.Select("id", "owner_user_id", "organization_id").First(&group, "id = ?", gid).Error; err != nil {
		return nil, true, err
	}
	var members []groupModels.GroupMember
	if err := db.Select("user_id", "role").
		Where("group_id = ? AND is_active = ?", gid, true).
		Order("created_at").
		Find(&members).Error; err != nil {
		return nil, true, err
	}
	memberIDs := make([]string, 0, len(members))
	for _, m := range members {
		memberIDs = append(memberIDs, m.UserID)
	}
	terminals, err := participantSessions(db, memberIDs, group.OrganizationID)
	if err != nil {
		return nil, true, err
	}
	blocks, err := participantBlocks(db, memberIDs, gid, group.OrganizationID)
	if err != nil {
		return nil, true, err
	}
	cut, err := terminalModels.NetworkCutInClass(db, memberIDs, gid)
	if err != nil {
		return nil, true, err
	}
	runs, err := openRunsOn(db, terminals)
	if err != nil {
		return nil, true, err
	}

	sessionsByUser := make(map[string][]dto.ParticipantSessionView, len(memberIDs))
	for _, t := range terminals {
		view := dto.ParticipantSessionView{
			SessionID:       t.SessionID,
			State:           string(t.State),
			NetworkFeature:  t.ComposedWithNetwork(),
			NetworkDisabled: t.NetworkDisabled,
			PersistenceMode: t.PersistenceMode,
		}
		if runID, open := runs[t.SessionID]; open {
			view.ScenarioSessionID = &runID
		}
		sessionsByUser[t.UserID] = append(sessionsByUser[t.UserID], view)
	}

	identityOf := newIdentityResolver(memberIDs)
	rows = make([]dto.ParticipantControlsRow, 0, len(members))
	for _, m := range members {
		userID := m.UserID
		// The class's owner is reported as such whatever their membership
		// row says: ClassControlScope refuses acting on them the same way.
		role := string(m.Role)
		if userID == group.OwnerUserID {
			role = string(groupModels.GroupMemberRoleOwner)
		}
		row := dto.ParticipantControlsRow{
			UserID:     userID,
			UserName:   identityOf(userID).name,
			Role:       role,
			NetworkCut: cut[userID],
			Sessions:   sessionsByUser[userID],
		}
		if row.Sessions == nil {
			row.Sessions = []dto.ParticipantSessionView{}
		}
		for _, b := range blocks[userID] {
			switch b.ScopeType {
			case terminalModels.SessionBlockScopeGroup:
				row.ClassBlock = &dto.ParticipantBlockView{Reason: b.Reason}
			case terminalModels.SessionBlockScopeOrg:
				row.OrgBlocked = true
			case terminalModels.SessionBlockScopeGlobal:
				row.GlobalBlock = true
			}
		}
		rows = append(rows, row)
	}
	return rows, true, nil
}

// participantSessions returns userIDs' running and stopped sessions in the
// organization orgID, as the actions reach them (ControlScope.sessionsOf): a
// class with no organization lists none. One query.
func participantSessions(db *gorm.DB, userIDs []string, orgID *uuid.UUID) ([]terminalModels.Terminal, error) {
	terminals := []terminalModels.Terminal{}
	if len(userIDs) == 0 {
		return terminals, nil
	}
	err := db.Where("user_id IN ?", userIDs).
		Where("state IN ?", []terminalModels.TerminalState{terminalModels.StateRunning, terminalModels.StateStopped}).
		Scopes(terminalModels.SupervisableByGroupOrgScope(orgID)).
		Order("created_at").
		Find(&terminals).Error
	return terminals, err
}

// participantBlocks returns, per user, their blocks in the class groupID, in
// its organization orgID and on the platform. One query.
func participantBlocks(db *gorm.DB, userIDs []string, groupID uuid.UUID, orgID *uuid.UUID) (map[string][]terminalModels.SessionBlock, error) {
	byUser := make(map[string][]terminalModels.SessionBlock)
	if len(userIDs) == 0 {
		return byUser, nil
	}
	scope := db.Where("scope_type = ?", terminalModels.SessionBlockScopeGlobal).
		Or("scope_type = ? AND scope_id = ?", terminalModels.SessionBlockScopeGroup, groupID)
	if orgID != nil {
		scope = scope.Or("scope_type = ? AND scope_id = ?", terminalModels.SessionBlockScopeOrg, *orgID)
	}
	var blocks []terminalModels.SessionBlock
	if err := db.Select("user_id", "scope_type", "reason").Where("user_id IN ?", userIDs).Where(scope).Find(&blocks).Error; err != nil {
		return nil, err
	}
	for _, b := range blocks {
		byUser[b.UserID] = append(byUser[b.UserID], b)
	}
	return byUser, nil
}

// openRunsOn returns, by terminal session id, the id of the scenario run open
// on each of terminals (status in scenarioModels.OpenSessionStatuses). One
// query.
func openRunsOn(db *gorm.DB, terminals []terminalModels.Terminal) (map[string]string, error) {
	runs := make(map[string]string)
	if len(terminals) == 0 {
		return runs, nil
	}
	sessionIDs := make([]string, 0, len(terminals))
	for _, t := range terminals {
		sessionIDs = append(sessionIDs, t.SessionID)
	}
	var rows []struct {
		ID                string
		TerminalSessionID string
	}
	if err := db.Table("scenario_sessions").
		Select("id, terminal_session_id").
		Where("terminal_session_id IN ?", sessionIDs).
		Where("status IN ?", scenarioModels.OpenSessionStatuses).
		Where("deleted_at IS NULL").
		Order("started_at").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		runs[r.TerminalSessionID] = r.ID // the latest open run wins
	}
	return runs, nil
}

// GetParticipantControls godoc
//
//	@Summary		A class's participants with their blocks and sessions
//	@Description	One row per active member of the class: their class block (with reason), whether they are blocked in its organization or on the platform, whether their internet is cut in the class, and their running and stopped sessions in the class's organization (network state, persistence mode, open scenario run). Manager+ of the class (or admin) only.
//	@Tags			terminals
//	@Produce		json
//	@Param			id	path	string	true	"Class-group ID"
//	@Security		Bearer
//	@Success		200	{array}		dto.ParticipantControlsRow
//	@Failure		403	{object}	errors.APIError	"Not a manager of this class"
//	@Router			/class-groups/{id}/participant-controls [get]
func (tc *terminalController) GetParticipantControls(ctx *gin.Context) {
	rows, allowed, err := ListParticipantControls(tc.db, ctx.Param("id"), ctx.GetString("userId"), access.IsAdmin(ctx.GetStringSlice("userRoles")))
	if !allowed {
		apiErrors.Respond(ctx, http.StatusForbidden, "You are not a manager of this group")
		return
	}
	if err != nil {
		httperrors.WriteParticipantControlError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, rows)
}

// classParticipant authorizes an action on the participant :userId of the
// class :id and returns who acts and the class scope; it answers the request
// itself (403) when refused.
func (tc *terminalController) classParticipant(ctx *gin.Context) (services.ControlActor, services.ControlScope, bool) {
	actor := httperrors.ControlActorFrom(ctx)
	gid, allowed := MayListGroupSessions(tc.db, ctx.Param("id"), actor.UserID, actor.IsAdmin)
	if !allowed {
		apiErrors.Respond(ctx, http.StatusForbidden, "You are not a manager of this group")
		return actor, services.ControlScope{}, false
	}
	scope, err := services.ClassControlScope(tc.db, gid, actor, ctx.Param("userId"))
	if err != nil {
		httperrors.WriteParticipantControlError(ctx, err)
		return actor, scope, false
	}
	return actor, scope, true
}

// StopParticipantSessions godoc
//
//	@Summary		Stop a participant's sessions in the class
//	@Description	Stops the participant's running sessions in the class (those named, or all of them); a persistent session keeps its disk. Any session named outside the class refuses the whole call.
//	@Tags			terminals
//	@Accept			json
//	@Produce		json
//	@Param			id		path	string									true	"Class-group ID"
//	@Param			userId	path	string									true	"Participant user ID"
//	@Param			body	body	dto.StopParticipantSessionsInput		false	"Sessions to stop"
//	@Security		Bearer
//	@Success		200	{object}	map[string]any	"results per session"
//	@Failure		403	{object}	errors.APIError
//	@Router			/class-groups/{id}/participants/{userId}/stop-sessions [post]
func (tc *terminalController) StopParticipantSessions(ctx *gin.Context) {
	actor, scope, ok := tc.classParticipant(ctx)
	if !ok {
		return
	}
	var input dto.StopParticipantSessionsInput
	if err := ctx.ShouldBindJSON(&input); err != nil && !errors.Is(err, io.EOF) {
		apiErrors.Respond(ctx, http.StatusBadRequest, "Invalid body")
		return
	}
	results, err := tc.participants.StopSessions(actor, ctx.Param("userId"), scope, input.SessionIDs)
	if err != nil {
		httperrors.WriteParticipantControlError(ctx, err)
		return
	}
	httperrors.WriteSessionActionResults(ctx, results)
}

// SetParticipantNetwork godoc
//
//	@Summary		Cut or restore a participant's internet access
//	@Description	Toggles internet access on the participant's sessions in the class composed with the network feature, running or stopped (those named, or all of them). Other sessions are reported skipped. tt-backend's refusal codes are kept per session.
//	@Tags			terminals
//	@Accept			json
//	@Produce		json
//	@Param			id		path	string							true	"Class-group ID"
//	@Param			userId	path	string							true	"Participant user ID"
//	@Param			body	body	dto.SetParticipantNetworkInput	true	"enabled, and optionally the sessions"
//	@Security		Bearer
//	@Success		200	{object}	map[string]any	"results per session"
//	@Failure		400	{object}	errors.APIError
//	@Failure		403	{object}	errors.APIError
//	@Failure		409	{object}	map[string]any	"every session refused (code)"
//	@Failure		502	{object}	map[string]any	"every session failed (code)"
//	@Router			/class-groups/{id}/participants/{userId}/network [put]
func (tc *terminalController) SetParticipantNetwork(ctx *gin.Context) {
	actor, scope, ok := tc.classParticipant(ctx)
	if !ok {
		return
	}
	var input dto.SetParticipantNetworkInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		apiErrors.Respond(ctx, http.StatusBadRequest, "enabled is required")
		return
	}
	results, err := tc.participants.SetNetwork(actor, ctx.Param("userId"), scope, *input.Enabled, input.SessionIDs)
	if err != nil {
		httperrors.WriteParticipantControlError(ctx, err)
		return
	}
	httperrors.WriteSessionActionResults(ctx, results)
}

// SetParticipantSessionBlock godoc
//
//	@Summary		Block a participant from starting sessions in the class
//	@Description	Blocks (or unblocks) the participant's session starts through this class: its assignments and its bulk starts. stop_running also stops their running sessions in the class.
//	@Tags			terminals
//	@Accept			json
//	@Produce		json
//	@Param			id		path	string						true	"Class-group ID"
//	@Param			userId	path	string						true	"Participant user ID"
//	@Param			body	body	dto.SetSessionBlockInput	true	"blocked, reason (500 characters max), stop_running"
//	@Security		Bearer
//	@Success		200	{object}	map[string]any
//	@Failure		400	{object}	errors.APIError
//	@Failure		403	{object}	errors.APIError
//	@Router			/class-groups/{id}/participants/{userId}/session-block [put]
func (tc *terminalController) SetParticipantSessionBlock(ctx *gin.Context) {
	actor, scope, ok := tc.classParticipant(ctx)
	if !ok {
		return
	}
	input, ok := httperrors.BindSessionBlock(ctx)
	if !ok {
		return
	}
	results, _, err := tc.participants.SetSessionBlock(actor, ctx.Param("userId"), scope, *input.Blocked, input.Reason, input.StopRunning)
	httperrors.WriteSessionBlockResult(ctx, *input.Blocked, results, err)
}
