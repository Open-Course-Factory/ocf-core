package scenarios_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"soli/formations/src/scenarios/models"
	scenarioController "soli/formations/src/scenarios/routes"
	"soli/formations/src/scenarios/services"
	terminalModels "soli/formations/src/terminalTrainer/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// The agent is the learner's tab: it runs what the engine asks in the machine
// it booted and answers. Everything it answers is untrusted input.

type agentRequest struct {
	ID      int               `json:"id"`
	Op      string            `json:"op"`
	Command []string          `json:"command"`
	Env     map[string]string `json:"env"`
	Data    string            `json:"data"`
}

type agentFixture struct {
	db       *gorm.DB
	learner  string
	run      models.ScenarioSession
	terminal string
	agents   *services.LocalAgents
	server   *httptest.Server
}

func newAgentFixture(t *testing.T) agentFixture {
	t.Helper()
	db := freshTestDB(t)
	learner := "agent-learner-" + uuid.NewString()
	terminal := terminalModels.LocalSessionIDPrefix + uuid.NewString()
	scenario := models.Scenario{Name: "agent-" + uuid.NewString(), Title: "Agent", CreatedByID: "author", Machine: models.ScenarioMachineLocal}
	require.NoError(t, db.Create(&scenario).Error)
	run := models.ScenarioSession{ScenarioID: scenario.ID, UserID: learner, TerminalSessionID: &terminal,
		Machine: models.ScenarioMachineLocal, Status: "active", StartedAt: time.Now()}
	require.NoError(t, db.Create(&run).Error)

	agents := services.NewLocalAgents()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", c.GetHeader("X-Test-User"))
		c.Set("userRoles", strings.Split(c.GetHeader("X-Test-Roles"), ","))
		c.Next()
	})
	router.GET("/api/v1/scenario-sessions/:id/agent", scenarioController.NewLocalAgentController(db, agents).ConnectAgent)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	return agentFixture{db: db, learner: learner, run: run, terminal: terminal, agents: agents, server: server}
}

// dial opens the agent connection of runID as userID; the response carries the
// refusal when there is one.
func (f agentFixture) dial(t *testing.T, runID uuid.UUID, userID string, roles string) (*websocket.Conn, *http.Response) {
	t.Helper()
	url := "ws" + strings.TrimPrefix(f.server.URL, "http") + "/api/v1/scenario-sessions/" + runID.String() + "/agent"
	conn, resp, _ := websocket.DefaultDialer.Dial(url, http.Header{"X-Test-User": {userID}, "X-Test-Roles": {roles}})
	if conn != nil {
		t.Cleanup(func() { _ = conn.Close() })
	}
	return conn, resp
}

// serveAgent answers every request on conn with answer, until the connection ends.
func serveAgent(conn *websocket.Conn, answer func(agentRequest) any) {
	go func() {
		for {
			var req agentRequest
			if err := conn.ReadJSON(&req); err != nil {
				return
			}
			if reply := answer(req); reply != nil {
				_ = conn.WriteJSON(reply)
			}
		}
	}()
}

func (f agentFixture) machines() *services.LocalMachineService {
	return services.NewLocalMachineService(f.agents, 200*time.Millisecond, time.Second)
}

func TestLocalAgent_RunsWhatTheEngineAsks(t *testing.T) {
	f := newAgentFixture(t)
	conn, resp := f.dial(t, f.run.ID, f.learner, "member")
	require.NotNil(t, conn, "status %v", resp.StatusCode)
	serveAgent(conn, func(req agentRequest) any {
		return map[string]any{"id": req.ID, "exit_code": 3, "stdout": strings.Join(req.Command, " "), "stderr": req.Env["K"]}
	})

	code, stdout, stderr, err := f.machines().ExecInContainer(f.terminal, []string{"echo", "hi"}, map[string]string{"K": "v"}, 5)
	require.NoError(t, err)
	assert.Equal(t, 3, code)
	assert.Equal(t, "echo hi", stdout)
	assert.Equal(t, "v", stderr)
}

func TestLocalAgent_VerifyPassesOnlyOnExitZero(t *testing.T) {
	f := newAgentFixture(t)
	conn, _ := f.dial(t, f.run.ID, f.learner, "member")
	exit := 1
	serveAgent(conn, func(req agentRequest) any {
		return map[string]any{"id": req.ID, "exit_code": exit, "stdout": "checked"}
	})
	step := &models.ScenarioStep{Order: 1, VerifyScript: "test -f /tmp/done"}

	passed, output, err := f.machines().VerifyStep(f.terminal, step)
	require.NoError(t, err)
	assert.False(t, passed)
	assert.Equal(t, "checked", output)

	exit = 0
	passed, _, err = f.machines().VerifyStep(f.terminal, step)
	require.NoError(t, err)
	assert.True(t, passed)
}

// A missing or dead agent is a clear error, never a pass.
func TestLocalAgent_NoAgentIsAnErrorNeverAPass(t *testing.T) {
	f := newAgentFixture(t)
	start := time.Now()
	passed, _, err := f.machines().VerifyStep(f.terminal, &models.ScenarioStep{Order: 1, VerifyScript: "true"})
	require.ErrorIs(t, err, services.ErrNoMachineAgent)
	assert.False(t, passed)
	assert.Less(t, time.Since(start), 2*time.Second, "waits for a booting machine, not forever")
}

func TestLocalAgent_GoneAgentFailsAtOnce(t *testing.T) {
	f := newAgentFixture(t)
	conn, _ := f.dial(t, f.run.ID, f.learner, "member")
	require.Eventually(t, func() bool { return f.agents.Connected(f.terminal) }, time.Second, 10*time.Millisecond)
	require.NoError(t, conn.Close())
	require.Eventually(t, func() bool { return !f.agents.Connected(f.terminal) }, time.Second, 10*time.Millisecond)

	slow := services.NewLocalMachineService(f.agents, time.Minute, time.Second)
	start := time.Now()
	_, _, _, err := slow.ExecInContainer(f.terminal, []string{"true"}, nil, 5)
	require.ErrorIs(t, err, services.ErrNoMachineAgent)
	assert.Less(t, time.Since(start), time.Second, "an agent that was there and left is not waited for")
}

func TestLocalAgent_UnansweredRequestTimesOut(t *testing.T) {
	f := newAgentFixture(t)
	conn, _ := f.dial(t, f.run.ID, f.learner, "member")
	serveAgent(conn, func(agentRequest) any { return nil })

	_, _, _, err := f.machines().ExecInContainer(f.terminal, []string{"sleep", "100"}, nil, 0)
	require.Error(t, err)
}

func TestLocalAgent_ConsoleWithoutTerminalIsNoLiveConsole(t *testing.T) {
	f := newAgentFixture(t)
	conn, _ := f.dial(t, f.run.ID, f.learner, "member")
	var typed string
	serveAgent(conn, func(req agentRequest) any {
		if req.Op != "console" {
			return map[string]any{"id": req.ID, "error": "unexpected"}
		}
		typed = req.Data
		return map[string]any{"id": req.ID, "error": "no_console"}
	})

	err := f.machines().WriteToConsole(f.terminal, "cd /srv")
	require.ErrorIs(t, err, services.ErrNoLiveConsole)
	assert.Equal(t, "cd /srv\n", typed, "typed as a line, as tt-backend's append_newline")
}

// Untrusted output is bounded: a tab cannot make the server hold megabytes.
func TestLocalAgent_OutputIsCapped(t *testing.T) {
	f := newAgentFixture(t)
	conn, _ := f.dial(t, f.run.ID, f.learner, "member")
	serveAgent(conn, func(req agentRequest) any {
		return map[string]any{"id": req.ID, "exit_code": 0, "stdout": strings.Repeat("x", 200_000)}
	})
	_, stdout, _, err := f.machines().ExecInContainer(f.terminal, []string{"yes"}, nil, 5)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(stdout), services.MaxAgentOutput)

	conn2, _ := f.dial(t, f.run.ID, f.learner, "member")
	assert.Nil(t, conn2, "still one agent per run")
}

func TestLocalAgent_PushFileGoesThroughExec(t *testing.T) {
	f := newAgentFixture(t)
	conn, _ := f.dial(t, f.run.ID, f.learner, "member")
	var got []string
	serveAgent(conn, func(req agentRequest) any {
		got = req.Command
		return map[string]any{"id": req.ID, "exit_code": 0}
	})
	require.NoError(t, f.machines().PushFile(f.terminal, "/root/.ocf_verify.sh", "#!/bin/sh\ntrue\n", "0700"))
	require.GreaterOrEqual(t, len(got), 3)
	assert.Equal(t, "sh", got[0])
	assert.Contains(t, got, "/root/.ocf_verify.sh")
	assert.Contains(t, got, "0700")
	assert.NotContains(t, strings.Join(got, " "), "true\n", "the content travels encoded, never as shell text")
}

func TestLocalAgentRoute_OwnerOfALocalRunOnly(t *testing.T) {
	f := newAgentFixture(t)

	_, resp := f.dial(t, f.run.ID, "someone-else", "member")
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)

	_, resp = f.dial(t, f.run.ID, "an-admin", "administrator")
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode, "no administrator bypass: the agent is the learner's machine")

	incusTerminal := "incus-" + uuid.NewString()
	incusRun := models.ScenarioSession{ScenarioID: f.run.ScenarioID, UserID: f.learner, TerminalSessionID: &incusTerminal,
		Machine: models.ScenarioMachineIncus, Status: "completed", StartedAt: time.Now()}
	require.NoError(t, f.db.Create(&incusRun).Error)
	_, resp = f.dial(t, incusRun.ID, f.learner, "member")
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusConflict, resp.StatusCode, "a container's run has no agent")

	_, resp = f.dial(t, uuid.New(), f.learner, "member")
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// The engine talks to one machine per run: a dispatcher sends a local
// terminal to the agent and every other one to tt-backend.
func TestRunMachines_DispatchesByTerminal(t *testing.T) {
	f := newAgentFixture(t)
	conn, _ := f.dial(t, f.run.ID, f.learner, "member")
	serveAgent(conn, func(req agentRequest) any { return map[string]any{"id": req.ID, "exit_code": 0, "stdout": "from-agent"} })

	tt := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"exit_code": 0, "stdout": "from-tt"})
	}))
	t.Cleanup(tt.Close)
	machines := services.NewRunMachines(services.NewVerificationServiceWithConfig(tt.URL, "k"), f.machines())

	_, out, _, err := machines.ExecInContainer(f.terminal, []string{"x"}, nil, 5)
	require.NoError(t, err)
	assert.Equal(t, "from-agent", out)
	_, out, _, err = machines.ExecInContainer("incus-sess", []string{"x"}, nil, 5)
	require.NoError(t, err)
	assert.Equal(t, "from-tt", out)
}
