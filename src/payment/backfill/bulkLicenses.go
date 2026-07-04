// bulkLicenses.go holds the one-shot data migration that repairs LEGACY bulk
// license batches created before the #367 / !269 fix.
//
// Pre-!269, handleBulkSubscriptionCreated wrote every license row with
// StripeSubscriptionID = the batch's (shared) Stripe subscription id. The
// partial unique index idx_user_stripe_sub_not_null (WHERE
// stripe_subscription_id IS NOT NULL) then let only ONE license row survive per
// batch, so production carries paid batches with total_quantity=N but a single
// license row bearing that shared stripe id — the rest were silently swallowed.
//
// RunBulkLicenses brings such a batch to the post-!269 shape: it NULLs the
// shared stripe_subscription_id on the surviving license row(s) (linkage is via
// SubscriptionBatchID; the batch keeps the stripe id) and backfills the missing
// license rows up to total_quantity so every seat is assignable via
// AssignLicense (which selects `subscription_batch_id = ? AND status =
// 'unassigned'`). It reuses backfill.Options (dry-run is the default), is
// idempotent (healthy batches are skipped), and is invoked by the operator
// command cmd/backfill_bulk_licenses — it is NOT wired into initialization.
//
// BACKFILL is gated on batch liveness (entitlement safety): only a live
// (active / pending_payment) batch may have new assignable seats created.
// A cancelled/expired (or any unknown/dead) batch is NOT paid, so growing it
// with assignable rows would grant entitlements on a dead subscription — those
// batches are still REPAIRED (shared id nulled, harmless) but never grown.
// A pending_payment batch's backfilled seats mirror the creation path and stay
// 'pending_payment' (not assignable until payment activates them). An anomalous
// batch with zero license rows has no StripeCustomerID source, so it is skipped
// and flagged (distinct from "healthy"). The migration only ever repairs and
// adds — it NEVER deletes rows.
//
// No Rollback is provided (unlike quota.go): NULLing the shared stripe id and
// creating rows is not cleanly reversible — the original per-row stripe ids are
// gone and there is no marker distinguishing a backfilled row from an
// organically-created one. A dry-run (the default) is the safety net instead.
package backfill

import (
	"fmt"

	"soli/formations/src/payment/models"

	"gorm.io/gorm"
)

// BulkLicenseBatchReport is the per-batch delta the migration emits to stdout.
type BulkLicenseBatchReport struct {
	BatchID  string
	Status   string // the batch's status (so operators see WHY a batch was gated)
	Repaired int    // license rows whose shared stripe id was (or would be) nulled
	Created  int    // license rows backfilled (or that would be) to reach total_quantity
	Reason   string
}

// BulkLicenseReport aggregates the outcome of a single RunBulkLicenses call.
// Counts are row-granular: Updated/WouldUpdate are legacy license rows repaired,
// Created/WouldCreate are backfilled rows.
type BulkLicenseReport struct {
	Total       int // subscription batches examined
	Skipped     int // healthy batches left untouched
	Updated     int // Apply: legacy license rows repaired (shared stripe id -> NULL)
	WouldUpdate int // dry-run equivalent of Updated
	Created     int // Apply: backfilled license rows created
	WouldCreate int // dry-run equivalent of Created
	Batches     []BulkLicenseBatchReport
}

// RunBulkLicenses repairs legacy bulk license batches. Idempotent: a batch that
// already has total_quantity license rows and none carrying a stripe id is
// skipped. When opts.Apply is false (the default) it only reports what it would
// do without writing anything. When Apply, all mutations run in one transaction
// so a mid-run failure leaves the DB untouched.
func RunBulkLicenses(db *gorm.DB, opts Options) (*BulkLicenseReport, error) {
	var batches []models.SubscriptionBatch
	if err := db.Find(&batches).Error; err != nil {
		return nil, fmt.Errorf("load subscription batches: %w", err)
	}

	report := &BulkLicenseReport{Total: len(batches)}

	apply := func(tx *gorm.DB) error {
		for i := range batches {
			batch := &batches[i]

			var existing int64
			if err := tx.Model(&models.UserSubscription{}).
				Where("subscription_batch_id = ?", batch.ID).
				Count(&existing).Error; err != nil {
				return fmt.Errorf("count licenses for batch %s: %w", batch.ID, err)
			}

			var withStripeID int64
			if err := tx.Model(&models.UserSubscription{}).
				Where("subscription_batch_id = ? AND stripe_subscription_id IS NOT NULL", batch.ID).
				Count(&withStripeID).Error; err != nil {
				return fmt.Errorf("count legacy licenses for batch %s: %w", batch.ID, err)
			}

			toRepair := int(withStripeID)
			live := isLiveBatchStatus(batch.Status)

			// Anomalous batch: no license rows at all. There is no source row to
			// copy StripeCustomerID from, so backfilling would create nil-customer
			// rows. Skip and flag distinctly (NOT "healthy") so an operator notices.
			if existing == 0 {
				report.Skipped++
				report.Batches = append(report.Batches, BulkLicenseBatchReport{
					BatchID: batch.ID.String(),
					Status:  batch.Status,
					Reason:  "anomalous: zero license rows (no StripeCustomerID source) — not backfilled",
				})
				continue
			}

			// Backfill only for live (paid/payable) batches — see package doc.
			// A dead batch is repaired below but never grown.
			toCreate := 0
			if live {
				toCreate = batch.TotalQuantity - int(existing)
				if toCreate < 0 {
					toCreate = 0
				}
			}

			// Nothing to do: no shared id to null and no seats to add. Skip.
			if toRepair == 0 && toCreate == 0 {
				report.Skipped++
				reason := "healthy (fully provisioned, no shared stripe id)"
				if !live {
					reason = fmt.Sprintf("%s batch: nothing to repair (dead — not backfilled)", batch.Status)
				}
				report.Batches = append(report.Batches, BulkLicenseBatchReport{
					BatchID: batch.ID.String(),
					Status:  batch.Status,
					Reason:  reason,
				})
				continue
			}

			reason := fmt.Sprintf("live batch: repair %d row(s), backfill %d to reach total_quantity %d",
				toRepair, toCreate, batch.TotalQuantity)
			if !live {
				reason = fmt.Sprintf("%s batch: repair %d row(s), NO backfill (not live)", batch.Status, toRepair)
			}
			br := BulkLicenseBatchReport{
				BatchID:  batch.ID.String(),
				Status:   batch.Status,
				Repaired: toRepair,
				Created:  toCreate,
				Reason:   reason,
			}

			if !opts.Apply {
				report.WouldUpdate += toRepair
				report.WouldCreate += toCreate
				report.Batches = append(report.Batches, br)
				continue
			}

			// Repair (all batches, live or dead): NULL the shared stripe id on every
			// license row carrying one, preserving all other columns (assignment:
			// user_id / status / subscription_type stay intact).
			if toRepair > 0 {
				res := tx.Model(&models.UserSubscription{}).
					Where("subscription_batch_id = ? AND stripe_subscription_id IS NOT NULL", batch.ID).
					Update("stripe_subscription_id", gorm.Expr("NULL"))
				if res.Error != nil {
					return fmt.Errorf("null shared stripe id for batch %s: %w", batch.ID, res.Error)
				}
				report.Updated += int(res.RowsAffected)
			}

			// Backfill (live batches only): create the missing seats with the
			// post-!269 shape. Mirror the creation path's license status — an active
			// batch's seats are 'unassigned' (assignable); a pending_payment batch's
			// seats stay 'pending_payment' (not assignable until payment activates
			// them). Copy StripeCustomerID from an existing license row (guaranteed
			// present: the zero-license case was handled above). Leave
			// StripeSubscriptionID NULL — linkage is via the batch.
			if toCreate > 0 {
				licenseStatus := "unassigned"
				if batch.Status == "pending_payment" {
					licenseStatus = "pending_payment"
				}

				var customerID *string
				var sample models.UserSubscription
				if err := tx.Where("subscription_batch_id = ?", batch.ID).First(&sample).Error; err == nil {
					customerID = sample.StripeCustomerID
				}

				purchaser := batch.PurchaserUserID
				for j := 0; j < toCreate; j++ {
					license := models.UserSubscription{
						UserID:              "",
						PurchaserUserID:     &purchaser,
						SubscriptionBatchID: &batch.ID,
						SubscriptionPlanID:  batch.SubscriptionPlanID,
						StripeCustomerID:    customerID,
						Status:              licenseStatus,
						CurrentPeriodStart:  batch.CurrentPeriodStart,
						CurrentPeriodEnd:    batch.CurrentPeriodEnd,
					}
					if err := tx.Create(&license).Error; err != nil {
						return fmt.Errorf("backfill license for batch %s: %w", batch.ID, err)
					}
					report.Created++
				}
			}

			report.Batches = append(report.Batches, br)
		}
		return nil
	}

	if opts.Apply {
		if err := db.Transaction(apply); err != nil {
			return nil, err
		}
	} else {
		// Dry-run: run the same read-only pass on the plain handle; it never writes.
		if err := apply(db); err != nil {
			return nil, err
		}
	}

	return report, nil
}

// isLiveBatchStatus reports whether a batch is in a paid/payable state that may
// have new assignable seats backfilled. The batch status field carries values
// active / pending_payment (live) and cancelled / expired (dead). Any unknown
// status is treated as NOT live, so the migration never grows a batch that
// isn't entitled to seats.
func isLiveBatchStatus(status string) bool {
	switch status {
	case "active", "pending_payment":
		return true
	default:
		return false
	}
}
