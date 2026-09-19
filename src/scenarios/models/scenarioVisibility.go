package models

import "gorm.io/gorm"

// PublicCatalogue is the one definition of "public scenario": a platform
// scenario (no organisation) flagged public. An organisation's scenarios
// never leave the organisation, so the flag is ignored on them here and
// refused on write by ScenarioAuthorizationHook. Every reader — learner
// catalogue, editor list, class picker, copy sources — goes through this
// scope so they cannot disagree.
func PublicCatalogue(db *gorm.DB) *gorm.DB {
	return NotArchived(db).Where("scenarios.is_public = ? AND scenarios.organization_id IS NULL", true)
}

// InPublicCatalogue is PublicCatalogue for a scenario already in memory. Keep
// the two in step: the scope is what the database filters, this is what a
// controller decides about one row.
func (s *Scenario) InPublicCatalogue() bool {
	return s.IsPublic && s.OrganizationID == nil && s.ArchivedAt == nil
}

// BeforeSave keeps the flag honest on every write path that carries a struct
// (imports, uploads, copies, the org and group create routes): an
// organisation's scenario is never public. Explicit attempts through the API
// are refused earlier, with a message, by ScenarioAuthorizationHook.
func (s *Scenario) BeforeSave(*gorm.DB) error {
	if s.OrganizationID != nil {
		s.IsPublic = false
	}
	return nil
}
