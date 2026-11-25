package scenarioController

import (
	"net/http"
	"slices"

	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/errors"
	config "soli/formations/src/configuration"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
	terminalModels "soli/formations/src/terminalTrainer/models"
	"soli/formations/src/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"gorm.io/gorm"
)

// LocalAgentController serves the agents of machines in learners' browsers:
// the tab that booted a run's machine connects here, and the engine runs the
// run's scripts through it.
type LocalAgentController struct {
	db     *gorm.DB
	agents *services.LocalAgents
}

func NewLocalAgentController(db *gorm.DB, agents *services.LocalAgents) *LocalAgentController {
	return &LocalAgentController{db: db, agents: agents}
}

// Browsers cannot set headers on a WebSocket: the token comes in the
// subprotocol (casdoor.MoveWebSocketTokenToHeader), and the origin must be
// one of the platform's fronts.
var agentUpgrader = websocket.Upgrader{
	Subprotocols: []string{casdoor.WebSocketAuthProtocol},
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin != "" && !config.IsOriginAllowed(origin) {
			utils.Warn("agent websocket upgrade refused: origin %q not in the frontend allowlist", origin)
			return false
		}
		return true
	},
}

// ConnectAgent godoc
//
//	@Summary		Connect the agent of a local machine
//	@Description	WebSocket for the tab running a scenario run's machine in the learner's browser. The run's owner only, for an open run on a local machine, one tab at a time.
//	@Tags			scenario-sessions
//	@Param			id	path	string	true	"Scenario session ID"
//	@Success		101
//	@Failure		403	{object}	errors.APIError
//	@Failure		404	{object}	errors.APIError
//	@Failure		409	{object}	errors.APIError
//	@Router			/scenario-sessions/{id}/agent [get]
//	@Security		BearerAuth
func (c *LocalAgentController) ConnectAgent(ctx *gin.Context) {
	runID, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		errors.Respond(ctx, http.StatusBadRequest, "Invalid session ID")
		return
	}
	var run models.ScenarioSession
	if err := c.db.First(&run, "id = ?", runID).Error; err != nil {
		errors.Respond(ctx, http.StatusNotFound, "Session not found")
		return
	}
	// The learner's own machine, and theirs alone: no administrator bypass.
	// Under impersonation userId is the impersonated learner, who owns it.
	if run.UserID != ctx.GetString("userId") {
		errors.Respond(ctx, http.StatusForbidden, "Not your session")
		return
	}
	if run.Machine != models.ScenarioMachineLocal || run.TerminalSessionID == nil ||
		!terminalModels.IsLocalSessionID(*run.TerminalSessionID) ||
		!slices.Contains(models.OpenSessionStatuses, run.Status) {
		errors.Respond(ctx, http.StatusConflict, "This run has no machine in the browser")
		return
	}
	terminalID := *run.TerminalSessionID
	if c.agents.Connected(terminalID) {
		errors.Respond(ctx, http.StatusConflict, services.ErrAgentAlreadyConnected.Error())
		return
	}

	conn, err := agentUpgrader.Upgrade(ctx.Writer, ctx.Request, nil)
	if err != nil {
		return // the upgrader has answered
	}
	agent, err := c.agents.Attach(terminalID, conn)
	if err != nil {
		// Lost a race with another tab between the check and the upgrade.
		_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(4409, err.Error()))
		_ = conn.Close()
		return
	}
	agent.Serve()
}
