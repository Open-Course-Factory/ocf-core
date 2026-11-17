package cli

import (
	"fmt"
	"os"
	"time"

	"soli/formations/src/organizations/services"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"gorm.io/gorm"
)

const BACKFILL_ACCOUNT_PROVENANCE_FLAG = "backfill-account-provenance"

// accountProvenanceCutoff is the release that records provenance itself: the
// backfill reads only memberships created before it.
var accountProvenanceCutoff = time.Date(2026, time.December, 1, 0, 0, 0, 0, time.UTC)

type casdoorAccounts struct{}

func (casdoorAccounts) GetUsers() ([]*casdoorsdk.User, error) { return casdoorsdk.GetUsers() }

// RunAccountProvenanceBackfill runs the one-off provenance backfill against the
// identity provider and prints its report; a failure exits with status 1.
func RunAccountProvenanceBackfill(db *gorm.DB, dryRun bool) {
	report, err := services.BackfillAccountProvenance(db, casdoorAccounts{}, accountProvenanceCutoff, dryRun)
	if err != nil {
		fmt.Fprintf(os.Stderr, "account provenance backfill failed, nothing written: %v\n", err)
		os.Exit(1)
	}
	mode := "applied"
	if dryRun {
		mode = "dry run, nothing written"
	}
	fmt.Printf("Account provenance backfill (%s), memberships created before %s\n", mode, accountProvenanceCutoff.Format(time.RFC3339))
	fmt.Printf("  marked as created by their organization: %d\n", len(report.Marked))
	fmt.Printf("  left unmarked:                           %d\n", len(report.LeftUnmarked))
	fmt.Printf("  erasure dates cleared:                   %d\n", len(report.ErasureDatesCleared))
	fmt.Printf("  accounts blocked by an unmarked offboarding: %d\n", len(report.BlockedAccounts))

	unmarked := map[string][]string{}
	var orgs []string
	for _, ref := range report.LeftUnmarked {
		org := ref.OrganizationID.String()
		if _, seen := unmarked[org]; !seen {
			orgs = append(orgs, org)
		}
		unmarked[org] = append(unmarked[org], ref.UserID)
	}
	fmt.Println("\nLeft unmarked, per team organization:")
	for _, org := range orgs {
		fmt.Printf("  %s (%d): %v\n", org, len(unmarked[org]), unmarked[org])
	}
	fmt.Println("\nBlocked accounts to review by hand (organization, user):")
	for _, ref := range report.BlockedAccounts {
		fmt.Printf("  %s %s\n", ref.OrganizationID, ref.UserID)
	}
}
