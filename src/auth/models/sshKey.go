package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type SshKey struct {
	gorm.Model
	ID         uuid.UUID `gorm:"type:uuid;primarykey"`
	Name       string    `gorm:"type:varchar(255)"`
	PrivateKey string    `gorm:"type:text"`
	UserID     uuid.UUID `gorm:"type:uuid;primarykey"`
}

func (s *SshKey) BeforeCreate(tx *gorm.DB) (err error) {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}

	return
}
