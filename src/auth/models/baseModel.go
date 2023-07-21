package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type BaseModel struct {
	gorm.Model
	ID uuid.UUID `gorm:"type:uuid;primarykey"`
}

func (b *BaseModel) BeforeCreate(tx *gorm.DB) (err error) {
	if b.ID == uuid.Nil {
		b.ID = uuid.New()
	}

	return
}

func ExtractBaseFromAny(obj interface{}) (BaseModel, bool) {
	switch v := obj.(type) {
	case Group:
		return v.BaseModel, true
	case User:
		return v.BaseModel, true
	case Role:
		return v.BaseModel, true
	case Organisation:
		return v.BaseModel, true
	default:
		return BaseModel{}, false
	}
}
