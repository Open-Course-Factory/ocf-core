package cron

import (
	"time"

	"soli/formations/src/scenarios/services"
	terminalServices "soli/formations/src/terminalTrainer/services"

	"gorm.io/gorm"
)

// StartLocalMachineSweepJob ends the local machines whose tab has been away
// for the grace delay or whose time is up, and closes the tab of a run ended
// on the server. Runs every minute.
func StartLocalMachineSweepJob(db *gorm.DB) {
	end := terminalServices.NewTerminalTrainerService(db).DeleteSession
	startJob("Local machine sweep", time.Minute, func() {
		services.SweepLocalMachines(db, services.Agents(), services.LocalMachineGrace, end)
	})
}
