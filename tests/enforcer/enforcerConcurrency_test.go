package enforcer_tests

import (
	"fmt"
	"sync"
	"testing"

	"soli/formations/src/auth/casdoor"

	"github.com/stretchr/testify/require"
)

// The application enforces on every request while member hooks and role sync
// add and remove policies. A policy change resets casbin's matcher cache, so
// the enforcer must serialise both, or a concurrent reader corrupts the cache:
// in production this ended the process with "sync: unlock of unlocked mutex".
// Run with -race.
func TestEnforcerConcurrentEnforceAndPolicyChangesAreSafe(t *testing.T) {
	enforcer, err := casdoor.NewEnforcer("../../src/configuration/keymatch_model.conf")
	require.NoError(t, err)
	_, err = enforcer.AddPolicy("member", "/api/v1/things/*", "GET")
	require.NoError(t, err)
	_, err = enforcer.AddGroupingPolicy("alice", "member")
	require.NoError(t, err)

	var wg sync.WaitGroup
	for r := 0; r < 8; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 300; i++ {
				_, _ = enforcer.Enforce("alice", "/api/v1/things/1", "GET")
			}
		}()
	}
	for w := 0; w < 2; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 300; i++ {
				user := fmt.Sprintf("user-%d-%d", w, i)
				_, _ = enforcer.AddGroupingPolicy(user, "member")
				_, _ = enforcer.RemoveGroupingPolicy(user, "member")
			}
		}(w)
	}
	wg.Wait()

	allowed, err := enforcer.Enforce("alice", "/api/v1/things/1", "GET")
	require.NoError(t, err)
	require.True(t, allowed)
}
