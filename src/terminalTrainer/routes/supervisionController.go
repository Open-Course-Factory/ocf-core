package terminalController

// supervisionController.go — HTTP + WebSocket handlers for terminal supervision
// (issue #425). The security-critical decisions live in supervision.go; this file
// is the transport: a group-scoped session listing, and a frame-aware WebSocket
// broker that observes a learner's tt-backend console and brokers take-hand.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	auditModels "soli/formations/src/audit/models"
	auditServices "soli/formations/src/audit/services"
	"soli/formations/src/auth/errors"
	paymentModels "soli/formations/src/payment/models"
	paymentServices "soli/formations/src/payment/services"
)

// supervisionReauthInterval is how often a live supervise stream re-checks that
// the trainer is still authorized (M1). On failure the stream is torn down.
const supervisionReauthInterval = 30 * time.Second

// isAdminFromRoles reports whether the roles slice carries the platform admin role.
func isAdminFromRoles(roles []string) bool {
	for _, r := range roles {
		if r == "administrator" {
			return true
		}
	}
	return false
}

// BuildSuperviseWSURL builds the tt-backend console WebSocket URL for supervision.
// It mirrors ConnectConsole's URL construction (https→wss, else ws; path
// /{apiVersion}[/{instanceType}]/console) and ALWAYS connects as a read-only
// observer with control frames on (role=observer, control=1) — that query is the
// entire read-only guarantee of supervision, so this is the single source of truth
// for the URL build.
func BuildSuperviseWSURL(terminalTrainerURL, apiVersion, instanceType, sessionID string) (string, error) {
	u, err := url.Parse(terminalTrainerURL)
	if err != nil {
		return "", err
	}
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	path := fmt.Sprintf("/%s", apiVersion)
	if instanceType != "" {
		path += fmt.Sprintf("/%s", instanceType)
	}
	path += "/console"
	u.Path = path

	q := u.Query()
	q.Set("id", sessionID)
	q.Set("role", "observer")
	q.Set("control", "1")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// BuildConsoleWSURL builds the tt-backend console WebSocket URL for a LEARNER's
// own console (ConnectConsole), mirroring its existing construction (https→wss,
// else ws; path /{apiVersion}[/{instanceType}]/console; query id=<sessionID>). It
// appends control=1 IFF the session is supervisable, which is what activates the
// learner's mandatory "being watched" indicator. Scope is base URL + id + control
// only — the handler still appends width/height, so a non-supervisable URL stays
// byte-for-byte identical to today's.
func BuildConsoleWSURL(terminalTrainerURL, apiVersion, instanceType, sessionID string, supervisable bool) (string, error) {
	u, err := url.Parse(terminalTrainerURL)
	if err != nil {
		return "", err
	}
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	path := fmt.Sprintf("/%s", apiVersion)
	if instanceType != "" {
		path += fmt.Sprintf("/%s", instanceType)
	}
	path += "/console"
	u.Path = path

	q := u.Query()
	q.Set("id", sessionID)
	if supervisable {
		q.Set("control", "1")
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// GetGroupTerminalSessions godoc
//
//	@Summary		List a class-group's active terminal sessions (for supervision)
//	@Description	Returns the active terminal sessions of the group's members. Manager+ of the group (or admin) only. The listing is strictly scoped to the given group — sessions from other groups never leak.
//	@Tags			terminals
//	@Produce		json
//	@Param			id	path	string	true	"Class-group ID"
//	@Security		Bearer
//	@Success		200	{array}		models.Terminal
//	@Failure		403	{object}	errors.APIError	"Not a manager of this group"
//	@Router			/class-groups/{id}/terminal-sessions [get]
func (tc *terminalController) GetGroupTerminalSessions(ctx *gin.Context) {
	groupID := ctx.Param("id")
	userID := ctx.GetString("userId")
	isAdmin := isAdminFromRoles(ctx.GetStringSlice("userRoles"))

	sessions, ok := ListGroupSupervisionSessions(tc.db, groupID, userID, isAdmin)
	if !ok {
		ctx.JSON(http.StatusForbidden, &errors.APIError{
			ErrorCode:    http.StatusForbidden,
			ErrorMessage: "You are not a manager of this group",
		})
		return
	}
	ctx.JSON(http.StatusOK, sessions)
}

// supervisionControlMsg is the trainer's in-band control envelope. Only the
// allow-listed types below are ever acted on; nothing else from the client is
// interpreted as a command (and no client bytes are forwarded upstream as control).
type supervisionControlMsg struct {
	Type string `json:"type"`
}

// resolveSupervisionPlan resolves the caller's effective plan (same path used to
// gate at WS open); nil when it cannot be resolved.
func (tc *terminalController) resolveSupervisionPlan(userID string) *paymentModels.SubscriptionPlan {
	res, err := paymentServices.NewEffectivePlanService(tc.db).GetUserEffectivePlan(userID, nil)
	if err != nil || res == nil {
		return nil
	}
	return res.Plan
}

// SuperviseSession godoc
//
//	@Summary		Supervise a learner's terminal (WebSocket, observer + take-hand)
//	@Description	Opens a read-only observer stream onto a learner's terminal for a group manager+, and brokers take-hand/release-hand via the trainer's in-band control frames. The learner's group is derived server-side from the session record (never client-supplied); requires a plan with session supervision. Authorization is re-checked periodically for the life of the stream.
//	@Tags			terminals
//	@Param			id	path	string	true	"Learner terminal session ID"
//	@Security		Bearer
//	@Success		101	{string}	string	"Switching Protocols (WebSocket)"
//	@Failure		403	{object}	errors.APIError	"Not authorized to supervise / plan lacks supervision"
//	@Failure		404	{object}	errors.APIError	"Session not found"
//	@Router			/terminals/{id}/supervise [get]
func (tc *terminalController) SuperviseSession(ctx *gin.Context) {
	sessionID := ctx.Param("id")
	if sessionID == "" {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{ErrorCode: http.StatusBadRequest, ErrorMessage: "Session ID is required"})
		return
	}
	userID := ctx.GetString("userId")
	isAdmin := isAdminFromRoles(ctx.GetStringSlice("userRoles"))

	// Authorization: derive the learner's group SERVER-SIDE and require manager+.
	groupID, ok := HasSupervisionAccess(tc.db, userID, isAdmin, sessionID)
	if !ok {
		ctx.JSON(http.StatusForbidden, &errors.APIError{ErrorCode: http.StatusForbidden, ErrorMessage: "You are not authorized to supervise this session"})
		return
	}

	// Plan gate (ANDed with authz): a valid manager on a plan without the feature
	// is still denied.
	if !PlanAllowsSupervision(tc.resolveSupervisionPlan(userID)) {
		ctx.JSON(http.StatusForbidden, &errors.APIError{ErrorCode: http.StatusForbidden, ErrorMessage: "Your plan does not include terminal supervision"})
		return
	}

	auditSvc := auditServices.NewAuditService(tc.db)
	if _, err := StartSupervision(tc.db, auditSvc, userID, isAdmin, sessionID); err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{ErrorCode: http.StatusInternalServerError, ErrorMessage: "Failed to start supervision"})
		return
	}

	// Resolve the terminal and the OWNER's key (tt-backend authorizes by the
	// session owner's key; the supervisor rides it — ocf-core is the authorizer).
	terminal, err := tc.service.GetSessionInfo(sessionID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{ErrorCode: http.StatusNotFound, ErrorMessage: "Session not found"})
		return
	}
	ownerKey, keyErr := tc.service.GetUserKey(terminal.UserID)
	if keyErr != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{ErrorCode: http.StatusInternalServerError, ErrorMessage: "Terminal owner's API key not found"})
		return
	}

	instanceType := terminal.InstanceType
	if instanceType == "" {
		instanceType = tc.terminalType
	}
	wsURL, buildErr := BuildSuperviseWSURL(tc.terminalTrainerURL, tc.apiVersion, instanceType, terminal.SessionID)
	if buildErr != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{ErrorCode: http.StatusInternalServerError, ErrorMessage: "Invalid terminal trainer URL"})
		return
	}

	clientConn, err := upgrader.Upgrade(ctx.Writer, ctx.Request, nil)
	if err != nil {
		return
	}
	defer clientConn.Close()

	headers := make(http.Header)
	headers.Set("X-API-Key", ownerKey.APIKey)
	upstream, _, err := websocket.DefaultDialer.Dial(wsURL, headers)
	if err != nil {
		clientConn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "Failed to connect to terminal trainer"))
		return
	}
	defer upstream.Close()

	// Teardown closes BOTH connections exactly once, so either side ending unblocks
	// the other (no half-open leak). done stops the background tickers.
	var closeOnce sync.Once
	teardown := func() { closeOnce.Do(func() { clientConn.Close(); upstream.Close() }) }
	defer teardown()
	done := make(chan struct{})
	var doneOnce sync.Once
	stop := func() { doneOnce.Do(func() { close(done) }) }
	defer stop()

	// Shared broker state: our tt-backend attachment id and whether we currently
	// hold the interactive hand.
	var stMu sync.Mutex
	attachmentID := ""
	promoted := false

	// Keepalive pings to the browser (matches ConnectConsole / tt-backend cadence).
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := clientConn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
					return
				}
			case <-done:
				return
			}
		}
	}()

	// M1: periodic re-authorization. On loss of access, demote (if promoted) then
	// tear the stream down.
	go func() {
		ticker := time.NewTicker(supervisionReauthInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if SupervisionStillAuthorized(tc.db, userID, isAdmin, sessionID) {
					continue
				}
				stMu.Lock()
				wasPromoted, aid := promoted, attachmentID
				stMu.Unlock()
				if wasPromoted && aid != "" {
					if perr := tc.patchAttachmentRole(terminal.SessionID, aid, "observer", ownerKey.APIKey); perr != nil {
						slog.Error("supervision demote-on-deauth PATCH failed", "session_id", sessionID, "err", perr)
					}
				}
				slog.Warn("supervision re-authorization failed; tearing down", "session_id", sessionID, "user_id", userID)
				teardown()
				return
			case <-done:
				return
			}
		}
	}()

	// Upstream (tt-backend) → client: forward everything, capture our attachment id
	// from the first control frame that carries one.
	go func() {
		defer teardown()
		for {
			mt, data, rerr := upstream.ReadMessage()
			if rerr != nil {
				return
			}
			if mt == websocket.BinaryMessage {
				// M2: tt-backend delivers a self-snapshot "joined" control event on
				// control-connect, so the FIRST control frame carrying an
				// attachment_id is ours. The explicit-self-frame hardening (never
				// mistaking another attachment's event for our own) is tracked as
				// tt-backend #126; until then the first-frame heuristic holds
				// because the self-snapshot precedes any other join broadcast.
				var ev struct {
					AttachmentID string `json:"attachment_id"`
				}
				if json.Unmarshal(data, &ev) == nil && ev.AttachmentID != "" {
					stMu.Lock()
					if attachmentID == "" {
						attachmentID = ev.AttachmentID
					}
					stMu.Unlock()
				}
			}
			if werr := clientConn.WriteMessage(mt, data); werr != nil {
				return
			}
		}
	}()

	// Client (trainer) → upstream: frame-aware allow-list. Binary client frames are
	// NEVER forwarded upstream; recognized supervision control envelopes are brokered
	// here; anything else is forwarded as terminal input (applied by tt-backend only
	// once this attachment is interactive).
	for {
		mt, data, rerr := clientConn.ReadMessage()
		if rerr != nil {
			break
		}
		if mt == websocket.BinaryMessage {
			continue // never forward raw client control bytes upstream
		}
		var msg supervisionControlMsg
		if json.Unmarshal(data, &msg) == nil && (msg.Type == "take_hand" || msg.Type == "release_hand") {
			stMu.Lock()
			aid := attachmentID
			stMu.Unlock()
			if aid == "" {
				continue // our attachment id is not known yet; cannot address the PATCH
			}
			switch msg.Type {
			case "take_hand":
				// Re-authorize + re-check plan + audit-before-act (all inside
				// TakeHandForSupervision); any failure denies the promotion.
				if err := TakeHandForSupervision(tc.db, auditSvc, tc.resolveSupervisionPlan(userID), userID, isAdmin, sessionID, groupID); err != nil {
					continue // fail-closed: no escalation
				}
				if perr := tc.patchAttachmentRole(terminal.SessionID, aid, "interactive", ownerKey.APIKey); perr != nil {
					// Record the failed act distinctly; do NOT escalate.
					slog.Error("supervision take-hand PATCH failed", "session_id", sessionID, "err", perr)
					_ = auditSvc.Log(buildSupervisionAuditStatus(auditModels.AuditEventSupervisionTakeHand, userID, sessionID, groupID, "failed"))
					continue
				}
				stMu.Lock()
				promoted = true
				stMu.Unlock()
			case "release_hand":
				if perr := tc.patchAttachmentRole(terminal.SessionID, aid, "observer", ownerKey.APIKey); perr != nil {
					slog.Error("supervision release-hand PATCH failed", "session_id", sessionID, "err", perr)
					continue
				}
				stMu.Lock()
				promoted = false
				stMu.Unlock()
				_ = auditSvc.Log(buildSupervisionAudit(auditModels.AuditEventSupervisionReleased, userID, sessionID, groupID))
			}
			continue
		}
		if werr := upstream.WriteMessage(websocket.TextMessage, data); werr != nil {
			break
		}
	}

	// The observe stream closed: bound the supervision window in the audit trail.
	teardown()
	_ = EndSupervision(tc.db, auditSvc, userID, isAdmin, sessionID, groupID)
}

// patchAttachmentRole calls tt-backend's REST role-transition endpoint to promote
// or demote our attachment, authenticated with the session owner's key.
func (tc *terminalController) patchAttachmentRole(ttSessionID, attachmentID, role, apiKey string) error {
	base, err := url.Parse(tc.terminalTrainerURL)
	if err != nil {
		return err
	}
	base.Path = fmt.Sprintf("/%s/sessions/%s/console/attachments/%s", tc.apiVersion, ttSessionID, attachmentID)
	body, _ := json.Marshal(map[string]string{"role": role})
	req, err := http.NewRequest(http.MethodPatch, base.String(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("attachment role PATCH failed: %d", resp.StatusCode)
	}
	return nil
}
