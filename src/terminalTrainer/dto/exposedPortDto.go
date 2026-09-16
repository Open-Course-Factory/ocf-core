package dto

import (
	"time"

	"github.com/google/uuid"
)

// CreateExposedPortInput is the request body for
// POST /api/v1/terminals/:id/exposed-ports.
type CreateExposedPortInput struct {
	Port int `binding:"required" json:"port"`
}

// ExposedPortResponse is the ocf-core-owned shape returned to the frontend
// for a published port. URL is precomputed server-side from EXPOSE_DOMAIN so
// the frontend never has to know the domain convention.
// AdminExposedPortResponse is the platform-admin view of an active exposure:
// the owner-facing shape plus who holds it and where the container runs.
type AdminExposedPortResponse struct {
	ExposedPortResponse
	UserID    string `json:"user_id"`
	SessionID string `json:"session_id"`
	Backend   string `json:"backend"`
}

type ExposedPortResponse struct {
	ID        uuid.UUID `json:"id"`
	Port      int       `json:"port"`
	Slug      string    `json:"slug"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}
