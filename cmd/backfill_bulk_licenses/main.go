// Command backfill_bulk_licenses repairs legacy bulk license batches created
// before the #367 / !269 fix: it NULLs the shared stripe_subscription_id on the
// surviving license row(s) and backfills the missing rows up to total_quantity
// so every seat is assignable. It is safe to run multiple times: healthy
// (fully-provisioned, no shared stripe id) batches are skipped.
//
// Usage:
//
//	go run ./cmd/backfill_bulk_licenses          # dry-run (default)
//	go run ./cmd/backfill_bulk_licenses --apply  # commit changes
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	sqldb "soli/formations/src/db"
	"soli/formations/src/payment/backfill"
)

func main() {
	apply := flag.Bool("apply", false, "Commit changes to the database. Default is dry-run.")
	envFile := flag.String("env", ".env", "Path to .env file holding POSTGRES_* variables")
	flag.Parse()

	sqldb.InitDBConnection(*envFile)
	if sqldb.DB == nil {
		log.Fatalf("backfill_bulk_licenses: database connection not initialised")
	}

	mode := "DRY-RUN"
	if *apply {
		mode = "APPLY"
	}
	fmt.Printf("[backfill_bulk_licenses] mode=%s\n", mode)

	report, err := backfill.RunBulkLicenses(sqldb.DB, backfill.Options{Apply: *apply})
	if err != nil {
		log.Fatalf("backfill_bulk_licenses: %v", err)
	}

	for _, b := range report.Batches {
		fmt.Printf("[batch: %s] %s\n", b.BatchID, b.Reason)
	}
	fmt.Printf("[backfill_bulk_licenses] total=%d skipped=%d updated=%d would_update=%d created=%d would_create=%d\n",
		report.Total, report.Skipped, report.Updated, report.WouldUpdate, report.Created, report.WouldCreate)

	if !*apply && (report.WouldUpdate > 0 || report.WouldCreate > 0) {
		fmt.Println("[backfill_bulk_licenses] dry-run complete — pass --apply to commit")
		os.Exit(0)
	}
}
