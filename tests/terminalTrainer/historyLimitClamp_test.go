package terminalTrainer_tests

import (
	"testing"

	"github.com/stretchr/testify/assert"

	terminalServices "soli/formations/src/terminalTrainer/services"
)

// TestClampHistoryLimit_Boundaries verifies the format-aware row-cap helper
// for the command-history endpoint (issue #302). The cap differs by format:
// JSON keeps the 1000-row cap (paginated UI table); CSV uses 100,000 (exports).
// limit <= 0 returns the default of 50 regardless of format.
func TestClampHistoryLimit_Boundaries(t *testing.T) {
	cases := []struct {
		name   string
		limit  int
		format string
		want   int
	}{
		{"csv export below cap returns input", 5000, "csv", 5000},
		{"csv export above cap clamped to 100k", 200000, "csv", 100000},
		{"csv export exactly at cap returns 100k", 100000, "csv", 100000},
		{"json above 1k clamped to 1k", 5000, "json", 1000},
		{"json below cap returns input", 50, "json", 50},
		{"empty format treated as json (1k cap)", 5000, "", 1000},
		{"unknown format treated as json (1k cap)", 5000, "xml", 1000},
		{"zero limit returns default 50", 0, "csv", 50},
		{"negative limit returns default 50", -1, "json", 50},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := terminalServices.ClampHistoryLimit(tc.limit, tc.format)
			assert.Equal(t, tc.want, got)
		})
	}
}
