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
	"soli/formations/src/auth/access"
	"soli/formations/src/auth/errors"
	paymentModels "soli/formations/src/payment/models"
	paymentServices "soli/formations/src/payment/services"
)

// supervisionReauthInterval is how often a live supervise stream re-checks that
// the trainer is still authorized (M1). On failure the stream is torn down.
const supervisionReauthInterval = 30 * time.Second

// BuildSuperviseWSURL builds the tt-backend console WebSocket URL for supervision.
// It mirrors ConnectConsole's URL construction (https→wss, else ws; path
// /{apiVersion}[/{instanceType}]/console) and ALWAYS connects as a read-only
// observer with control frames on (role=observer, control=1) — that query is the
// entire read-only guarantee of supervision, so this is the single source of truth
// for the URL build.
func BuildSuperviseWSURL(terminalTrainerURL, apiVersion, instanceType, sessionID string) (string, error) {
	u, err := consoleWSBase(terminalTrainerURL, apiVersion, instanceType)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("id", sessionID)
	q.Set("role", "observer")
	q.Set("control", "1")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// consoleWSBase parses the tt-backend base URL and returns it upgraded to the
// WebSocket scheme (https→wss, else ws) with the console path
// /{apiVersion}[/{instanceType}]/console set. Query is left empty — each console
// URL builder sets only its distinctive parameters. Single source of truth for the
// scheme upgrade + console path shared by the supervise and learner builders.
func consoleWSBase(terminalTrainerURL, apiVersion, instanceType string) (*url.URL, error) {
	u, err := url.Parse(terminalTrainerURL)
	if err != nil {
		return nil, err
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
	return u, nil
}

// BuildConsoleWSURL builds the tt-backend console WebSocket URL for a LEARNER's
// own console (ConnectConsole), mirroring its existing construction (https→wss,
// else ws; path /{apiVersion}[/{instanceType}]/console; query id=<sessionID>). It
// appends control=1 IFF the session is supervisable, which is what activates the
// learner's mandatory "being watched" indicator. Scope is base URL + id + control
// only — the handler still appends width/height, so a non-supervisable URL stays
// byte-for-byte identical to today's.
func BuildConsoleWSURL(terminalTrainerURL, apiVersion, instanceType, sessionID string, supervisable bool) (string, error) {
	u, err := consoleWSBase(terminalTrainerURL, apiVersion, instanceType)
	if err != nil {
		return "", err
	}
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
//	@Success		200	{array}		terminalController.SupervisionSession
//	@Failure		403	{object}	errors.APIError	"Not a manager of this group"
//	@Router			/class-groups/{id}/terminal-sessions [get]
func (tc *terminalController) GetGroupTerminalSessions(ctx *gin.Context) {
	groupID := ctx.Param("id")
	userID := ctx.GetString("userId")
	isAdmin := access.IsAdmin(ctx.GetStringSlice("userRoles"))

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

// bindSelfAttachmentID returns the attachment id the supervise proxy should be
// bound to after observing frame `data`, given it is currently bound to `current`
// ("" = not yet bound). It binds ONLY on the tt-backend self-snapshot control
// frame (type=="attachment" && event=="self") carrying a non-empty attachment_id,
// and only while not already bound (first self frame wins). Any other frame — a
// "joined" broadcast for another attachment, a non-attachment event, or
// malformed/empty bytes — returns `current` unchanged.
//
// The explicit self frame is the fix for the release-hand-demotes-the-learner
// incident: the old "first control frame carrying an attachment_id is ours"
// heuristic bound the learner console's id whenever tt-backend broadcast its
// "joined" event before any self-snapshot.
func bindSelfAttachmentID(current string, data []byte) string {
	if current != "" {
		return current
	}
	var ev struct {
		Type         string `json:"type"`
		Event        string `json:"event"`
		AttachmentID string `json:"attachment_id"`
	}
	if json.Unmarshal(data, &ev) != nil {
		return current
	}
	if ev.Type == "attachment" && ev.Event == "self" && ev.AttachmentID != "" {
		return ev.AttachmentID
	}
	return current
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

	target, ok := tc.prepareSupervision(ctx, sessionID)
	if !ok {
		return // prepareSupervision already wrote the error response
	}

	clientConn, err := upgrader.Upgrade(ctx.Writer, ctx.Request, nil)
	if err != nil {
		return
	}
	defer clientConn.Close()

	upstream, err := dialSupervisionUpstream(target.wsURL, target.ownerAPIKey)
	if err != nil {
		clientConn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "Failed to connect to terminal trainer"))
		return
	}
	defer upstream.Close()

	newSuperviseBroker(tc, target, clientConn, upstream).run()
}

// superviseTarget is the fully-resolved, authorized connection target for a
// supervision session, produced by prepareSupervision once authorization, the plan
// gate, and the owner-key / upstream-URL lookups have all passed.
type superviseTarget struct {
	auditSvc    auditServices.AuditService
	userID      string
	isAdmin     bool
	groupID     string
	sessionID   string // ocf-core session id (used for authz + audit)
	ttSessionID string // tt-backend session id (used for the console URL + PATCH)
	ownerAPIKey string
	wsURL       string
}

// prepareSupervision runs SuperviseSession's full pre-connection sequence:
// server-side authorization, the plan gate, the supervision-started audit, and
// resolving the terminal owner's key + upstream URL. On any failure it writes the
// appropriate JSON error to ctx and returns ok=false; the caller must not upgrade.
func (tc *terminalController) prepareSupervision(ctx *gin.Context, sessionID string) (*superviseTarget, bool) {
	userID := ctx.GetString("userId")
	isAdmin := access.IsAdmin(ctx.GetStringSlice("userRoles"))

	// Authorization: derive the learner's group SERVER-SIDE and require manager+.
	groupID, ok := HasSupervisionAccess(tc.db, userID, isAdmin, sessionID)
	if !ok {
		ctx.JSON(http.StatusForbidden, &errors.APIError{ErrorCode: http.StatusForbidden, ErrorMessage: "You are not authorized to supervise this session"})
		return nil, false
	}

	// Plan gate (ANDed with authz): a valid manager on a plan without the feature
	// is still denied.
	if !PlanAllowsSupervision(tc.resolveSupervisionPlan(userID)) {
		ctx.JSON(http.StatusForbidden, &errors.APIError{ErrorCode: http.StatusForbidden, ErrorMessage: "Your plan does not include terminal supervision"})
		return nil, false
	}

	auditSvc := auditServices.NewAuditService(tc.db)
	if _, err := StartSupervision(tc.db, auditSvc, userID, isAdmin, sessionID); err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{ErrorCode: http.StatusInternalServerError, ErrorMessage: "Failed to start supervision"})
		return nil, false
	}

	// Resolve the terminal and the OWNER's key (tt-backend authorizes by the
	// session owner's key; the supervisor rides it — ocf-core is the authorizer).
	terminal, err := tc.service.GetSessionInfo(sessionID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{ErrorCode: http.StatusNotFound, ErrorMessage: "Session not found"})
		return nil, false
	}
	ownerKey, keyErr := tc.service.GetUserKey(terminal.UserID)
	if keyErr != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{ErrorCode: http.StatusInternalServerError, ErrorMessage: "Terminal owner's API key not found"})
		return nil, false
	}

	instanceType := terminal.InstanceType
	if instanceType == "" {
		instanceType = tc.terminalType
	}
	wsURL, buildErr := BuildSuperviseWSURL(tc.terminalTrainerURL, tc.apiVersion, instanceType, terminal.SessionID)
	if buildErr != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{ErrorCode: http.StatusInternalServerError, ErrorMessage: "Invalid terminal trainer URL"})
		return nil, false
	}

	return &superviseTarget{
		auditSvc:    auditSvc,
		userID:      userID,
		isAdmin:     isAdmin,
		groupID:     groupID,
		sessionID:   sessionID,
		ttSessionID: terminal.SessionID,
		ownerAPIKey: ownerKey.APIKey,
		wsURL:       wsURL,
	}, true
}

// supervisionDialer connects to tt-backend with a bounded handshake so a wedged
// backend cannot leave the supervise dial hanging indefinitely.
var supervisionDialer = &websocket.Dialer{HandshakeTimeout: 10 * time.Second}

// dialSupervisionUpstream opens the observer WebSocket to tt-backend, riding the
// session owner's API key.
func dialSupervisionUpstream(wsURL, ownerAPIKey string) (*websocket.Conn, error) {
	headers := make(http.Header)
	headers.Set("X-API-Key", ownerAPIKey)
	conn, _, err := supervisionDialer.Dial(wsURL, headers)
	return conn, err
}

// superviseBroker brokers a single supervision session between the trainer's
// browser (clientConn) and the learner's tt-backend console (upstream): it forwards
// output, learns its own attachment id, brokers take/release-hand, and re-authorizes
// periodically. teardown closes BOTH connections exactly once so either side ending
// unblocks the other (no half-open leak); stop closes done to halt the tickers.
type superviseBroker struct {
	tc         *terminalController
	auditSvc   auditServices.AuditService
	clientConn *websocket.Conn
	upstream   *websocket.Conn

	userID      string
	isAdmin     bool
	sessionID   string
	ttSessionID string
	ownerAPIKey string
	groupID     string

	teardown func()
	stop     func()
	done     chan struct{}

	mu           sync.Mutex
	attachmentID string
	promoted     bool
}

func newSuperviseBroker(tc *terminalController, t *superviseTarget, clientConn, upstream *websocket.Conn) *superviseBroker {
	return &superviseBroker{
		tc:          tc,
		auditSvc:    t.auditSvc,
		clientConn:  clientConn,
		upstream:    upstream,
		userID:      t.userID,
		isAdmin:     t.isAdmin,
		sessionID:   t.sessionID,
		ttSessionID: t.ttSessionID,
		ownerAPIKey: t.ownerAPIKey,
		groupID:     t.groupID,
	}
}

// run wires the once-only teardown/stop discipline, launches the background loops,
// then drives the client-frame broker synchronously until either side ends. On exit
// it tears the stream down and bounds the supervision window in the audit trail
// (emitting `released` first when the hand was still held at disconnect).
func (b *superviseBroker) run() {
	var closeOnce sync.Once
	b.teardown = func() { closeOnce.Do(func() { b.clientConn.Close(); b.upstream.Close() }) }
	defer b.teardown()
	b.done = make(chan struct{})
	var doneOnce sync.Once
	b.stop = func() { doneOnce.Do(func() { close(b.done) }) }
	defer b.stop()

	go b.keepaliveLoop()
	go b.reauthLoop()
	go b.pumpUpstream()

	b.brokerClientFrames()

	b.teardown()
	_ = EndSupervision(b.tc.db, b.auditSvc, b.userID, b.isAdmin, b.sessionID, b.groupID, b.handHeld())
}

// keepaliveLoop pings the browser on the ConnectConsole / tt-backend cadence until
// the stream is stopped.
func (b *superviseBroker) keepaliveLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := b.clientConn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
				return
			}
		case <-b.done:
			return
		}
	}
}

// reauthLoop is the M1 periodic re-authorization: on loss of access it demotes the
// attachment (if the hand is held) and tears the stream down.
func (b *superviseBroker) reauthLoop() {
	ticker := time.NewTicker(supervisionReauthInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if SupervisionStillAuthorized(b.tc.db, b.userID, b.isAdmin, b.sessionID) {
				continue
			}
			b.mu.Lock()
			wasPromoted, aid := b.promoted, b.attachmentID
			b.mu.Unlock()
			if wasPromoted && aid != "" {
				if perr := b.tc.patchAttachmentRole(b.ttSessionID, aid, "observer", b.ownerAPIKey); perr != nil {
					slog.Error("supervision demote-on-deauth PATCH failed", "session_id", b.sessionID, "err", perr)
				}
			}
			slog.Warn("supervision re-authorization failed; tearing down", "session_id", b.sessionID, "user_id", b.userID)
			b.teardown()
			return
		case <-b.done:
			return
		}
	}
}

// pumpUpstream forwards every tt-backend frame to the browser and, on binary frames,
// learns our OWN attachment id from tt-backend's explicit self-snapshot control
// frame ({"type":"attachment","event":"self",...}), sent first to every control
// attachment. Binding only on that frame (never a "joined" broadcast for another
// attachment) is what keeps take/release-hand addressing the supervisor's attachment
// and never the learner's console.
func (b *superviseBroker) pumpUpstream() {
	defer b.teardown()
	for {
		mt, data, rerr := b.upstream.ReadMessage()
		if rerr != nil {
			return
		}
		if mt == websocket.BinaryMessage {
			b.mu.Lock()
			b.attachmentID = bindSelfAttachmentID(b.attachmentID, data)
			b.mu.Unlock()
		}
		if werr := b.clientConn.WriteMessage(mt, data); werr != nil {
			return
		}
	}
}

// brokerClientFrames drives the trainer→upstream frame-aware allow-list. Binary
// client frames are NEVER forwarded upstream; recognized supervision control
// envelopes are brokered here; anything else is forwarded as terminal input (applied
// by tt-backend only once this attachment is interactive). It returns when either
// side ends, leaving run() to tear down and close the audit window.
func (b *superviseBroker) brokerClientFrames() {
	for {
		mt, data, rerr := b.clientConn.ReadMessage()
		if rerr != nil {
			return
		}
		if mt == websocket.BinaryMessage {
			continue // never forward raw client control bytes upstream
		}
		var msg supervisionControlMsg
		if json.Unmarshal(data, &msg) == nil && (msg.Type == "take_hand" || msg.Type == "release_hand") {
			b.handleControl(msg.Type)
			continue
		}
		if werr := b.upstream.WriteMessage(websocket.TextMessage, data); werr != nil {
			return
		}
	}
}

// handleControl brokers one take_hand / release_hand frame against our bound
// attachment. A frame arriving before we know our attachment id is dropped (logged),
// since the PATCH cannot be addressed yet.
func (b *superviseBroker) handleControl(msgType string) {
	b.mu.Lock()
	aid := b.attachmentID
	b.mu.Unlock()
	if aid == "" {
		// Our attachment id is not known yet; cannot address the PATCH. This should
		// not happen once tt-backend has sent the self frame, so log it to make a
		// mis-ordered deploy (self frame missing) debuggable.
		slog.Warn("supervision control frame dropped: attachment id not yet bound (is tt-backend sending the self frame?)",
			"session_id", b.sessionID, "type", msgType)
		return
	}
	switch msgType {
	case "take_hand":
		// Re-resolve the plan on EVERY take_hand rather than reusing the one from WS
		// open: a plan revoked mid-session must deny a later escalation even though
		// the stream opened under a valid plan. TakeHandForSupervision re-authorizes,
		// re-checks the plan, and audits-before-act; any failure denies the promotion
		// (fail-closed, no escalation).
		if err := TakeHandForSupervision(b.tc.db, b.auditSvc, b.tc.resolveSupervisionPlan(b.userID), b.userID, b.isAdmin, b.sessionID, b.groupID); err != nil {
			return
		}
		if perr := b.tc.patchAttachmentRole(b.ttSessionID, aid, "interactive", b.ownerAPIKey); perr != nil {
			// Record the failed act distinctly; do NOT escalate.
			slog.Error("supervision take-hand PATCH failed", "session_id", b.sessionID, "err", perr)
			_ = b.auditSvc.Log(buildSupervisionAuditStatus(auditModels.AuditEventSupervisionTakeHand, b.userID, b.sessionID, b.groupID, "failed"))
			return
		}
		b.mu.Lock()
		b.promoted = true
		b.mu.Unlock()
	case "release_hand":
		if perr := b.tc.patchAttachmentRole(b.ttSessionID, aid, "observer", b.ownerAPIKey); perr != nil {
			slog.Error("supervision release-hand PATCH failed", "session_id", b.sessionID, "err", perr)
			return
		}
		b.mu.Lock()
		b.promoted = false
		b.mu.Unlock()
		_ = b.auditSvc.Log(buildSupervisionAudit(auditModels.AuditEventSupervisionReleased, b.userID, b.sessionID, b.groupID))
	}
}

// handHeld reports whether the trainer currently holds the interactive hand.
func (b *superviseBroker) handHeld() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.promoted
}

// supervisionHTTPClient bounds tt-backend REST calls (the attachment role PATCH) so
// a slow backend cannot block the broker's control path indefinitely.
var supervisionHTTPClient = &http.Client{Timeout: 30 * time.Second}

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

	resp, err := supervisionHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("attachment role PATCH failed: %d", resp.StatusCode)
	}
	return nil
}
