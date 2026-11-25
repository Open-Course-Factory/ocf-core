package services

import (
	"errors"
	"fmt"
	"sync"
	"time"

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
// Everything an agent sends is untrusted input: the learner owns the tab.

// ErrNoMachineAgent: the learner's tab is not connected, so nothing can run in
// their machine. Never read as a pass.
var ErrNoMachineAgent = errors.New("the machine in the learner's browser is not connected")

// ErrAgentAlreadyConnected: a run's machine is open in one tab at a time.
var ErrAgentAlreadyConnected = errors.New("this run's machine is already open in another tab")

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
}

// LocalAgents holds the connected agents, one per terminal session.
//
// ponytail: in memory, so one ocf-core replica (or sticky routing on the run);
// a shared hub when there are several. `seen` and `arrivals` keep one small
// entry per local terminal for the process's life.
type LocalAgents struct {
	mu       sync.Mutex
	agents   map[string]*localAgent
	arrivals map[string]chan struct{} // closed when the terminal's agent connects
	seen     map[string]bool          // terminals whose agent connected once
}

func NewLocalAgents() *LocalAgents {
	return &LocalAgents{
		agents:   map[string]*localAgent{},
		arrivals: map[string]chan struct{}{},
		seen:     map[string]bool{},
	}
}

var platformAgents = NewLocalAgents()

// Agents is the hub the platform serves agents from.
func Agents() *LocalAgents { return platformAgents }

// Connected reports whether terminalID's agent is connected now.
func (h *LocalAgents) Connected(terminalID string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.agents[terminalID] != nil
}

// Attach takes conn as terminalID's agent. Serve it next.
func (h *LocalAgents) Attach(terminalID string, conn *websocket.Conn) (*localAgent, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.agents[terminalID] != nil {
		return nil, ErrAgentAlreadyConnected
	}
	agent := &localAgent{
		hub: h, terminalID: terminalID, conn: conn,
		pending: map[int]chan agentReply{}, done: make(chan struct{}),
	}
	h.agents[terminalID] = agent
	h.seen[terminalID] = true
	if arrived, ok := h.arrivals[terminalID]; ok {
		close(arrived)
		delete(h.arrivals, terminalID)
	}
	return agent, nil
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
	if h.seen[terminalID] {
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
	}
}

type localAgent struct {
	hub        *LocalAgents
	terminalID string
	conn       *websocket.Conn
	writeMu    sync.Mutex

	mu      sync.Mutex
	nextID  int
	pending map[int]chan agentReply
	done    chan struct{}
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
