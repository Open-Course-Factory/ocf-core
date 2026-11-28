package services

import (
	"fmt"
	"time"

	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/terminalTrainer/models"

	"github.com/google/uuid"
)

// defaultLocalExpiry is how long a local session lasts when its plan sets no
// duration, as tt-backend's default does for a container.
const defaultLocalExpiry = time.Hour

// localHost is the host of machines that run in the learner's browser tab.
// There is nothing to build or stop on a server: it names the session and
// gives it its expiry, and the tab does the rest.
type localHost struct{}

func (localHost) createSession(_ string, body map[string]any, _ string) (*dto.TerminalTrainerSessionResponse, error) {
	expiry := defaultLocalExpiry
	if seconds, ok := body["expiry"].(int); ok && seconds > 0 {
		expiry = time.Duration(seconds) * time.Second
	}
	return &dto.TerminalTrainerSessionResponse{
		SessionID: models.LocalSessionIDPrefix + uuid.NewString(),
		ExpiresAt: time.Now().Add(expiry).Unix(),
		Backend:   models.LocalBackend,
	}, nil
}

// stopSession: a local machine keeps nothing, so stopping it ends it.
func (localHost) stopSession(string, string) (*time.Time, error) { return nil, nil }

// startSession: a local machine has no pause, so there is never one to start again.
func (localHost) startSession(string, string, int) (int64, error) {
	return 0, fmt.Errorf("%w: a machine in the browser cannot be paused", ErrContainerGone)
}

func (localHost) deleteSession(string, string) error { return nil }

// sessionHosts sends each call to the host of its session: the learner's
// browser for a local one, tt-backend's containers for the others.
type sessionHosts struct {
	incus sessionHost
	local sessionHost
}

func newSessionHosts(incus sessionHost) sessionHosts {
	return sessionHosts{incus: incus, local: localHost{}}
}

func (h sessionHosts) of(sessionID string) sessionHost {
	if models.IsLocalSessionID(sessionID) {
		return h.local
	}
	return h.incus
}

// createSession creates a container: a local session is no host's to create
// (StartLocalSession stores its row itself).
func (h sessionHosts) createSession(backend string, body map[string]any, userAPIKey string) (*dto.TerminalTrainerSessionResponse, error) {
	return h.incus.createSession(backend, body, userAPIKey)
}

func (h sessionHosts) stopSession(sessionID, userAPIKey string) (*time.Time, error) {
	return h.of(sessionID).stopSession(sessionID, userAPIKey)
}

func (h sessionHosts) startSession(sessionID, userAPIKey string, expirySeconds int) (int64, error) {
	return h.of(sessionID).startSession(sessionID, userAPIKey, expirySeconds)
}

func (h sessionHosts) deleteSession(sessionID, userAPIKey string) error {
	return h.of(sessionID).deleteSession(sessionID, userAPIKey)
}
