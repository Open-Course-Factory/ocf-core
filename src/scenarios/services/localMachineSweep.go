package services

import (
	"time"

	terminalModels "soli/formations/src/terminalTrainer/models"
	"soli/formations/src/utils"

	"gorm.io/gorm"
)

// LocalMachineGrace is how long a local run's tab may stay away before its
// terminal ends (plan rule 9), so the learner may start another. Reopening the run
// after that rebuilds it.
const LocalMachineGrace = 15 * time.Minute

// SweepLocalMachines does for machines in learners' browsers what tt-backend
// does for containers: it ends a terminal whose tab has been away for grace,
// or whose time is up, and closes the tab of a terminal ended on the server
// (a trainer's stop, a block, an erasure), so its machine stops answering.
// end ends a terminal the way the platform does.
func SweepLocalMachines(db *gorm.DB, agents *LocalAgents, grace time.Duration, end func(terminalID string) error) {
	var open []terminalModels.Terminal
	if err := db.Where("session_id LIKE ? AND state = ?", terminalModels.LocalSessionIDPrefix+"%", terminalModels.StateRunning).
		Find(&open).Error; err != nil {
		utils.Error("local machine sweep: listing terminals: %v", err)
		return
	}
	live := map[string]bool{}
	for i := range open {
		terminal := &open[i]
		away := agents.awaySince(terminal.SessionID)
		gone := !away.IsZero() && time.Since(away) >= grace
		if terminal.IsLive() && !gone {
			live[terminal.SessionID] = true
			continue
		}
		if err := end(terminal.SessionID); err != nil {
			utils.Error("local machine sweep: ending %s: %v", terminal.SessionID, err)
			live[terminal.SessionID] = true // tried again next sweep
		}
	}
	for _, terminalID := range agents.connectedTerminals() {
		if !live[terminalID] {
			agents.Close(terminalID, AgentCloseEnded, "this run's machine has ended")
		}
	}
	agents.forgetAllBut(live)
}
