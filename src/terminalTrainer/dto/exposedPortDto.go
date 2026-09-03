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
type ExposedPortResponse struct {
	ID        uuid.UUID `json:"id"`
	Port      int       `json:"port"`
	Slug      string    `json:"slug"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}
