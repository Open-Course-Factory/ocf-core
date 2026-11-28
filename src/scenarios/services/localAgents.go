package services

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"sync"
	"time"

	terminalServices "soli/formations/src/terminalTrainer/services"

	"github.com/gorilla/websocket"
)

// The agent of a local machine is the learner's tab, connected over a
// WebSocket: it runs in the machine it booted what the engine asks, and
// answers. The protocol is two requests, each answered by its id:
//
//	{"id":1,"op":"exec","command":[...],"env":{...},"timeout":10}
//	  -> {"id":1,"exit_code":0,"stdout":"...","stderr":"..."}
//	{"id":2,"op":"console","data":"text\n"}
//	  -> {"id":2} or {"id":2,"error":"no_console"}
//
// and two events from the tab: when the learner's shell opens, and the key
// its machine asks the Debian mirror with (nginx.conf /local-machine/apt):
//
//	{"event":"console_attached"}
//	{"event":"mirror_key","key":"..."}
//
// Everything an agent sends is untrusted input: the learner owns the tab.

// ErrNoMachineAgent: the learner's tab is not connected, so nothing can run in
// their machine. Never read as a pass.
var ErrNoMachineAgent = errors.New("the machine in the learner's browser is not connected")

// ErrAgentAlreadyConnected: a run's machine is open in one tab at a time.
var ErrAgentAlreadyConnected = errors.New("this run's machine is already open in another tab")

// ErrMachineLost: the tab booted a new machine (a reload), so the run's
// machine and everything built in it are gone.
var ErrMachineLost = errors.New("the machine of this run was lost")

// Close codes the tab acts on.
const (
	// AgentCloseEnded: the run's terminal has ended; the machine stops.
	AgentCloseEnded = 4000
	// AgentCloseOtherTab: the run's machine is open in another tab.
	AgentCloseOtherTab = 4409
	// AgentCloseMachineLost: the run's terminal was ended because its
	// machine was lost; the tab rebuilds the run.
	AgentCloseMachineLost = 4410
)

const (
	// MaxAgentOutput bounds each of stdout and stderr kept from an answer.
	MaxAgentOutput = 64 << 10
	// maxAgentMessage bounds one message from an agent; a larger one ends
	// the connection.
	maxAgentMessage = 1 << 20
	agentPingEvery  = 30 * time.Second
	agentSilence    = 75 * time.Second // no message nor pong for this long: gone
	agentWriteWait  = 10 * time.Second
)

type agentRequest struct {
	ID      int               `json:"id"`
	Op      string            `json:"op"`
	Command []string          `json:"command,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Timeout int               `json:"timeout,omitempty"`
	Data    string            `json:"data,omitempty"`
}

type agentReply struct {
	ID       int    `json:"id"`
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	Error    string `json:"error"`
	Event    string `json:"event"`
	Key      string `json:"key"`
}

// maxMirrorKey bounds the key an agent announces; the tab sends a UUID.
const maxMirrorKey = 128

// LocalAgents holds the connected agents, one per terminal session.
//
// ponytail: in memory, so one ocf-core replica (or sticky routing on the run);
// a shared hub when there are several. A restart forgets which machine each
// terminal ran on, so a reload during one is not seen as a lost machine.
type LocalAgents struct {
	mu       sync.Mutex
	agents   map[string]*localAgent
	arrivals map[string]chan struct{} // closed when the terminal's agent connects
	boots    map[string]string        // the machine each terminal's agent came from
	away     map[string]time.Time     // since when each terminal has had no agent
	building map[string]int           // terminals whose world is being built
}

func NewLocalAgents() *LocalAgents {
	return &LocalAgents{
		agents:   map[string]*localAgent{},
		arrivals: map[string]chan struct{}{},
		boots:    map[string]string{},
		away:     map[string]time.Time{},
		building: map[string]int{},
	}
}

var platformAgents = NewLocalAgents()

// Agents is the hub the platform serves agents from.
func Agents() *LocalAgents { return platformAgents }

// MirrorKeyMatches reports whether terminalID's tab is connected and
// announced key as its machine's key for the Debian mirror. No key matches
// an empty one.
func (h *LocalAgents) MirrorKeyMatches(terminalID, key string) bool {
	h.mu.Lock()
	agent := h.agents[terminalID]
	h.mu.Unlock()
	if agent == nil || key == "" {
		return false
	}
	agent.mu.Lock()
	known := agent.mirrorKey
	agent.mu.Unlock()
	return known != "" && subtle.ConstantTimeCompare([]byte(known), []byte(key)) == 1
}

// Connected reports whether terminalID's agent is connected now.
func (h *LocalAgents) Connected(terminalID string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.agents[terminalID] != nil
}

// Admits reports whether the machine boot may serve terminalID: not while
// another tab serves it, and not when the terminal ran on another machine.
// The same machine is admitted while its old connection is still held: after
// a network change the server may not have seen that socket die yet.
func (h *LocalAgents) Admits(terminalID, boot string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.admits(terminalID, boot)
}

func (h *LocalAgents) admits(terminalID, boot string) error {
	if h.agents[terminalID] != nil && h.boots[terminalID] != boot {
		return ErrAgentAlreadyConnected
	}
	if known := h.boots[terminalID]; known != "" && known != boot {
		return ErrMachineLost
	}
	return nil
}

// Building reports whether terminalID's world is being built: a machine lost
// meanwhile (a reload) is taken in its place, and the build starts over on it.
func (h *LocalAgents) Building(terminalID string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.building[terminalID] > 0
}

// ReplaceMachine makes the next machine to connect terminalID's own.
func (h *LocalAgents) ReplaceMachine(terminalID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.boots, terminalID)
}

// startBuild marks terminalID as being built until the returned func is called.
func (h *LocalAgents) startBuild(terminalID string) func() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.building[terminalID]++
	return func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if h.building[terminalID]--; h.building[terminalID] <= 0 {
			delete(h.building, terminalID)
		}
	}
}

// awaitReturn waits up to wait for terminalID to have an agent, one that
// connected before included.
func (h *LocalAgents) awaitReturn(terminalID string, wait time.Duration) bool {
	h.mu.Lock()
	if h.agents[terminalID] != nil {
		h.mu.Unlock()
		return true
	}
	arrived, ok := h.arrivals[terminalID]
	if !ok {
		arrived = make(chan struct{})
		h.arrivals[terminalID] = arrived
	}
	h.mu.Unlock()
	select {
	case <-arrived:
		return h.Connected(terminalID)
	case <-time.After(wait):
		return false
	}
}

// Attach takes conn, from the machine boot, as terminalID's agent, closing
// that machine's stale connection if one is still held. Serve it next.
func (h *LocalAgents) Attach(terminalID, boot string, conn *websocket.Conn) (*localAgent, error) {
	agent, stale, err := h.attach(terminalID, boot, conn)
	if stale != nil {
		stale.close(websocket.CloseNormalClosure, "replaced by a new connection of the same machine")
	}
	return agent, err
}

func (h *LocalAgents) attach(terminalID, boot string, conn *websocket.Conn) (agent, stale *localAgent, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.admits(terminalID, boot); err != nil {
		return nil, nil, err
	}
	stale = h.agents[terminalID]
	agent = &localAgent{
		hub: h, terminalID: terminalID, conn: conn,
		pending: map[int]chan agentReply{}, done: make(chan struct{}),
	}
	h.agents[terminalID] = agent
	h.boots[terminalID] = boot
	delete(h.away, terminalID)
	if arrived, ok := h.arrivals[terminalID]; ok {
		close(arrived)
		delete(h.arrivals, terminalID)
	}
	return agent, stale, nil
}

// await returns terminalID's agent. One that never connected is waited for up
// to bootWait — the tab is booting its machine; one that connected and left
// is not: the learner closed or reloaded the tab.
func (h *LocalAgents) await(terminalID string, bootWait time.Duration) (*localAgent, error) {
	h.mu.Lock()
	if agent := h.agents[terminalID]; agent != nil {
		h.mu.Unlock()
		return agent, nil
	}
	if h.boots[terminalID] != "" {
		h.mu.Unlock()
		return nil, ErrNoMachineAgent
	}
	arrived, ok := h.arrivals[terminalID]
	if !ok {
		arrived = make(chan struct{})
		h.arrivals[terminalID] = arrived
	}
	h.mu.Unlock()

	select {
	case <-arrived:
		h.mu.Lock()
		defer h.mu.Unlock()
		if agent := h.agents[terminalID]; agent != nil {
			return agent, nil
		}
		return nil, ErrNoMachineAgent
	case <-time.After(bootWait):
		return nil, ErrNoMachineAgent
	}
}

func (h *LocalAgents) detach(agent *localAgent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.agents[agent.terminalID] == agent {
		delete(h.agents, agent.terminalID)
		h.away[agent.terminalID] = time.Now()
	}
}

// awaySince returns since when terminalID has had no agent, zero while one
// is connected. A terminal the hub has not seen is counted from now.
func (h *LocalAgents) awaySince(terminalID string) time.Time {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.agents[terminalID] != nil {
		return time.Time{}
	}
	since, ok := h.away[terminalID]
	if !ok {
		since = time.Now()
		h.away[terminalID] = since
	}
	return since
}

// connectedTerminals lists the terminals with an agent now.
func (h *LocalAgents) connectedTerminals() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	ids := make([]string, 0, len(h.agents))
	for id := range h.agents {
		ids = append(ids, id)
	}
	return ids
}

// forget drops what the hub knows of an ended terminal.
func (h *LocalAgents) forget(terminalID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.boots, terminalID)
	delete(h.away, terminalID)
}

// Close tells terminalID's agent, if connected, why it is closed, and closes it.
func (h *LocalAgents) Close(terminalID string, code int, reason string) {
	h.mu.Lock()
	agent := h.agents[terminalID]
	h.mu.Unlock()
	if agent != nil {
		agent.close(code, reason)
	}
}

// CloseConn closes a connection that was never taken as an agent.
func CloseConn(conn *websocket.Conn, code int, reason string) {
	_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(code, reason), time.Now().Add(agentWriteWait))
	_ = conn.Close()
}

type localAgent struct {
	hub        *LocalAgents
	terminalID string
	conn       *websocket.Conn
	writeMu    sync.Mutex

	mu        sync.Mutex
	nextID    int
	pending   map[int]chan agentReply
	done      chan struct{}
	mirrorKey string // what its machine asks the Debian mirror with; "" until announced
}

// Serve reads the agent's answers until it goes, keeping it alive with pings,
// then detaches it. It blocks for the connection's life.
func (a *localAgent) Serve() {
	defer func() {
		close(a.done)
		a.hub.detach(a)
		_ = a.conn.Close()
	}()

	a.conn.SetReadLimit(maxAgentMessage)
	alive := func() { _ = a.conn.SetReadDeadline(time.Now().Add(agentSilence)) }
	alive()
	a.conn.SetPongHandler(func(string) error { alive(); return nil })

	stopPings := make(chan struct{})
	defer close(stopPings)
	go a.ping(stopPings)

	for {
		var reply agentReply
		if err := a.conn.ReadJSON(&reply); err != nil {
			return
		}
		alive()
		if reply.Event == "console_attached" {
			// What tt-backend reports for a container's console: a foreground
			// script left pending by the build is typed now. In its own
			// goroutine, because typing it is a request this loop must answer.
			go terminalServices.ReportConsoleAttach(a.terminalID)
			continue
		}
		if reply.Event == "mirror_key" {
			if len(reply.Key) <= maxMirrorKey {
				a.mu.Lock()
				a.mirrorKey = reply.Key
				a.mu.Unlock()
			}
			continue
		}
		a.mu.Lock()
		waiting := a.pending[reply.ID]
		a.mu.Unlock()
		if waiting != nil {
			select {
			case waiting <- reply:
			default: // answered twice: the first answer stands
			}
		}
	}
}

func (a *localAgent) close(code int, reason string) {
	a.writeMu.Lock()
	defer a.writeMu.Unlock()
	CloseConn(a.conn, code, reason)
}

func (a *localAgent) ping(stop <-chan struct{}) {
	ticker := time.NewTicker(agentPingEvery)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			a.writeMu.Lock()
			err := a.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(agentWriteWait))
			a.writeMu.Unlock()
			if err != nil {
				return
			}
		}
	}
}

// call sends req and waits up to wait for its answer.
func (a *localAgent) call(req agentRequest, wait time.Duration) (agentReply, error) {
	answer := make(chan agentReply, 1)
	a.mu.Lock()
	a.nextID++
	req.ID = a.nextID
	a.pending[req.ID] = answer
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		delete(a.pending, req.ID)
		a.mu.Unlock()
	}()

	a.writeMu.Lock()
	_ = a.conn.SetWriteDeadline(time.Now().Add(agentWriteWait))
	err := a.conn.WriteJSON(req)
	a.writeMu.Unlock()
	if err != nil {
		return agentReply{}, fmt.Errorf("%w: %v", ErrNoMachineAgent, err)
	}

	select {
	case reply := <-answer:
		return reply, nil
	case <-a.done:
		return agentReply{}, ErrNoMachineAgent
	case <-time.After(wait):
		return agentReply{}, fmt.Errorf("the machine in the learner's browser did not answer within %s", wait)
	}
}
