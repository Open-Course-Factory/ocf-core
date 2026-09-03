package models

import (
	entityManagementModels "soli/formations/src/entityManagement/models"
	"time"

	"github.com/google/uuid"
)

// ExposedPort records a user-initiated publication of a port from inside a
// running terminal session to a public URL, served by an operator-run
// Traefik reverse-proxy instance that discovers active exposures via
// GET /internal/traefik/dynamic-config (see
// src/terminalTrainer/routes/traefikConfigController.go).
//
// A row here is purely a routing intent + the container coordinates
// resolved from tt-backend at creation time — it does not itself open
// anything. The port becomes reachable only once Traefik has polled the
// dynamic-config endpoint and the container is actually listening on
// ContainerPort.
type ExposedPort struct {
	entityManagementModels.BaseModel
	TerminalID uuid.UUID `gorm:"type:uuid;not null;index" json:"terminal_id"`
	// SessionID is denormalized from Terminal.SessionID so the Traefik
	// config query and lifecycle cleanup don't need to join back to
	// terminals for the common case.
	SessionID string `gorm:"type:varchar(255);not null;index" json:"session_id"`
	UserID    string `gorm:"type:varchar(255);not null;index" json:"user_id"`
	// ContainerPort is the port the user's process listens on inside the
	// sandbox (validated to 1024-65535 — see exposedPortService).
	ContainerPort int `gorm:"not null" json:"container_port"`
	// Slug is the public subdomain label (https://<slug>.<EXPOSE_DOMAIN>).
	// Randomly generated, never derived from SessionID/ContainerPort, so a
	// public URL cannot be guessed from another exposure's URL.
	Slug string `gorm:"type:varchar(32);uniqueIndex;not null" json:"slug"`
	// ContainerIP is resolved once from tt-backend's /info endpoint
	// (TerminalTrainerSessionInfo.IP) at creation time. Not re-resolved
	// periodically in this MVP — a container whose IP changes while an
	// exposure is active requires the user to re-create the exposure.
	ContainerIP string    `gorm:"type:varchar(64);not null" json:"container_ip"`
	ExpiresAt   time.Time `gorm:"not null" json:"expires_at"`
}

func (e ExposedPort) GetBaseModel() entityManagementModels.BaseModel {
	return e.BaseModel
}

func (e ExposedPort) GetReferenceObject() string {
	return "ExposedPort"
}
