package services

import (
	"testing"
	"time"

	terminalModels "soli/formations/src/terminalTrainer/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// What the hub knows of a terminal ended elsewhere (an abandon, a trainer's
// stop, a lost machine) is dropped by the next sweep; a live terminal's is kept.
func TestSweepLocalMachines_ForgetsTerminalsNoLongerLive(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&terminalModels.Terminal{}))
	live := terminalModels.LocalSessionIDPrefix + "live"
	require.NoError(t, db.Create(&terminalModels.Terminal{SessionID: live, UserID: "u", State: terminalModels.StateRunning,
		ExpiresAt: time.Now().Add(time.Hour)}).Error)
	ended := terminalModels.LocalSessionIDPrefix + "ended"

	hub := NewLocalAgents()
	for _, id := range []string{live, ended} {
		hub.boots[id] = "boot"
		hub.away[id] = time.Now()
	}
	waiting := make(chan struct{})
	hub.arrivals[ended] = waiting

	SweepLocalMachines(db, hub, time.Hour, func(string) error { return nil })

	assert.NotContains(t, hub.boots, ended)
	assert.NotContains(t, hub.away, ended)
	assert.NotContains(t, hub.arrivals, ended)
	assert.Contains(t, hub.boots, live)
	assert.Contains(t, hub.away, live)
	select {
	case <-waiting:
	default:
		t.Fatal("a wait for the ended terminal's tab is not released")
	}
}
