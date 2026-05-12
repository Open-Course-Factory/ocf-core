// Command backfill_quota translates legacy count-based plan quotas into
// the new budget model (MaxCPU / MaxMemoryMB) without breaking existing
// readers. It is safe to run multiple times: plans already on the budget
// model are skipped.
//
// Usage:
//
//	go run ./cmd/backfill_quota          # dry-run (default)
//	go run ./cmd/backfill_quota --apply  # commit changes
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
		log.Fatalf("backfill_quota: database connection not initialised")
	}

	mode := "DRY-RUN"
	if *apply {
		mode = "APPLY"
	}
	fmt.Printf("[backfill_quota] mode=%s\n", mode)

	report, err := backfill.Run(sqldb.DB, backfill.Options{Apply: *apply})
	if err != nil {
		log.Fatalf("backfill_quota: %v", err)
	}

	for _, p := range report.Plans {
		fmt.Printf("[plan: %s id=%s] %s\n", p.Name, p.PlanID, p.Reason)
	}
	fmt.Printf("[backfill_quota] total=%d updated=%d would_update=%d skipped=%d\n",
		report.Total, report.Updated, report.WouldUpdate, report.Skipped)

	if !*apply && report.WouldUpdate > 0 {
		fmt.Println("[backfill_quota] dry-run complete — pass --apply to commit")
		os.Exit(0)
	}
}
