package scenarios_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	configModels "soli/formations/src/configuration/models"
	"soli/formations/src/scenarios/models"
	scenarioController "soli/formations/src/scenarios/routes"
	"soli/formations/src/scenarios/services"
	terminalModels "soli/formations/src/terminalTrainer/models"
	terminalServices "soli/formations/src/terminalTrainer/services"

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
	key := terminalModels.UserTerminalKey{UserID: learner, APIKey: "key-" + terminal, KeyName: "k-" + terminal, IsActive: true}
	require.NoError(t, db.Create(&key).Error)
	require.NoError(t, db.Create(&terminalModels.Terminal{SessionID: terminal, UserID: learner, State: terminalModels.StateRunning,
		ExpiresAt: time.Now().Add(time.Hour), InstanceType: "debian", UserTerminalKeyID: key.ID, Backend: terminalModels.LocalBackend}).Error)

	agents := services.NewLocalAgents()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", c.GetHeader("X-Test-User"))
		c.Set("userRoles", strings.Split(c.GetHeader("X-Test-Roles"), ","))
		c.Next()
	})
	router.GET("/api/v1/scenario-sessions/:id/agent", scenarioController.NewLocalAgentController(db, agents, endTerminal(db)).ConnectAgent)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	return agentFixture{db: db, learner: learner, run: run, terminal: terminal, agents: agents, server: server}
}

// endTerminal ends a terminal the way the platform does.
func endTerminal(db *gorm.DB) func(string) error {
	return terminalServices.NewTerminalTrainerService(db).DeleteSession
}

// dial opens the agent connection of runID as userID, from the tab's first
// machine; the response carries the refusal when there is one.
func (f agentFixture) dial(t *testing.T, runID uuid.UUID, userID string, roles string) (*websocket.Conn, *http.Response) {
	t.Helper()
	return f.dialBoot(t, runID, userID, roles, "boot-1")
}

// dialBoot is dial from the machine the tab booted as boot.
func (f agentFixture) dialBoot(t *testing.T, runID uuid.UUID, userID string, roles string, boot string) (*websocket.Conn, *http.Response) {
	t.Helper()
	url := "ws" + strings.TrimPrefix(f.server.URL, "http") + "/api/v1/scenario-sessions/" + runID.String() + "/agent?boot=" + boot
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
	assert.Equal(t, services.AgentCloseOtherTab, closeCode(t, conn2), "still one agent per run")
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

// A foreground script staged while the console was held is typed when the
// learner's shell opens. tt-backend reports that for a container; for a local
// machine the agent does, and the same observer delivers the script.
func TestLocalAgent_ReportsTheLearnersConsoleOpening(t *testing.T) {
	f := newAgentFixture(t)
	attached := make(chan string, 1)
	terminalServices.SetConsoleAttachedObserver(func(terminalID string) { attached <- terminalID })
	t.Cleanup(func() { terminalServices.SetConsoleAttachedObserver(nil) })

	conn, _ := f.dial(t, f.run.ID, f.learner, "member")
	require.NotNil(t, conn)
	require.NoError(t, conn.WriteJSON(map[string]any{"event": "console_attached"}))

	select {
	case got := <-attached:
		assert.Equal(t, f.terminal, got)
	case <-time.After(2 * time.Second):
		t.Fatal("the console opening was not reported")
	}
}

// closeCode waits for the server to close conn and returns its close code.
func closeCode(t *testing.T, conn *websocket.Conn) int {
	t.Helper()
	require.NotNil(t, conn)
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			var closeErr *websocket.CloseError
			if errors.As(err, &closeErr) {
				return closeErr.Code
			}
			t.Fatalf("connection ended without a close code: %v", err)
		}
	}
}

func (f agentFixture) terminalState(t *testing.T) terminalModels.TerminalState {
	t.Helper()
	var row terminalModels.Terminal
	require.NoError(t, f.db.Unscoped().Where("session_id = ?", f.terminal).First(&row).Error)
	return row.State
}

func (f agentFixture) connectAndLeave(t *testing.T, boot string) {
	t.Helper()
	conn, _ := f.dialBoot(t, f.run.ID, f.learner, "member", boot)
	require.NotNil(t, conn)
	require.Eventually(t, func() bool { return f.agents.Connected(f.terminal) }, time.Second, 10*time.Millisecond)
	require.NoError(t, conn.Close())
	require.Eventually(t, func() bool { return !f.agents.Connected(f.terminal) }, time.Second, 10*time.Millisecond)
}

// A reload boots a new machine, empty: the run's machine is lost. Its
// terminal ends, so the run is rebuilt at its step, and the tab is told so.
func TestLocalAgent_ANewMachineEndsTheLostOne(t *testing.T) {
	f := newAgentFixture(t)
	f.connectAndLeave(t, "boot-1")

	conn, _ := f.dialBoot(t, f.run.ID, f.learner, "member", "boot-2")
	assert.Equal(t, services.AgentCloseMachineLost, closeCode(t, conn))
	assert.Equal(t, terminalModels.StateDeleted, f.terminalState(t))
	assert.False(t, f.agents.Connected(f.terminal))
}

// The same machine coming back (a network blip, the learner back on the page)
// takes its run up again.
func TestLocalAgent_TheSameMachineReconnects(t *testing.T) {
	f := newAgentFixture(t)
	f.connectAndLeave(t, "boot-1")

	conn, _ := f.dialBoot(t, f.run.ID, f.learner, "member", "boot-1")
	require.NotNil(t, conn)
	require.Eventually(t, func() bool { return f.agents.Connected(f.terminal) }, time.Second, 10*time.Millisecond)
	assert.Equal(t, terminalModels.StateRunning, f.terminalState(t))
}

// A second tab is told the run is open elsewhere, and the first keeps it.
func TestLocalAgent_ASecondTabIsToldTheRunIsOpenElsewhere(t *testing.T) {
	f := newAgentFixture(t)
	first, _ := f.dialBoot(t, f.run.ID, f.learner, "member", "boot-1")
	require.NotNil(t, first)
	require.Eventually(t, func() bool { return f.agents.Connected(f.terminal) }, time.Second, 10*time.Millisecond)

	second, _ := f.dialBoot(t, f.run.ID, f.learner, "member", "boot-2")
	assert.Equal(t, services.AgentCloseOtherTab, closeCode(t, second))
	assert.True(t, f.agents.Connected(f.terminal))
	assert.Equal(t, terminalModels.StateRunning, f.terminalState(t))
}

// A run whose terminal has ended has no machine to serve: the tab is told.
func TestLocalAgent_AnEndedTerminalIsToldSo(t *testing.T) {
	f := newAgentFixture(t)
	require.NoError(t, endTerminal(f.db)(f.terminal))

	conn, _ := f.dialBoot(t, f.run.ID, f.learner, "member", "boot-1")
	assert.Equal(t, services.AgentCloseEnded, closeCode(t, conn))
}

// Rule 9: a tab gone for the grace delay ends its run's terminal, which frees
// the plan's budget; reopening the run rebuilds it.
func TestSweepLocalMachines_EndsAMachineGoneForTheGraceDelay(t *testing.T) {
	f := newAgentFixture(t)
	f.connectAndLeave(t, "boot-1")
	grace := 200 * time.Millisecond

	services.SweepLocalMachines(f.db, f.agents, grace, endTerminal(f.db))
	assert.Equal(t, terminalModels.StateRunning, f.terminalState(t), "within the grace delay")

	time.Sleep(grace)
	services.SweepLocalMachines(f.db, f.agents, grace, endTerminal(f.db))
	assert.Equal(t, terminalModels.StateDeleted, f.terminalState(t))
}

// A tab never opened, or one lost while ocf-core restarted, has its grace
// delay counted from the first sweep that sees it gone.
func TestSweepLocalMachines_CountsAnUnseenTabFromTheFirstSweep(t *testing.T) {
	f := newAgentFixture(t)
	grace := 200 * time.Millisecond

	services.SweepLocalMachines(f.db, f.agents, grace, endTerminal(f.db))
	assert.Equal(t, terminalModels.StateRunning, f.terminalState(t))

	time.Sleep(grace)
	services.SweepLocalMachines(f.db, f.agents, grace, endTerminal(f.db))
	assert.Equal(t, terminalModels.StateDeleted, f.terminalState(t))
}

// A connected tab is never ended for being away.
func TestSweepLocalMachines_KeepsAConnectedMachine(t *testing.T) {
	f := newAgentFixture(t)
	conn, _ := f.dial(t, f.run.ID, f.learner, "member")
	require.NotNil(t, conn)
	require.Eventually(t, func() bool { return f.agents.Connected(f.terminal) }, time.Second, 10*time.Millisecond)

	services.SweepLocalMachines(f.db, f.agents, 0, endTerminal(f.db))
	assert.Equal(t, terminalModels.StateRunning, f.terminalState(t))
	assert.True(t, f.agents.Connected(f.terminal))
}

// A run ended on the server (expiry, a trainer's stop, a block, an erasure)
// reaches the tab: its agent is closed, so the machine stops answering.
func TestSweepLocalMachines_ClosesTheTabOfAnEndedTerminal(t *testing.T) {
	f := newAgentFixture(t)
	conn, _ := f.dial(t, f.run.ID, f.learner, "member")
	require.NotNil(t, conn)
	require.Eventually(t, func() bool { return f.agents.Connected(f.terminal) }, time.Second, 10*time.Millisecond)
	require.NoError(t, endTerminal(f.db)(f.terminal))

	services.SweepLocalMachines(f.db, f.agents, time.Hour, endTerminal(f.db))
	assert.Equal(t, services.AgentCloseEnded, closeCode(t, conn))
}

// An expired local terminal ends, and its tab is told.
func TestSweepLocalMachines_EndsAnExpiredMachine(t *testing.T) {
	f := newAgentFixture(t)
	conn, _ := f.dial(t, f.run.ID, f.learner, "member")
	require.NotNil(t, conn)
	require.Eventually(t, func() bool { return f.agents.Connected(f.terminal) }, time.Second, 10*time.Millisecond)
	require.NoError(t, f.db.Model(&terminalModels.Terminal{}).Where("session_id = ?", f.terminal).
		Update("expires_at", time.Now().Add(-time.Minute)).Error)

	services.SweepLocalMachines(f.db, f.agents, time.Hour, endTerminal(f.db))
	assert.Equal(t, services.AgentCloseEnded, closeCode(t, conn))
	assert.Equal(t, terminalModels.StateDeleted, f.terminalState(t))
}

// A reload while the run is first being built boots an empty machine: the
// build starts over on it, and the run comes up there, instead of failing
// its setup.
func TestLocalBuild_StartsOverOnTheMachineOfAReloadedTab(t *testing.T) {
	f := newAgentFixture(t)
	scenario := models.Scenario{Name: "rebuilt-" + uuid.NewString(), Title: "Rebuilt", CreatedByID: "author",
		Machine: models.ScenarioMachineLocal, SetupScript: "echo setting-up"}
	require.NoError(t, f.db.Create(&scenario).Error)
	require.NoError(t, f.db.Create(&models.ScenarioStep{ScenarioID: scenario.ID, Order: 0, Title: "Do", StepType: "terminal"}).Error)
	require.NoError(t, f.db.Unscoped().Delete(&f.run).Error) // this run binds its own terminal

	machines := services.NewRunMachines(&mockVerificationService{}, services.NewLocalMachineService(f.agents, 5*time.Second, time.Second))
	run, err := services.NewScenarioSessionService(f.db, &mockFlagService{}, machines).StartScenario(f.learner, scenario.ID, f.terminal, "")
	require.NoError(t, err)
	require.NoError(t, f.db.Model(&models.ScenarioSession{}).Where("id = ?", run.ID).Update("machine", models.ScenarioMachineLocal).Error)

	// The first machine is lost while it runs the setup: the tab reloads.
	first, _ := f.dialBoot(t, run.ID, f.learner, "member", "boot-1")
	require.NotNil(t, first)
	var req agentRequest
	require.NoError(t, first.ReadJSON(&req))
	require.NoError(t, first.Close())

	second, _ := f.dialBoot(t, run.ID, f.learner, "member", "boot-2")
	require.NotNil(t, second)
	ran := make(chan string, 16)
	serveAgent(second, func(req agentRequest) any {
		ran <- strings.Join(req.Command, " ")
		return map[string]any{"id": req.ID, "exit_code": 0}
	})

	require.Eventually(t, func() bool {
		var got models.ScenarioSession
		return f.db.First(&got, "id = ?", run.ID).Error == nil && got.Status == "active"
	}, 10*time.Second, 50*time.Millisecond, "the run comes up on the new machine")
	assert.Equal(t, terminalModels.StateRunning, f.terminalState(t), "the terminal is kept")
	assert.NotEmpty(t, ran, "the build ran again on the new machine")
}

// mirrorAccess asks what nginx asks before serving the Debian mirror to a
// machine in a browser.
func (f agentFixture) mirrorAccess(t *testing.T, terminalID string) int {
	t.Helper()
	router := gin.New()
	router.GET("/api/v1/local-machines/mirror-access", scenarioController.NewLocalAgentController(f.db, f.agents, endTerminal(f.db)).MirrorAccess)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/local-machines/mirror-access", nil)
	req.Header.Set("X-Local-Machine", terminalID)
	router.ServeHTTP(w, req)
	return w.Code
}

func switchLocalMachines(t *testing.T, db *gorm.DB, on bool) {
	t.Helper()
	feature := configModels.Feature{Key: terminalServices.LocalMachinesFeatureKey, Name: "x"}
	require.NoError(t, db.Where("key = ?", feature.Key).FirstOrCreate(&feature).Error)
	require.NoError(t, db.Model(&feature).Update("enabled", on).Error)
}

// Plan rule 11: the Debian mirror serves a machine in a browser while its
// run's tab is connected, the switch on; anything else is refused at once.
func TestMirrorAccess_OnlyAConnectedLocalMachineWhileTheSwitchIsOn(t *testing.T) {
	f := newAgentFixture(t)
	switchLocalMachines(t, f.db, true)
	assert.Equal(t, http.StatusForbidden, f.mirrorAccess(t, f.terminal), "no tab connected")

	conn, _ := f.dial(t, f.run.ID, f.learner, "member")
	require.NotNil(t, conn)
	require.Eventually(t, func() bool { return f.agents.Connected(f.terminal) }, time.Second, 10*time.Millisecond)
	assert.Equal(t, http.StatusNoContent, f.mirrorAccess(t, f.terminal))

	assert.Equal(t, http.StatusForbidden, f.mirrorAccess(t, "local-"+uuid.NewString()), "an unknown machine")
	assert.Equal(t, http.StatusForbidden, f.mirrorAccess(t, "incus-session"), "not a machine in a browser")
	assert.Equal(t, http.StatusForbidden, f.mirrorAccess(t, ""), "no machine named")

	switchLocalMachines(t, f.db, false)
	assert.Equal(t, http.StatusForbidden, f.mirrorAccess(t, f.terminal), "the switch off refuses at once, even an open run")

	switchLocalMachines(t, f.db, true)
	require.NoError(t, endTerminal(f.db)(f.terminal))
	assert.Equal(t, http.StatusForbidden, f.mirrorAccess(t, f.terminal), "an ended run")
}
