package models

import "gorm.io/gorm"

// PublicCatalogue restricts a query to the scenarios offered to everyone: the
// seeded catalogue rather than one organisation's authoring. Archiving retires
// a scenario for everybody, so the rule is public AND not archived — one home
// for it, whether a learner browses, a teacher assigns or an org copies.
//
// Applies its clauses immediately (not via Scopes) so it can also be nested in
// an Or(...) group.
func PublicCatalogue(db *gorm.DB) *gorm.DB {
	return NotArchived(db).Where("scenarios.is_public = ?", true)
}
