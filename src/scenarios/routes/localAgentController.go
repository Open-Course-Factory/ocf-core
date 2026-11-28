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
	terminalServices "soli/formations/src/terminalTrainer/services"
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
	// end ends a terminal the way the platform does.
	end func(terminalID string) error
}

func NewLocalAgentController(db *gorm.DB, agents *services.LocalAgents, end func(terminalID string) error) *LocalAgentController {
	return &LocalAgentController{db: db, agents: agents, end: end}
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
//	@Param			id		path	string	true	"Scenario session ID"
//	@Param			boot	query	string	true	"ID of the machine the tab booted"
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
	boot := ctx.Query("boot")
	if boot == "" || len(boot) > maxBootID {
		errors.Respond(ctx, http.StatusBadRequest, "Missing or invalid machine boot ID")
		return
	}
	terminalID := *run.TerminalSessionID

	// A browser cannot read why an upgrade was refused, so from here every
	// refusal the tab acts on is a close code on an upgraded connection.
	code, reason := c.refusal(terminalID, boot)
	if code == services.AgentCloseMachineLost {
		code = c.replaceLostMachine(terminalID)
	}
	conn, err := agentUpgrader.Upgrade(ctx.Writer, ctx.Request, nil)
	if err != nil {
		return // the upgrader has answered
	}
	if code != 0 {
		services.CloseConn(conn, code, reason)
		return
	}
	agent, err := c.agents.Attach(terminalID, boot, conn)
	if err != nil {
		// Lost a race with another tab between the check and the upgrade.
		services.CloseConn(conn, services.AgentCloseOtherTab, err.Error())
		return
	}
	c.recordBoot(terminalID, boot)
	agent.Serve()
}

// maxBootID bounds the boot ID a tab gives (Terminal.LocalBoot); the tab
// sends a UUID.
const maxBootID = 128

// recordBoot keeps on terminalID's row the machine its agent came from, so a
// restart does not forget it (LocalAgents.RecallBoot).
func (c *LocalAgentController) recordBoot(terminalID, boot string) {
	if err := c.db.Model(&terminalModels.Terminal{}).Where("session_id = ? AND local_boot <> ?", terminalID, boot).
		Update("local_boot", boot).Error; err != nil {
		utils.Error("recording the machine of %s: %v", terminalID, err)
	}
}

// replaceLostMachine deals with a tab that booted another machine than the
// terminal's: a reload, so the terminal's machine is lost. While its world is
// built, the new machine takes its place and the build starts over on it
// (0: admitted). Otherwise the terminal ends, so the run is rebuilt at its
// step (plan rule 10), and the tab is told.
func (c *LocalAgentController) replaceLostMachine(terminalID string) int {
	if c.agents.Building(terminalID) {
		c.agents.ReplaceMachine(terminalID)
		return 0
	}
	if err := c.end(terminalID); err != nil {
		utils.Error("ending the lost machine of %s: %v", terminalID, err)
	}
	return services.AgentCloseMachineLost
}

// refusal says why the machine boot may not serve terminalID, as a close
// code, 0 when it may. A machine other than the terminal's own means the
// tab was reloaded and booted an empty one: the terminal ends, so the run is
// rebuilt at its step (plan rule 10).
func (c *LocalAgentController) refusal(terminalID, boot string) (int, string) {
	var terminal terminalModels.Terminal
	if err := c.db.Where("session_id = ?", terminalID).First(&terminal).Error; err != nil || !terminal.IsLive() {
		return services.AgentCloseEnded, "this run's machine has ended"
	}
	c.agents.RecallBoot(terminalID, terminal.LocalBoot)
	switch err := c.agents.Admits(terminalID, boot); err {
	case nil:
		return 0, ""
	case services.ErrMachineLost:
		return services.AgentCloseMachineLost, err.Error()
	default:
		return services.AgentCloseOtherTab, err.Error()
	}
}

// MirrorAccess godoc
//
//	@Summary		May a machine in a browser use the Debian mirror
//	@Description	Asked by the front's nginx before it serves the Debian mirror to a machine in a learner's browser (auth_request): 204 while the machine's run is open, its tab connected and the key the one that tab announced, with the platform switch on; 403 otherwise.
//	@Tags			scenario-sessions
//	@Param			X-Local-Machine	header	string	true	"Terminal session ID of the machine"
//	@Param			X-Local-Machine-Key	header	string	true	"The key the machine's tab announced over its agent"
//	@Success		204
//	@Failure		403
//	@Router			/local-machines/mirror-access [get]
//
// The terminal id is no secret, so the machine also gives the key its tab
// chose at boot and told core over the agent: only that tab knows it, and a
// reload replaces it.
//
// The machine comes in a header, not the path: nginx then asks a fixed URL,
// which needs no resolver for core's host name.
func (c *LocalAgentController) MirrorAccess(ctx *gin.Context) {
	terminalID := ctx.GetHeader("X-Local-Machine")
	key := ctx.GetHeader("X-Local-Machine-Key")
	var terminal terminalModels.Terminal
	allowed := terminalModels.IsLocalSessionID(terminalID) &&
		terminalServices.LocalMachinesEnabled(c.db) &&
		c.db.Where("session_id = ?", terminalID).First(&terminal).Error == nil &&
		terminal.IsLive() &&
		c.agents.MirrorKeyMatches(terminalID, key)
	if !allowed {
		ctx.Status(http.StatusForbidden)
		return
	}
	ctx.Status(http.StatusNoContent)
}
