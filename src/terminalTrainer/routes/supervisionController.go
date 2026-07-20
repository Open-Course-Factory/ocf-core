package terminalController

// supervisionController.go — HTTP + WebSocket handlers for terminal supervision
// (issue #425). The security-critical decisions live in supervision.go; this file
// is the transport: a group-scoped session listing, and a frame-aware WebSocket
// broker that observes a learner's tt-backend console and brokers take-hand.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	auditModels "soli/formations/src/audit/models"
	auditServices "soli/formations/src/audit/services"
	"soli/formations/src/auth/errors"
	paymentServices "soli/formations/src/payment/services"
)

// isAdminFromRoles reports whether the roles slice carries the platform admin role.
func isAdminFromRoles(roles []string) bool {
	for _, r := range roles {
		if r == "administrator" {
			return true
		}
	}
	return false
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

// SuperviseSession godoc
//
//	@Summary		Supervise a learner's terminal (WebSocket, observer + take-hand)
//	@Description	Opens a read-only observer stream onto a learner's terminal for a group manager+, and brokers take-hand/release-hand via the trainer's in-band control frames. The learner's group is derived server-side from the session record (never client-supplied); requires a plan with session supervision.
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

	// Plan gate: the caller's effective plan must include session supervision.
	// ANDed with the authz decision — a valid manager on a plan without the
	// feature is still denied.
	planResult, planErr := paymentServices.NewEffectivePlanService(tc.db).GetUserEffectivePlan(userID, nil)
	if planErr != nil || planResult == nil || !PlanAllowsSupervision(planResult.Plan) {
		ctx.JSON(http.StatusForbidden, &errors.APIError{ErrorCode: http.StatusForbidden, ErrorMessage: "Your plan does not include terminal supervision"})
		return
	}

	// Audit the start (after authorizing). A failed audit write aborts before upgrade.
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

	// Build the tt-backend console URL as an OBSERVER with control frames on.
	wsURL, buildErr := tc.buildSuperviseWSURL(terminal.InstanceType, terminal.SessionID)
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

	// Keepalive pings to the browser (matches ConnectConsole / tt-backend cadence).
	pingTicker := time.NewTicker(30 * time.Second)
	defer pingTicker.Stop()
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			select {
			case <-pingTicker.C:
				if err := clientConn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
					return
				}
			case <-done:
				return
			}
		}
	}()

	// Our tt-backend attachment id, learned from the first control frame carrying
	// one (the self-snapshot "joined" delivered on control-connect). Needed to
	// address our attachment in the take-hand PATCH.
	var attMu sync.Mutex
	var attachmentID string

	// Upstream (tt-backend) → client: forward everything, and sniff control frames
	// (binary JSON) to capture our attachment id.
	go func() {
		for {
			mt, data, rerr := upstream.ReadMessage()
			if rerr != nil {
				break
			}
			if mt == websocket.BinaryMessage {
				var ev struct {
					AttachmentID string `json:"attachment_id"`
				}
				if json.Unmarshal(data, &ev) == nil && ev.AttachmentID != "" {
					attMu.Lock()
					if attachmentID == "" {
						attachmentID = ev.AttachmentID
					}
					attMu.Unlock()
				}
			}
			if werr := clientConn.WriteMessage(mt, data); werr != nil {
				break
			}
		}
	}()

	// Client (trainer) → upstream: frame-aware allow-list. Client control bytes
	// (binary frames) are NEVER forwarded upstream; recognized supervision control
	// envelopes are brokered here; anything else is forwarded as terminal input
	// (which tt-backend applies only once this attachment is interactive).
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
			attMu.Lock()
			aid := attachmentID
			attMu.Unlock()
			tc.brokerSupervisionControl(auditSvc, userID, isAdmin, sessionID, groupID, terminal.SessionID, aid, ownerKey.APIKey, msg.Type)
			continue
		}
		if werr := upstream.WriteMessage(websocket.TextMessage, data); werr != nil {
			break
		}
	}
}

// brokerSupervisionControl handles an in-band take_hand / release_hand request by
// re-authorizing + auditing (fail-closed) and then driving tt-backend's REST role
// PATCH. It never forwards the raw control message upstream.
func (tc *terminalController) brokerSupervisionControl(audit auditServices.AuditService, userID string, isAdmin bool, sessionID, groupID, ttSessionID, attachmentID, apiKey, ctlType string) {
	if attachmentID == "" {
		return // our attachment id is not known yet; cannot address the PATCH
	}
	switch ctlType {
	case "take_hand":
		// Audit-before-act, fail-closed: a failed audit write blocks the promotion.
		if err := TakeHandForSupervision(tc.db, audit, userID, isAdmin, sessionID, groupID); err != nil {
			return
		}
		_ = tc.patchAttachmentRole(ttSessionID, attachmentID, "interactive", apiKey)
	case "release_hand":
		if err := tc.patchAttachmentRole(ttSessionID, attachmentID, "observer", apiKey); err == nil {
			_ = audit.Log(buildSupervisionAudit(auditModels.AuditEventSupervisionReleased, userID, sessionID, groupID))
		}
	}
}

// buildSuperviseWSURL builds the tt-backend observer console WebSocket URL.
func (tc *terminalController) buildSuperviseWSURL(instanceType, ttSessionID string) (string, error) {
	u, err := url.Parse(tc.terminalTrainerURL)
	if err != nil {
		return "", err
	}
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	path := fmt.Sprintf("/%s", tc.apiVersion)
	if instanceType != "" {
		path += fmt.Sprintf("/%s", instanceType)
	} else if tc.terminalType != "" {
		path += fmt.Sprintf("/%s", tc.terminalType)
	}
	path += "/console"
	u.Path = path

	q := u.Query()
	q.Set("id", ttSessionID)
	q.Set("role", "observer")
	q.Set("control", "1")
	u.RawQuery = q.Encode()
	return u.String(), nil
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
