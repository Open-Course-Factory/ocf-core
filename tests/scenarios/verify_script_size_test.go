package scenarios_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/scenarios/services"
)

func TestRunVerifyScript_SmallScript_RunsInline(t *testing.T) {
	execer := &bgTrackingVerificationService{}

	_, _, err := services.RunVerifyScript(execer, "term-1", "#!/bin/bash\ntest -f /root/done")

	require.NoError(t, err)
	assert.Empty(t, execer.pushFileCalls)
	require.Len(t, execer.execCalls, 1)
	assert.Equal(t, []string{"/bin/bash", "-c", "#!/bin/bash\ntest -f /root/done"}, execer.execCalls[0].command)
	assert.Equal(t, 10, execer.execCalls[0].timeout)
}

// tt-backend refuses an exec argument over 4 KB, so a check that size could
// never run inline: every Vérifier failed with "Failed to verify step".
func TestRunVerifyScript_LargeScript_RunsFromAPushedFile(t *testing.T) {
	execer := &bgTrackingVerificationService{}
	script := "#!/bin/bash\n" + strings.Repeat("test -f /root/done || exit 1\n", 200)
	require.Greater(t, len(script), 4096)

	_, _, err := services.RunVerifyScript(execer, "term-1", script)

	require.NoError(t, err)
	require.Len(t, execer.pushFileCalls, 1)
	push := execer.pushFileCalls[0]
	assert.Equal(t, script, push.content)
	assert.Equal(t, "0700", push.mode)
	assert.True(t, strings.HasPrefix(push.targetPath, "/root/"), "a check must stay unreadable to the learner: %s", push.targetPath)
	require.Len(t, execer.execCalls, 2)
	assert.Equal(t, []string{"/bin/bash", push.targetPath}, execer.execCalls[0].command)
	assert.Equal(t, 10, execer.execCalls[0].timeout)
	assert.Nil(t, execer.execCalls[0].env, "a check never receives the step's flag")
	assert.Equal(t, []string{"rm", "-f", push.targetPath}, execer.execCalls[1].command)
}
