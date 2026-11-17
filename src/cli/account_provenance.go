package cli

import (
	"fmt"
	"os"
	"time"

	"soli/formations/src/organizations/services"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"gorm.io/gorm"
)

const (
	BACKFILL_ACCOUNT_PROVENANCE_FLAG = "backfill-account-provenance"
	CREATED_BEFORE_FLAG              = "created-before"
)

type casdoorAccounts struct{}

func (casdoorAccounts) GetUsers() ([]*casdoorsdk.User, error) { return casdoorsdk.GetUsers() }

// RunAccountProvenanceBackfill runs the one-off provenance backfill against the
// identity provider and prints its report; a failure exits with status 1.
// createdBefore is the deploy time of the release that records provenance
// itself: memberships created from then on carry it already.
func RunAccountProvenanceBackfill(db *gorm.DB, createdBefore string, dryRun bool) {
	cutoff, err := time.Parse(time.RFC3339, createdBefore)
	if err != nil {
		fmt.Fprintf(os.Stderr, "--%s requires --%s <RFC3339>, the deploy time of this release (e.g. 2026-11-20T14:00:00Z); got %q\n",
			BACKFILL_ACCOUNT_PROVENANCE_FLAG, CREATED_BEFORE_FLAG, createdBefore)
		os.Exit(1)
	}
	report, err := services.BackfillAccountProvenance(db, casdoorAccounts{}, cutoff, dryRun)
	if err != nil {
		fmt.Fprintf(os.Stderr, "account provenance backfill failed, nothing written: %v\n", err)
		os.Exit(1)
	}
	mode := "applied"
	if dryRun {
		mode = "dry run, nothing written"
	}
	fmt.Printf("Account provenance backfill (%s), memberships created before %s\n", mode, cutoff.Format(time.RFC3339))
	fmt.Println("  this binary ran its normal startup migrations first: run it from the deployed release's own pod")
	fmt.Printf("  marked as created by their organization: %d\n", len(report.Marked))
	fmt.Printf("  left unmarked:                           %d\n", len(report.LeftUnmarked))
	fmt.Printf("  erasure dates cleared:                   %d\n", len(report.ErasureDatesCleared))
	fmt.Printf("  accounts blocked by an unmarked offboarding: %d\n", len(report.BlockedAccounts))
	fmt.Printf("  marked members offboarded without erasure date: %d\n", len(report.OffboardedWithoutErasureDate))

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
	fmt.Println("\nMarked members offboarded without erasure date (organization, user): offboarded between the deploy and this run,")
	fmt.Println("they were not blocked and have no erasure date; reinstate and offboard them again if blocking and erasure were intended:")
	for _, ref := range report.OffboardedWithoutErasureDate {
		fmt.Printf("  %s %s\n", ref.OrganizationID, ref.UserID)
	}
}
