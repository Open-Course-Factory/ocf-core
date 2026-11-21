package terminalTrainer_tests

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	terminalController "soli/formations/src/terminalTrainer/routes"
)

func TestSupervisorResizeFrame_InBounds_IsTtBackendResize(t *testing.T) {
	frame, ok := terminalController.SupervisorResizeFrame(120, 40)
	require.True(t, ok)
	var msg map[string]any
	require.NoError(t, json.Unmarshal(frame, &msg))
	assert.Equal(t, map[string]any{"type": "resize", "cols": 120.0, "rows": 40.0}, msg)
}

func TestSupervisorResizeFrame_OutOfBounds_IsDropped(t *testing.T) {
	for _, size := range [][2]int{{0, 24}, {80, 0}, {-1, 24}, {501, 24}, {80, 501}} {
		_, ok := terminalController.SupervisorResizeFrame(size[0], size[1])
		assert.False(t, ok, "%v", size)
	}
}
