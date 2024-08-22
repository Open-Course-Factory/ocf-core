package models

import (
	"reflect"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type BaseModel struct {
	gorm.Model
	ID       uuid.UUID `gorm:"type:uuid;primarykey"`
	OwnerIDs []string  `gorm:"serializer:json"`
}

func (b *BaseModel) BeforeCreate(tx *gorm.DB) (err error) {
	if b.ID == uuid.Nil {
		b.ID = uuid.New()
	}

	return
}

type InterfaceWithBaseModel interface {
	GetBaseModel() BaseModel
	GetReferenceObject() string
}

func GetBaseModel(obj interface{}) (BaseModel, bool) {
	if v, ok := obj.(InterfaceWithBaseModel); ok {
		return v.GetBaseModel(), true
	}
	return BaseModel{}, false
}

func GetReferenceObject(obj interface{}) (string, bool) {
	if v, ok := obj.(InterfaceWithBaseModel); ok {
		return v.GetReferenceObject(), true
	}
	return reflect.TypeOf(obj).Name(), false
}
