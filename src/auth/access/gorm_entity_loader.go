package access

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
//
// entityName is the Go struct name (e.g. "Terminal", "ScenarioSession") and
// fieldName is the Go field name (e.g. "UserID", "PurchaserUserID"), exactly
// as declared in route permissions (RoutePermission.AccessRule). Both are
// translated to their database identifiers via GORM's NamingStrategy before
// being interpolated into SQL.
//
// Assumption: callers pass Go names (PascalCase). The entity registry only
// uses Go struct names, so this is the production contract. NamingStrategy
// is idempotent for already-snake_case+plural names (the underlying
// schema.toDBName + inflection logic is a no-op on properly normalized
// inputs), so passing a pre-translated name still works in practice.
func (l *GormEntityLoader) GetOwnerField(entityName string, entityID string, fieldName string) (string, error) {
	if entityID == "" {
		return "", errors.New("entity ID must not be empty")
	}

	tableName := l.db.NamingStrategy.TableName(entityName)
	columnName := l.db.NamingStrategy.ColumnName(tableName, fieldName)

	if err := validateSQLIdentifier(tableName); err != nil {
		return "", fmt.Errorf("unsafe table name: %w", err)
	}
	if err := validateSQLIdentifier(columnName); err != nil {
		return "", fmt.Errorf("unsafe column name: %w", err)
	}

	var value string
	query := fmt.Sprintf("SELECT %s FROM %s WHERE id = ?", columnName, tableName)
	result := l.db.Raw(query, entityID).Scan(&value)
	if result.Error != nil {
		return "", result.Error
	}
	if result.RowsAffected == 0 {
		return "", fmt.Errorf("entity not found in %s with id %s", tableName, entityID)
	}

	return value, nil
}
