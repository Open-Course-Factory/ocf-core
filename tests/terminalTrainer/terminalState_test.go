package terminalTrainer_tests

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

// TestTerminalState_JSONRoundTrip_ByteIdentical pins the JSON wire format of
// the State field on the Terminal model. The frontend's TerminalSession union
// type (ocf-front src/types/terminal.ts) discriminates on the raw "state"
// string, so the named-string TerminalState type must marshal byte-identically
// to the underlying literal — no spaces, no quoting differences, no Go-side
// type info leaking into the JSON.
//
// This test is the wire-format guarantee that justifies flipping the Go field
// from string to TerminalState without coordinating with the frontend.
func TestTerminalState_JSONRoundTrip_ByteIdentical(t *testing.T) {
	testCases := []struct {
		name     string
		state    models.TerminalState
		expected string // exact substring that must appear in the JSON
	}{
		{name: "running", state: models.StateRunning, expected: `"state":"running"`},
		{name: "stopped", state: models.StateStopped, expected: `"state":"stopped"`},
		{name: "deleted", state: models.StateDeleted, expected: `"state":"deleted"`},
		{name: "starting", state: models.StateStarting, expected: `"state":"starting"`},
		{name: "resuming", state: models.StateResuming, expected: `"state":"resuming"`},
		{name: "hibernating", state: models.StateHibernating, expected: `"state":"hibernating"`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			terminal := models.Terminal{
				SessionID:         "test-session",
				UserID:            "test-user",
				State:             tc.state,
				ExpiresAt:         time.Now(),
				UserTerminalKeyID: uuid.New(),
			}

			raw, err := json.Marshal(terminal)
			require.NoError(t, err)

			assert.True(t,
				strings.Contains(string(raw), tc.expected),
				"marshaled JSON must contain %q exactly; got %s", tc.expected, string(raw),
			)
		})
	}
}
