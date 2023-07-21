package models

import (
	"github.com/google/uuid"
)

type SshKey struct {
	BaseModel
	Name       string    `gorm:"type:varchar(255)"`
	PrivateKey string    `gorm:"type:text"`
	UserID     uuid.UUID `gorm:"type:uuid;primarykey"`
}
