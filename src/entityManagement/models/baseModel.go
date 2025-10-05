package models

import (
	"reflect"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

type BaseModel struct {
	gorm.Model
	ID       uuid.UUID      `gorm:"type:uuid;primarykey"`
	OwnerIDs pq.StringArray `gorm:"type:text[]"`
}

func (b *BaseModel) BeforeCreate(tx *gorm.DB) (err error) {
	if b.ID == uuid.Nil {
		// Use UUIDv7 for time-ordered, sortable IDs (required for cursor pagination)
		// UUIDv7 contains a timestamp in the first 48 bits, making it naturally sortable
		// This ensures "WHERE id > cursor ORDER BY id ASC" works correctly
		b.ID, err = uuid.NewV7()
		if err != nil {
			// Fallback to random UUID if v7 generation fails (should never happen)
			b.ID = uuid.New()
		}
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
