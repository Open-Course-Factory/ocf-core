// Source-order guard for issue #588: a one-shot CLI run (course generation via
// cli.ParseFlags) must exit before main.go starts any background job, or it
// also runs member erasure, class auto-archive, Stripe sync, etc. against the
// configured database. Start-up ordering inside func main cannot be exercised
// directly, so this reads main.go.
package startup_tests

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain_CliParseFlags_RunsBeforeBackgroundJobs(t *testing.T) {
	content, err := os.ReadFile("../../main.go")
	require.NoError(t, err)
	source := string(content)

	cliIdx := strings.Index(source, "cli.ParseFlags(")
	require.Greater(t, cliIdx, 0, "cli.ParseFlags call should exist in main.go")

	for _, start := range []string{
		"cron.Start",                    // cron jobs (member erasure, auto-archive, ...)
		"stripeSyncWorker.Start(",       // Stripe sync worker goroutine
		"impersonationSvc.ExpireStale(", // impersonation expiry ticker goroutine
	} {
		idx := strings.Index(source, start)
		require.Greater(t, idx, 0, "%s should exist in main.go", start)
		assert.Less(t, cliIdx, idx,
			"cli.ParseFlags must run before %s: a CLI run would otherwise start background jobs", start)
	}
}
