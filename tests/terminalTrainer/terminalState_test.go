package terminalTrainer_tests

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"soli/formations/src/terminalTrainer/models"
)

// TestTerminalStateConstants_PinnedWireFormat pins the underlying string value
// of every TerminalState constant. These strings are the GORM column + JSON
// field format and are consumed by the ocf-front TerminalSession union type
// (src/types/terminal.ts) — renaming any of them is a breaking change to the
// wire contract and must trip this test loudly.
func TestTerminalStateConstants_PinnedWireFormat(t *testing.T) {
	testCases := []struct {
		name     string
		state    models.TerminalState
		expected string
	}{
		{name: "running", state: models.StateRunning, expected: "running"},
		{name: "stopped", state: models.StateStopped, expected: "stopped"},
		{name: "deleted", state: models.StateDeleted, expected: "deleted"},
		{name: "starting", state: models.StateStarting, expected: "starting"},
		{name: "resuming", state: models.StateResuming, expected: "resuming"},
		{name: "hibernating", state: models.StateHibernating, expected: "hibernating"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, string(tc.state))
		})
	}
}

// TestTerminalStateConstants_Distinct guards against a copy-paste regression
// where two constants would share the same underlying string (e.g.
// StateStarting = "starting"; StateResuming = "starting"). Such a bug would
// silently fold two lifecycle states into one and would not be caught by the
// pinned-wire-format test alone.
func TestTerminalStateConstants_Distinct(t *testing.T) {
	all := []models.TerminalState{
		models.StateRunning,
		models.StateStopped,
		models.StateDeleted,
		models.StateStarting,
		models.StateResuming,
		models.StateHibernating,
	}

	seen := make(map[string]models.TerminalState, len(all))
	for _, s := range all {
		v := string(s)
		if prev, ok := seen[v]; ok {
			t.Errorf("duplicate underlying value %q shared by %q and %q", v, prev, s)
			continue
		}
		seen[v] = s
	}

	assert.Equal(t, len(all), len(seen), "every TerminalState constant must have a unique underlying string")
}
