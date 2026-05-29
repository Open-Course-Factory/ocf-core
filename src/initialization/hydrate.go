package initialization

import (
	"context"
	"time"

	"gorm.io/gorm"

	"soli/formations/src/payment/catalog"
	terminalServices "soli/formations/src/terminalTrainer/services"
	"soli/formations/src/utils"
)

// HydrateSizeCatalog fetches the authoritative size catalog from
// tt-backend at startup and applies it to the in-memory budget catalog.
// On fetch failure the hardcoded fallback in payment/catalog remains
// active so ocf-core can still serve budget decisions while tt-backend
// is unreachable. Per-key disagreements between live and fallback are
// logged as warnings — that's the early-warning system for silent
// drift between the two services.
func HydrateSizeCatalog(db *gorm.DB) {
	svc := terminalServices.NewTerminalTrainerService(db)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	sizes, err := svc.FetchRawSizes(ctx)
	if err != nil {
		utils.Warn("size catalog hydration: tt-backend unreachable, using hardcoded fallback: %v", err)
		return
	}
	if len(sizes) == 0 {
		utils.Warn("size catalog hydration: tt-backend returned an empty catalog, using hardcoded fallback")
		return
	}

	sources := make([]catalog.SourceSize, 0, len(sizes))
	for _, s := range sizes {
		sources = append(sources, catalog.SourceSize{
			Key:          s.Key,
			CPU:          s.CPU,
			CPUAllowance: s.CPUAllowance,
			Memory:       s.Memory,
			SortOrder:    s.SortOrder,
		})
	}
	drifts := catalog.Hydrate(sources)
	for _, d := range drifts {
		utils.Warn("size catalog drift on %q: fallback=%+v live=%+v (%s)", d.Key, d.Fallback, d.Live, d.Reason)
	}
	utils.Info("size catalog hydrated from tt-backend (%d sizes, %d drift entries)", len(sources), len(drifts))
}
