package test_tools

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	orgModels "soli/formations/src/organizations/models"
)

// SeedLiveOrganization creates the live organization id (no-op when it
// exists): a class pointing at a missing organization grants nobody but
// administrators anything (access.CreatorAuthorityCondition).
func SeedLiveOrganization(db *gorm.DB, id uuid.UUID) error {
	org := orgModels.Organization{Name: "org-" + id.String(), DisplayName: "Org", OwnerUserID: "org-owner", OrganizationType: orgModels.OrgTypeTeam, IsActive: true}
	org.ID = id
	return db.Omit("Metadata", "AllowedBackends", "Members", "Groups").Clauses(clause.OnConflict{DoNothing: true}).Create(&org).Error
}
