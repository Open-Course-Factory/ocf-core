package casbin

import (
	"errors"
	"fmt"

	"gorm.io/gorm"
)

// GormEntityLoader implements EntityLoader using a GORM database connection.
type GormEntityLoader struct {
	db *gorm.DB
}

// NewGormEntityLoader creates an EntityLoader backed by GORM.
func NewGormEntityLoader(db *gorm.DB) *GormEntityLoader {
	return &GormEntityLoader{db: db}
}

// GetOwnerField retrieves a single field value from an entity row.
// entityName is the table name, entityID is the primary key, and fieldName is the column name.
func (l *GormEntityLoader) GetOwnerField(entityName string, entityID string, fieldName string) (string, error) {
	if entityID == "" {
		return "", errors.New("entity ID must not be empty")
	}

	var value string
	query := fmt.Sprintf("SELECT %s FROM %s WHERE id = ?", fieldName, entityName)
	result := l.db.Raw(query, entityID).Scan(&value)
	if result.Error != nil {
		return "", result.Error
	}
	if result.RowsAffected == 0 {
		return "", fmt.Errorf("entity not found in %s with id %s", entityName, entityID)
	}

	return value, nil
}
