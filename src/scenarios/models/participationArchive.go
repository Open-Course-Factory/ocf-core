package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ParticipationArchive is a frozen copy of a class's participation report, kept
// after the data it was built from is purged or erased. It has no foreign keys
// and no soft delete: it outlives the class, and an expired copy must really go.
type ParticipationArchive struct {
	ID             uuid.UUID `gorm:"type:uuid;primarykey" json:"id"`
	GroupID        uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_participation_archive_coverage" json:"group_id"`
	OrganizationID uuid.UUID `gorm:"type:uuid;not null;index" json:"organization_id"`
	Title          string    `gorm:"not null" json:"title"`
	// Coverage says which days the copy covers, so the same days are archived once.
	Coverage  string    `gorm:"not null;uniqueIndex:idx_participation_archive_coverage" json:"coverage"`
	Report    string    `gorm:"type:text;not null" json:"-"` // Postgres compresses large text values
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `gorm:"not null;index" json:"expires_at"`
}

func (a *ParticipationArchive) BeforeCreate(*gorm.DB) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	return nil
}
