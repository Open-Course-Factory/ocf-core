package services

// participantControlsService.go — what a trainer may do to a participant's
// sessions (#580): stop them, cut or restore their internet access, and block
// the participant from starting sessions. One service for the three places it
// is offered from (a class, an organization, the platform admin), so scoping,
// audit and acting have one home.
//
// Every action is audited BEFORE it runs, all its audit rows in one
// transaction: an audit failure refuses the whole action and leaves nothing
// behind.

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	auditModels "soli/formations/src/audit/models"
	"soli/formations/src/auth/access"
	"soli/formations/src/auth/casdoor"
	auditServices "soli/formations/src/audit/services"
	groupModels "soli/formations/src/groups/models"
	orgModels "soli/formations/src/organizations/models"
	paymentServices "soli/formations/src/payment/services"
	"soli/formations/src/terminalTrainer/models"
	"soli/formations/src/terminalTrainer/repositories"
	"soli/formations/src/utils"
)

// ErrParticipantForbidden refuses an action on a participant: the caller does
// not manage the scope, or the target is the caller, a manager or owner of
// the scope, not an active member of it, or a session named is not theirs in
// it. One error for all, so a refusal tells nothing about which.
var ErrParticipantForbidden = errors.New("participant_forbidden")

// ErrSessionBlockReasonTooLong refuses a block reason over
// MaxSessionBlockReasonLength characters.
var ErrSessionBlockReasonTooLong = errors.New("reason_too_long")

// MaxSessionBlockReasonLength is the size of session_blocks.reason.
const MaxSessionBlockReasonLength = 500

// ControlActor is who acts: the caller, whether they are a platform
// administrator, and the administrator impersonating them, if any.
type ControlActor struct {
	UserID         string
	ImpersonatorID string
	IsAdmin        bool
}

// ControlScope is where an action applies: its block scope (class,
// organization or platform) and the sessions it reaches, those launched in
// OrgID. A global scope reaches every session of the participant.
type ControlScope struct {
	Type  string     // models.SessionBlockScope*
	ID    *uuid.UUID // the class or organization; nil when global
	OrgID *uuid.UUID // the organization sessions must run in; unused when global
}

// sessionsOf scopes a terminals query to userID's sessions in the scope. A
// class with no organization reaches no session, as on the supervision wall.
func (s ControlScope) sessionsOf(db *gorm.DB, userID string) *gorm.DB {
	query := db.Model(&models.Terminal{}).Where("user_id = ?", userID)
	if s.Type == models.SessionBlockScopeGlobal {
		return query
	}
	return query.Scopes(models.SupervisableByGroupOrgScope(s.OrgID))
}

// GlobalControlScope is the platform scope of an administrator's block.
func GlobalControlScope() ControlScope {
	return ControlScope{Type: models.SessionBlockScopeGlobal}
}

// ClassControlScope checks that targetUserID is a participant actor may act on
// in the class groupID — an active member, below manager and not its owner
// (a platform administrator may also act on its managers and owner), never
// the actor — and returns the class scope. The caller must already have
// checked that actor manages the class.
func ClassControlScope(db *gorm.DB, groupID uuid.UUID, actor ControlActor, targetUserID string) (ControlScope, error) {
	if targetUserID == "" || targetUserID == actor.UserID {
		return ControlScope{}, ErrParticipantForbidden
	}
	var group groupModels.ClassGroup
	if err := db.Select("id", "owner_user_id", "organization_id").First(&group, "id = ?", groupID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ControlScope{}, ErrParticipantForbidden
		}
		return ControlScope{}, err
	}
	var member groupModels.GroupMember
	err := db.Where("group_id = ? AND user_id = ? AND is_active = ?", groupID, targetUserID, true).First(&member).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ControlScope{}, ErrParticipantForbidden
	}
	if err != nil {
		return ControlScope{}, err
	}
	if !actor.IsAdmin {
		if member.Role != groupModels.GroupMemberRoleMember || group.OwnerUserID == targetUserID {
			return ControlScope{}, ErrParticipantForbidden
		}
		manages, err := managesOrganization(db, group.OrganizationID, targetUserID)
		if err != nil {
			return ControlScope{}, err
		}
		if manages {
			return ControlScope{}, ErrParticipantForbidden
		}
	}
	return ControlScope{Type: models.SessionBlockScopeGroup, ID: &groupID, OrgID: group.OrganizationID}, nil
}

// managesOrganization reports whether userID owns the organization orgID or
// is an active manager or owner member of it. A nil organization has none.
func managesOrganization(db *gorm.DB, orgID *uuid.UUID, userID string) (bool, error) {
	if orgID == nil {
		return false, nil
	}
	var org orgModels.Organization
	err := db.Select("id", "owner_user_id").First(&org, "id = ?", *orgID).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, err
	}
	if err == nil && org.OwnerUserID == userID {
		return true, nil
	}
	var n int64
	err = db.Model(&orgModels.OrganizationMember{}).
		Where("organization_id = ? AND user_id = ? AND is_active = ? AND role IN ?", *orgID, userID, true,
			[]orgModels.OrganizationMemberRole{orgModels.OrgRoleManager, orgModels.OrgRoleOwner}).
		Count(&n).Error
	return n > 0, err
}

// OrgControlScope checks that actor manages the organization orgID (a platform
// administrator always does) and that targetUserID is a participant of it
// actor may act on — an active member, below manager, not its owner and not a
// platform administrator (a platform administrator may also act on those),
// never the actor — and returns the organization scope.
func OrgControlScope(db *gorm.DB, orgID uuid.UUID, actor ControlActor, targetUserID string) (ControlScope, error) {
	if !actor.IsAdmin {
		var caller orgModels.OrganizationMember
		err := db.Where("organization_id = ? AND user_id = ? AND is_active = ?", orgID, actor.UserID, true).First(&caller).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ControlScope{}, ErrParticipantForbidden
		}
		if err != nil {
			return ControlScope{}, err
		}
		if !caller.IsManager() {
			return ControlScope{}, ErrParticipantForbidden
		}
	}
	if targetUserID == "" || targetUserID == actor.UserID {
		return ControlScope{}, ErrParticipantForbidden
	}
	var org orgModels.Organization
	if err := db.Select("id", "owner_user_id").First(&org, "id = ?", orgID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ControlScope{}, ErrParticipantForbidden
		}
		return ControlScope{}, err
	}
	var member orgModels.OrganizationMember
	err := db.Where("organization_id = ? AND user_id = ? AND is_active = ?", orgID, targetUserID, true).First(&member).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ControlScope{}, ErrParticipantForbidden
	}
	if err != nil {
		return ControlScope{}, err
	}
	if !actor.IsAdmin && (member.IsManager() || org.OwnerUserID == targetUserID) {
		return ControlScope{}, ErrParticipantForbidden
	}
	if !actor.IsAdmin {
		// Same lookup as organizations/services' rank rule (which this package cannot
		// import: it imports us); fails closed, an unknown role counts as administrator.
		if casdoor.Enforcer == nil {
			return ControlScope{}, ErrParticipantForbidden
		}
		roles, err := casdoor.Enforcer.GetRolesForUser(targetUserID)
		if err != nil || access.IsAdmin(roles) {
			return ControlScope{}, ErrParticipantForbidden
		}
	}
	return ControlScope{Type: models.SessionBlockScopeOrg, ID: &orgID, OrgID: &orgID}, nil
}

// SessionActionResult is the outcome of an action on one session. Skipped
// marks a session the action does not apply to (not running, no network
// feature). Code is the reason of a failure; Status its HTTP status.
type SessionActionResult struct {
	SessionID string `json:"session_id"`
	OK        bool   `json:"ok"`
	Skipped   bool   `json:"skipped,omitempty"`
	Code      string `json:"code,omitempty"`
	Status    int    `json:"-"`
}

// networkCodeStatus maps tt-backend's network toggle codes to the status
// ocf-core answers with.
var networkCodeStatus = map[string]int{
	"feature_not_in_session": http.StatusConflict,
	"session_not_running":    http.StatusConflict,
	"vm_not_supported":       http.StatusConflict,
	"network_renew_failed":   http.StatusBadGateway,
	"network_still_present":  http.StatusInternalServerError,
	"state_not_recorded":     http.StatusInternalServerError,
}

// ParticipantControlsService stops a participant's sessions, toggles their
// internet access and blocks their session starts.
type ParticipantControlsService struct {
	db         *gorm.DB
	proxy      *terminalProxyClient
	lifecycle  *terminalLifecycleService
	repository repositories.TerminalRepository
}

// NewParticipantControlsService reads tt-backend's URL and admin key from the
// environment, as NewTerminalTrainerService does. It builds only the pieces it
// uses: the full service also starts a background enum refresh.
func NewParticipantControlsService(db *gorm.DB) *ParticipantControlsService {
	repository := repositories.NewTerminalRepository(db)
	proxy := newTerminalProxyClient(repository)
	quota := paymentServices.NewQuotaService(db, paymentServices.NewEffectivePlanService(db))
	lifecycle := newTerminalLifecycleService(proxy, newTerminalSyncService(proxy, repository, db), repository, quota, db)
	return &ParticipantControlsService{db: db, proxy: proxy, lifecycle: lifecycle, repository: repository}
}

// sessionsFor returns the sessions an action on targetUserID covers in scope:
// the ones named in sessionIDs, every one of which must be the target's in
// the scope (ErrParticipantForbidden otherwise, nothing acted on), or, when
// none is named, the target's sessions in the scope in one of states.
func (s *ParticipantControlsService) sessionsFor(scope ControlScope, targetUserID string, sessionIDs []string, states []models.TerminalState) ([]models.Terminal, error) {
	var terminals []models.Terminal
	query := scope.sessionsOf(s.db, targetUserID)
	if len(sessionIDs) == 0 {
		err := query.Where("state IN ?", states).Order("created_at").Find(&terminals).Error
		return terminals, err
	}
	unique := slices.Compact(slices.Sorted(slices.Values(sessionIDs)))
	if err := query.Where("session_id IN ?", unique).Order("created_at").Find(&terminals).Error; err != nil {
		return nil, err
	}
	if len(terminals) != len(unique) {
		return nil, ErrParticipantForbidden
	}
	return terminals, nil
}

// StopSessions stops targetUserID's running sessions in scope (those named in
// sessionIDs, or all of them), through the same lifecycle as the learner's
// own stop: a persistent session keeps its disk.
func (s *ParticipantControlsService) StopSessions(actor ControlActor, targetUserID string, scope ControlScope, sessionIDs []string) ([]SessionActionResult, error) {
	terminals, err := s.sessionsFor(scope, targetUserID, sessionIDs, []models.TerminalState{models.StateRunning})
	if err != nil {
		return nil, err
	}
	results := make([]SessionActionResult, 0, len(terminals))
	var toStop []models.Terminal
	for _, t := range terminals {
		if t.State != models.StateRunning {
			results = append(results, SessionActionResult{SessionID: t.SessionID, Skipped: true})
			continue
		}
		toStop = append(toStop, t)
	}
	if err := s.auditThen(nil, stopAudits(actor, targetUserID, scope, toStop)); err != nil {
		return nil, err
	}
	return append(results, s.stop(toStop)...), nil
}

// SetNetwork cuts (enabled false) or restores targetUserID's internet access on
// their sessions in scope composed with the network feature, running or
// stopped (a stopped one comes back as recorded). Without sessionIDs, every
// such session, and in a class the cut is sticky: it is recorded for the
// participant (participant_network_cuts) so every session composed for them
// in the class's organization starts without internet until it is lifted the
// same way. A session without the feature, or in another state, is skipped.
//
// A cut drops the session's exposed ports before tt-backend is asked, whatever
// it answers: their address dies with the cut. A restore drops them on
// success: a re-attached NIC takes a new address.
func (s *ParticipantControlsService) SetNetwork(actor ControlActor, targetUserID string, scope ControlScope, enabled bool, sessionIDs []string) ([]SessionActionResult, error) {
	toggleable := []models.TerminalState{models.StateRunning, models.StateStopped}
	terminals, err := s.sessionsFor(scope, targetUserID, sessionIDs, toggleable)
	if err != nil {
		return nil, err
	}
	results := make([]SessionActionResult, 0, len(terminals))
	var toToggle []models.Terminal
	for _, t := range terminals {
		if !t.ComposedWithNetwork() || !slices.Contains(toggleable, t.State) {
			results = append(results, SessionActionResult{SessionID: t.SessionID, Skipped: true})
			continue
		}
		toToggle = append(toToggle, t)
	}

	event := auditModels.AuditEventTerminalNetworkDisabled
	if enabled {
		event = auditModels.AuditEventTerminalNetworkEnabled
	}
	// A sticky cut or restore is audited on each session it toggles, marked
	// sticky; with no session to toggle, once, naming none.
	sticky := len(sessionIDs) == 0 && scope.Type == models.SessionBlockScopeGroup && scope.ID != nil
	var extra map[string]any
	if sticky {
		extra = map[string]any{"sticky": true}
	}
	entries := make([]auditModels.AuditLogCreate, 0, len(toToggle)+1)
	for _, t := range toToggle {
		entries = append(entries, controlAudit(event, actor, targetUserID, scope, t.SessionID, extra))
	}
	if sticky && len(toToggle) == 0 {
		entries = append(entries, controlAudit(event, actor, targetUserID, scope, "", extra))
	}
	var recordCut func(tx *gorm.DB) error
	if sticky {
		recordCut = func(tx *gorm.DB) error {
			if enabled {
				return models.DeleteNetworkCut(tx, targetUserID, *scope.ID)
			}
			return models.UpsertNetworkCut(tx, targetUserID, *scope.ID, scope.OrgID, actor.UserID)
		}
	}
	if err := s.auditThen(recordCut, entries); err != nil {
		return nil, err
	}

	for _, t := range toToggle {
		results = append(results, s.toggleNetwork(t, enabled))
	}
	return results, nil
}

func (s *ParticipantControlsService) toggleNetwork(t models.Terminal, enabled bool) SessionActionResult {
	if !enabled {
		s.dropExposedPorts(t.SessionID)
	}
	if err := s.proxy.setSessionNetworkInAPI(t.SessionID, enabled); err != nil {
		code := "network_toggle_failed"
		var toggleErr *NetworkToggleError
		if errors.As(err, &toggleErr) && toggleErr.Code != "" {
			code = toggleErr.Code
		}
		status, known := networkCodeStatus[code]
		if !known {
			status = http.StatusBadGateway
		}
		utils.Warn("network toggle (enabled=%t) of session %s failed: %v", enabled, t.SessionID, err)
		return SessionActionResult{SessionID: t.SessionID, Code: code, Status: status}
	}
	if err := s.db.Model(&models.Terminal{}).Where("id = ?", t.ID).Update("network_disabled", !enabled).Error; err != nil {
		utils.Error("network of session %s toggled (enabled=%t) but not recorded: %v", t.SessionID, enabled, err)
		return SessionActionResult{SessionID: t.SessionID, Code: "state_not_recorded", Status: http.StatusInternalServerError}
	}
	if enabled {
		s.dropExposedPorts(t.SessionID)
	}
	return SessionActionResult{SessionID: t.SessionID, OK: true}
}

func (s *ParticipantControlsService) dropExposedPorts(sessionID string) {
	if err := s.repository.DeleteExposedPortsBySessionID(sessionID); err != nil {
		utils.Warn("failed to drop exposed ports of session %s on a network toggle: %v", sessionID, err)
	}
}

// SetSessionBlock blocks targetUserID from starting sessions in scope (or
// lifts the block), recording reason and actor. A second block updates the
// first. stopRunning also stops the target's running sessions in scope. The
// block commits with its audit rows, the stops run after. removed reports an
// unblock that lifted a block: lifting none changes and audits nothing.
func (s *ParticipantControlsService) SetSessionBlock(actor ControlActor, targetUserID string, scope ControlScope, blocked bool, reason string, stopRunning bool) (results []SessionActionResult, removed bool, err error) {
	if len([]rune(reason)) > MaxSessionBlockReasonLength {
		return nil, false, ErrSessionBlockReasonTooLong
	}
	if !blocked {
		err := s.db.Transaction(func(tx *gorm.DB) error {
			deleted := blockQuery(tx, targetUserID, scope).Delete(&models.SessionBlock{})
			if deleted.Error != nil {
				return deleted.Error
			}
			if deleted.RowsAffected == 0 {
				return nil
			}
			removed = true
			entry := controlAudit(auditModels.AuditEventUserSessionBlockCleared, actor, targetUserID, scope, "", nil)
			if err := auditServices.LogAll(tx, []auditModels.AuditLogCreate{entry}); err != nil {
				return fmt.Errorf("audit before acting: %w", err)
			}
			return nil
		})
		if err != nil {
			return nil, false, err
		}
		return []SessionActionResult{}, removed, nil
	}

	var toStop []models.Terminal
	if stopRunning {
		if toStop, err = s.sessionsFor(scope, targetUserID, nil, []models.TerminalState{models.StateRunning}); err != nil {
			return nil, false, err
		}
	}
	extra := map[string]any{"has_reason": reason != "", "stop_running": stopRunning}
	entries := append([]auditModels.AuditLogCreate{controlAudit(auditModels.AuditEventUserSessionBlockSet, actor, targetUserID, scope, "", extra)},
		stopAudits(actor, targetUserID, scope, toStop)...)
	if err := s.auditThen(func(tx *gorm.DB) error {
		return upsertBlock(tx, targetUserID, scope, reason, actor.UserID)
	}, entries); err != nil {
		return nil, false, err
	}
	return s.stop(toStop), false, nil
}

func blockQuery(tx *gorm.DB, userID string, scope ControlScope) *gorm.DB {
	query := tx.Where("user_id = ? AND scope_type = ?", userID, scope.Type)
	if scope.ID == nil {
		return query.Where("scope_id IS NULL")
	}
	return query.Where("scope_id = ?", *scope.ID)
}

// upsertBlock writes the one block of userID in scope, updating its reason and
// author when it exists. A global block's uniqueness is the partial index
// MigrateSessionBlockGlobalIndex; the others', the composite one.
func upsertBlock(tx *gorm.DB, userID string, scope ControlScope, reason, blockedBy string) error {
	conflict := clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "scope_type"}, {Name: "scope_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"reason", "blocked_by"}),
	}
	if scope.Type == models.SessionBlockScopeGlobal {
		conflict.Columns = []clause.Column{{Name: "user_id"}}
		// A literal, not a bound value: PostgreSQL infers the partial index
		// from the predicate's text.
		conflict.TargetWhere = clause.Where{Exprs: []clause.Expression{
			clause.Expr{SQL: "scope_type = 'global'"},
		}}
	}
	return tx.Clauses(conflict).Create(&models.SessionBlock{
		UserID: userID, ScopeType: scope.Type, ScopeID: scope.ID, Reason: reason, BlockedBy: blockedBy,
	}).Error
}

// auditThen writes entries and runs also (when given) in one transaction.
func (s *ParticipantControlsService) auditThen(also func(tx *gorm.DB) error, entries []auditModels.AuditLogCreate) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := auditServices.LogAll(tx, entries); err != nil {
			return fmt.Errorf("audit before acting: %w", err)
		}
		if also != nil {
			return also(tx)
		}
		return nil
	})
}

// stop stops each running terminal through the lifecycle.
func (s *ParticipantControlsService) stop(terminals []models.Terminal) []SessionActionResult {
	results := make([]SessionActionResult, 0, len(terminals))
	for _, t := range terminals {
		if err := s.lifecycle.StopSession(t.SessionID); err != nil {
			utils.Error("stopping session %s for a trainer failed: %v", t.SessionID, err)
			results = append(results, SessionActionResult{SessionID: t.SessionID, Code: "stop_failed", Status: http.StatusInternalServerError})
			continue
		}
		results = append(results, SessionActionResult{SessionID: t.SessionID, OK: true})
	}
	return results
}

func stopAudits(actor ControlActor, targetUserID string, scope ControlScope, terminals []models.Terminal) []auditModels.AuditLogCreate {
	entries := make([]auditModels.AuditLogCreate, 0, len(terminals))
	for _, t := range terminals {
		entries = append(entries, controlAudit(auditModels.AuditEventTerminalStoppedByTrainer, actor, targetUserID, scope, t.SessionID, nil))
	}
	return entries
}

// controlAudit builds the audit row of one action, written before it runs
// (status "requested"). Identities live in the typed columns only (ActorID,
// TargetID, OnBehalfOfID, where the ids parse: Casdoor ids are UUIDs in
// production); Metadata keeps the scope, the class, the organization and, for
// a session action, the session.
func controlAudit(event auditModels.AuditEventType, actor ControlActor, targetUserID string, scope ControlScope, sessionID string, extra map[string]any) auditModels.AuditLogCreate {
	meta := map[string]any{"scope": scope.Type}
	entry := auditModels.AuditLogCreate{
		EventType:  event,
		Severity:   auditModels.AuditSeverityWarning,
		TargetType: "user",
		Action:     string(event),
		Status:     "requested",
	}
	if scope.Type == models.SessionBlockScopeGroup {
		meta["group_id"] = scope.ID.String()
		entry.GroupID = scope.ID
	}
	// An organization scope's OrgID is the organization itself.
	if scope.Type != models.SessionBlockScopeGlobal && scope.OrgID != nil {
		meta["organization_id"] = scope.OrgID.String()
		entry.OrganizationID = scope.OrgID
	}
	if sessionID != "" {
		meta["session_id"] = sessionID
		entry.SessionID = sessionID
		entry.TargetType = "terminal_session"
		entry.TargetName = sessionID
	}
	if id, err := uuid.Parse(targetUserID); err == nil {
		entry.TargetID = &id
	}
	if id, err := uuid.Parse(actor.UserID); err == nil {
		entry.ActorID = &id
	}
	// The human administrator is the actor, on behalf of the impersonated user.
	auditServices.ApplyImpersonation(&entry, actor.ImpersonatorID)
	for k, v := range extra {
		meta[k] = v
	}
	raw, _ := json.Marshal(meta)
	entry.Metadata = string(raw)
	return entry
}
