package cron

import (
	"time"

	terminalServices "soli/formations/src/terminalTrainer/services"

	"gorm.io/gorm"
)

// StartExposedPortReconcileJob drops the public URL of a container that
// tt-backend stopped, reaped or re-addressed behind ocf-core's back. One
// minute is the longest a freed bridge address may stay routable.
func StartExposedPortReconcileJob(db *gorm.DB) {
	startJob("Exposed port reconcile", time.Minute, func() { terminalServices.ReconcileExposedPorts(db) })
}
