package services

import (
	"fmt"
	"regexp"
	"strconv"
	"time"

	organizationModels "soli/formations/src/organizations/models"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// IdentityAccountLister lists every account of the identity provider.
type IdentityAccountLister interface {
	GetUsers() ([]*casdoorsdk.User, error)
}

// MemberRef names one membership row.
type MemberRef struct {
	OrganizationID uuid.UUID
	UserID         string
}

// ProvenanceBackfillReport lists what BackfillAccountProvenance marked (or
// would mark), the team memberships it left unmarked, the erasure dates it
// cleared on those, and the accounts an unmarked membership's offboarding
// blocked, which an operator reviews by hand.
type ProvenanceBackfillReport struct {
	Marked, LeftUnmarked, ErasureDatesCleared, BlockedAccounts []MemberRef
}

// importedName is the name the import gives an account: a base, then the unix
// seconds of its creation (importedUsername).
var importedName = regexp.MustCompile(`[-_](\d{10})$`)

// BackfillAccountProvenance marks the team memberships created before cutoff
// whose organization created the account through the import, from the traces
// the import leaves: no "username" (self-signup) nor "created_by_admin"
// property, a name ending in its creation's unix seconds, and a membership
// joined within minutes of the account's creation. An unmarked offboarded
// membership loses its erasure date (the organization may no longer erase the
// account). Rows already marked are left alone, so a second run changes
// nothing. A dry run reports and writes nothing.
func BackfillAccountProvenance(db *gorm.DB, accounts IdentityAccountLister, cutoff time.Time, dryRun bool) (ProvenanceBackfillReport, error) {
	var report ProvenanceBackfillReport
	users, err := accounts.GetUsers()
	if err != nil {
		return report, fmt.Errorf("failed to list identity accounts: %w", err)
	}
	byID := make(map[string]*casdoorsdk.User, len(users))
	for _, u := range users {
		byID[u.Id] = u
	}

	var rows []organizationModels.OrganizationMember
	if err := db.Joins("JOIN organizations ON organizations.id = organization_members.organization_id").
		Where("organizations.organization_type = ? AND organization_members.account_created_by_organization = ? AND organization_members.created_at < ?",
			organizationModels.OrgTypeTeam, false, cutoff).
		Order("organization_members.organization_id, organization_members.user_id").
		Find(&rows).Error; err != nil {
		return report, fmt.Errorf("failed to list team memberships: %w", err)
	}

	for _, row := range rows {
		ref := MemberRef{OrganizationID: row.OrganizationID, UserID: row.UserID}
		if createdThroughImport(byID[row.UserID], row.JoinedAt) {
			report.Marked = append(report.Marked, ref)
			continue
		}
		report.LeftUnmarked = append(report.LeftUnmarked, ref)
		if row.LeftAt != nil && row.ScheduledErasureAt != nil {
			report.ErasureDatesCleared = append(report.ErasureDatesCleared, ref)
		}
		if row.AccountForbiddenByOffboarding {
			report.BlockedAccounts = append(report.BlockedAccounts, ref)
		}
	}
	if dryRun {
		return report, nil
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		for _, ref := range report.Marked {
			if err := memberRow(tx, ref).Update("account_created_by_organization", true).Error; err != nil {
				return fmt.Errorf("failed to mark %s in %s: %w", ref.UserID, ref.OrganizationID, err)
			}
		}
		for _, ref := range report.ErasureDatesCleared {
			if err := memberRow(tx, ref).Update("scheduled_erasure_at", nil).Error; err != nil {
				return fmt.Errorf("failed to clear the erasure date of %s in %s: %w", ref.UserID, ref.OrganizationID, err)
			}
		}
		return nil
	})
	return report, err
}

func memberRow(tx *gorm.DB, ref MemberRef) *gorm.DB {
	return tx.Model(&organizationModels.OrganizationMember{}).
		Where("organization_id = ? AND user_id = ?", ref.OrganizationID, ref.UserID)
}

// createdThroughImport reports whether account carries every trace the import
// leaves and was created within minutes of joinedAt.
func createdThroughImport(account *casdoorsdk.User, joinedAt time.Time) bool {
	if account == nil {
		return false
	}
	_, selfSignup := account.Properties["username"]
	_, adminCreated := account.Properties["created_by_admin"]
	if selfSignup || adminCreated {
		return false
	}
	createdAt, err := time.Parse(time.RFC3339, account.CreatedTime)
	if err != nil {
		return false
	}
	match := importedName.FindStringSubmatch(account.Name)
	if match == nil {
		return false
	}
	stamp, _ := strconv.ParseInt(match[1], 10, 64)
	return within(time.Unix(stamp, 0), createdAt, 5*time.Minute) && within(joinedAt, createdAt, 10*time.Minute)
}

func within(a, b time.Time, d time.Duration) bool {
	diff := a.Sub(b)
	return diff <= d && diff >= -d
}
