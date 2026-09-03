package models

import (
	"time"

	"gorm.io/gorm"
)

// Archivable is the column an entity embeds — next to BaseModel, never inside
// it — to opt into the framework's archiving capability. Together with
// `Archivable: true` on its registration it yields the archive/unarchive
// actions, the four archive hook points and the default not-archived list
// scope. An archived row is retired, not deleted: it keeps its ID and every
// reference to it stays valid.
type Archivable struct {
	ArchivedAt *time.Time `gorm:"index" json:"archived_at,omitempty"`
}

// IsArchived reports whether the row carries an archive stamp.
func (a Archivable) IsArchived() bool { return a.ArchivedAt != nil }

// ArchivableModel is satisfied by any model embedding Archivable. The
// registration service checks it at boot so an entity cannot declare itself
// archivable without the column.
type ArchivableModel interface {
	IsArchived() bool
}

// NotArchivedSQL is the one spelling of the "row is not archived" predicate.
// Both NotArchived (hand-written queries) and the generic list filter emit it,
// so the two can never drift apart.
func NotArchivedSQL(table string) string {
	if table == "" {
		return "archived_at IS NULL"
	}
	return table + ".archived_at IS NULL"
}

// NotArchived is the canonical scope for hand-written queries that must skip
// archived rows: db.Scopes(models.NotArchived("scenarios")).
func NotArchived(table string) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where(NotArchivedSQL(table))
	}
}
