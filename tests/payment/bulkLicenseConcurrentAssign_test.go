package payment_tests

import (
	"fmt"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"

	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupBulkLicensePostgresDB connects to a real PostgreSQL for the concurrency
// test. SQLite serializes writers (single-writer lock) and therefore
// structurally masks the batch-row race — the ledger corruption below only
// manifests against a database that lets two transactions read the batch row
// concurrently. Skips (does not fail) when Postgres is unreachable so the
// package still builds and runs everywhere.
func setupBulkLicensePostgresDB(t *testing.T) *gorm.DB {
	t.Helper()

	env := func(key, def string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		return def
	}
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s connect_timeout=5",
		env("POSTGRES_HOST", "localhost"),
		env("POSTGRES_PORT", "5432"),
		env("POSTGRES_USER", "postgres"),
		env("POSTGRES_PASSWORD", "postgres"),
		env("POSTGRES_DB", "ocf_test"),
		env("POSTGRES_SSLMODE", "disable"),
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Skipf("PostgreSQL not available: %v. Set POSTGRES_HOST to run this test.", err)
		return nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Skipf("failed to get DB handle: %v", err)
		return nil
	}
	if err := sqlDB.Ping(); err != nil {
		t.Skipf("PostgreSQL ping failed: %v", err)
		return nil
	}

	// Fresh, isolated schema for the three tables this test touches. Drop first
	// so a leftover schema from a prior run or another suite can't interfere.
	tables := []any{&models.UserSubscription{}, &models.SubscriptionBatch{}, &models.SubscriptionPlan{}}
	_ = db.Migrator().DropTable(tables...)
	require.NoError(t, db.AutoMigrate(&models.SubscriptionPlan{}, &models.SubscriptionBatch{}, &models.UserSubscription{}))

	t.Cleanup(func() {
		_ = db.Migrator().DropTable(tables...)
		sqlDB.Close()
	})
	return db
}

// TestBulkLicense_ConcurrentAssignLastSeat_LedgerStaysConsistent races several
// AssignLicense calls for the last free seat of a batch and asserts the batch
// ledger cannot overshoot the paid quantity.
//
// WHY this is a real bug: AssignLicense tries to serialize concurrent assigns by
// locking the batch row with tx.Set("gorm:query_option", "FOR UPDATE"). That is
// a GORM v1 mechanism; this project runs GORM v2 (gorm.io/gorm), where it is a
// SILENT NO-OP — GORM v2 requires clause.Locking{Strength:"UPDATE"} instead. So
// the batch row is read UNLOCKED. Two concurrent transactions both read
// AssignedQuantity < TotalQuantity, both pass the availability check, and both
// run `assigned_quantity = assigned_quantity + 1`, pushing the counter past
// TotalQuantity and out of sync with the real number of assigned rows.
func TestBulkLicense_ConcurrentAssignLastSeat_LedgerStaysConsistent(t *testing.T) {
	if runtime.GOMAXPROCS(0) < 2 {
		runtime.GOMAXPROCS(2)
	}
	db := setupBulkLicensePostgresDB(t)

	// Give each racing goroutine a real, already-warm connection so the race is
	// decided inside the transactions, not on connection setup.
	const concurrency = 8
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(concurrency + 2)
	sqlDB.SetMaxIdleConns(concurrency + 2)

	svc := services.NewBulkLicenseService(db)
	purchaserID := "purchaser-concurrent-lastseat"

	plan := &models.SubscriptionPlan{
		BaseModel:   entityManagementModels.BaseModel{ID: uuid.New()},
		Name:        "Concurrent Assign Plan",
		PriceAmount: 1000,
		Currency:    "eur",
		IsActive:    true,
	}
	require.NoError(t, db.Create(plan).Error)

	// One free seat in the LEDGER (TotalQuantity - AssignedQuantity == 1), but
	// deliberately MORE than one unassigned license row available. The extra
	// unassigned rows strip away the incidental "grab an unassigned row" guard,
	// leaving the batch-row availability check (AssignedQuantity < TotalQuantity)
	// as the SOLE gate against over-assignment — which is exactly the gate the
	// no-op FOR UPDATE fails to protect. This mirrors the task spec: N total,
	// N-1 assigned, 2+ unassigned rows.
	const totalQuantity = 2
	const assignedBase = 1
	const unassignedRows = concurrency

	// seedBatch creates one fresh batch (one free seat) with `unassignedRows`
	// spare unassigned license rows and returns it.
	seedBatch := func(round int) *models.SubscriptionBatch {
		batch := &models.SubscriptionBatch{
			BaseModel:            entityManagementModels.BaseModel{ID: uuid.New()},
			PurchaserUserID:      purchaserID,
			SubscriptionPlanID:   plan.ID,
			StripeSubscriptionID: fmt.Sprintf("sub_conc_%d_%s", round, uuid.New().String()[:8]),
			TotalQuantity:        totalQuantity,
			AssignedQuantity:     assignedBase,
			Status:               "active",
			CurrentPeriodStart:   time.Now().Add(-24 * time.Hour),
			CurrentPeriodEnd:     time.Now().Add(30 * 24 * time.Hour),
		}
		require.NoError(t, db.Create(batch).Error)

		seedLicense := func(assigned bool) {
			stripeSubID := batch.StripeSubscriptionID + "-lic-" + uuid.New().String()[:8]
			lic := models.UserSubscription{
				BaseModel:            entityManagementModels.BaseModel{ID: uuid.New()},
				PurchaserUserID:      &purchaserID,
				SubscriptionBatchID:  &batch.ID,
				SubscriptionPlanID:   plan.ID,
				StripeSubscriptionID: &stripeSubID,
				Status:               "unassigned",
				CurrentPeriodStart:   batch.CurrentPeriodStart,
				CurrentPeriodEnd:     batch.CurrentPeriodEnd,
			}
			if assigned {
				lic.UserID = "base-user-" + uuid.New().String()[:8]
				lic.Status = "active"
				lic.SubscriptionType = "assigned"
			}
			require.NoError(t, db.Create(&lic).Error)
		}
		for i := 0; i < assignedBase; i++ {
			seedLicense(true)
		}
		for i := 0; i < unassignedRows; i++ {
			seedLicense(false)
		}
		return batch
	}

	// The corruption window (a racer's batch-check must land before the winner
	// commits) is narrow, so replay the race over many fresh batches and fail on
	// the FIRST ledger corruption. A correct (row-locking) implementation keeps
	// every round consistent; the buggy no-op lock corrupts within a few rounds.
	const rounds = 40
	for round := 0; round < rounds; round++ {
		batch := seedBatch(round)

		start := make(chan struct{})
		var wg sync.WaitGroup
		successes := make([]bool, concurrency)
		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				targetUser := fmt.Sprintf("race-%d-%d-%s", round, idx, uuid.New().String()[:8])
				<-start
				if _, err := svc.AssignLicense(batch.ID, purchaserID, targetUser); err == nil {
					successes[idx] = true
				}
			}(i)
		}
		close(start)
		wg.Wait()

		successCount := 0
		for _, ok := range successes {
			if ok {
				successCount++
			}
		}

		// Observable DB state after this round's race.
		var finalBatch models.SubscriptionBatch
		require.NoError(t, db.First(&finalBatch, batch.ID).Error)

		var realAssigned int64
		require.NoError(t, db.Model(&models.UserSubscription{}).
			Where("subscription_batch_id = ? AND status = ? AND user_id <> ''", batch.ID, "active").
			Count(&realAssigned).Error)

		// The paid cap must never be exceeded, in the ledger or in reality.
		require.LessOrEqualf(t, finalBatch.AssignedQuantity, totalQuantity,
			"round %d: ledger assigned_quantity=%d overshot total_quantity=%d (batch row read unlocked; FOR UPDATE is a GORM-v2 no-op)",
			round, finalBatch.AssignedQuantity, totalQuantity)
		require.LessOrEqualf(t, realAssigned, int64(totalQuantity),
			"round %d: real assigned row count=%d overshot total_quantity=%d", round, realAssigned, totalQuantity)

		// The ledger must match the real number of assigned rows.
		require.Equalf(t, int64(finalBatch.AssignedQuantity), realAssigned,
			"round %d: ledger assigned_quantity=%d diverged from real assigned rows=%d", round, finalBatch.AssignedQuantity, realAssigned)

		// Exactly one racer should have won the single free seat.
		require.Equalf(t, assignedBase+1, int(realAssigned),
			"round %d: expected exactly one new assignment, got %d assigned rows", round, realAssigned)
		require.Equalf(t, 1, successCount,
			"round %d: expected exactly one AssignLicense to succeed, got %d", round, successCount)
	}
}
