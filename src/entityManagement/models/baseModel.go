package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type BaseModel struct {
	gorm.Model
	ID uuid.UUID `gorm:"type:uuid;primarykey"`
}

type InterfaceWithBaseModel interface {
	GetBaseModel() BaseModel
}

func (b *BaseModel) BeforeCreate(tx *gorm.DB) (err error) {
	if b.ID == uuid.Nil {
		b.ID = uuid.New()
	}

	return
}

func getBaseModel(obj interface{}) (BaseModel, bool) {
	if v, ok := obj.(InterfaceWithBaseModel); ok {
		return v.GetBaseModel(), true
	}
	return BaseModel{}, false
}

func ExtractBaseModelFromAny(obj interface{}) (BaseModel, bool) {
	return getBaseModel(obj)
}
