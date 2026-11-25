package services

import (
	"time"

	"soli/formations/src/terminalTrainer/dto"
)

// sessionHost runs the machine behind a terminal session: it creates it,
// stops it, starts it again and deletes it. The callers keep every decision
// around those calls (budget, expiry, persistence, state of the local row);
// the host only does what it is asked.
type sessionHost interface {
	// createSession provisions a session and answers once it runs. A refusal
	// for the key's budget comes back as a *BudgetRejection.
	createSession(backend string, body map[string]any, userAPIKey string) (*dto.TerminalTrainerSessionResponse, error)
	// stopSession stops a session: returns the time until which a persistent
	// session keeps its disk, nil for an ephemeral one, which is gone.
	stopSession(sessionID, userAPIKey string) (*time.Time, error)
	// startSession starts a stopped session again for expirySeconds (0: the
	// host's default) and returns its new expiry in unix seconds, 0 if unknown.
	// An error wrapping ErrContainerGone means there is nothing left to start.
	startSession(sessionID, userAPIKey string, expirySeconds int) (int64, error)
	// deleteSession destroys a session and its disk.
	deleteSession(sessionID, userAPIKey string) error
}

var (
	_ sessionHost = (*terminalProxyClient)(nil) // tt-backend's Incus containers
	_ sessionHost = localHost{}                 // the learner's browser tab
	_ sessionHost = sessionHosts{}              // either, by session
)
