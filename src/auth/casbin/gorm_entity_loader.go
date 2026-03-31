package casbin

import (
	"errors"
	"fmt"
	"regexp"

	"gorm.io/gorm"
)

// safeIdentifier matches valid SQL identifiers (letters, digits, underscores, starting with letter or underscore).
var safeIdentifier = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// validateSQLIdentifier ensures a name contains only safe characters for SQL identifier interpolation.
func validateSQLIdentifier(name string) error {
	if !safeIdentifier.MatchString(name) {
		return fmt.Errorf("invalid SQL identifier: %q", name)
	}
	return nil
}

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

	if err := validateSQLIdentifier(entityName); err != nil {
		return "", fmt.Errorf("unsafe entity name: %w", err)
	}
	if err := validateSQLIdentifier(fieldName); err != nil {
		return "", fmt.Errorf("unsafe field name: %w", err)
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
