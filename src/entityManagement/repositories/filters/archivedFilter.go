package filters

import (
	"soli/formations/src/entityManagement/models"

	"gorm.io/gorm"
)

// NotArchivedKey is the sentinel filter key the generic list handler injects for
// archivable entities unless the caller passed include_archived=true. The
// double-underscore prefix guarantees it can never collide with a real entity
// field name coming from a query parameter.
const NotArchivedKey = "__not_archived"

// NotArchivedFilter hides archived rows from a list query. It emits the same
// SQL fragment as models.NotArchived so the generic list and hand-written
// queries share one definition of "archived".
type NotArchivedFilter struct{}

// NewNotArchivedFilter creates the default archive read-scope filter strategy.
func NewNotArchivedFilter() *NotArchivedFilter {
	return &NotArchivedFilter{}
}

// Priority matches the other read scopes: this is a framework scope, not a
// user-supplied field filter, so it runs ahead of the standard field strategies.
func (f *NotArchivedFilter) Priority() int {
	return 5
}

// Matches handles only the sentinel key injected by the list handler.
func (f *NotArchivedFilter) Matches(key string, value any) bool {
	return key == NotArchivedKey
}

// Apply restricts the query to rows whose archived_at is NULL.
func (f *NotArchivedFilter) Apply(query *gorm.DB, key string, value any, tableName string) *gorm.DB {
	return query.Where(models.NotArchivedSQL(tableName))
}
