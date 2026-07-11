package services

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"soli/formations/src/auth/mocks"
)

// White-box test (package services) because reconcileEntityPolicy is
// unexported and there is no exported entry point that lets us drive it
// against a controllable enforcer without standing up the full entity
// registration + DB stack. Testing the function directly is the cleanest
// reachable approach.
//
// This is the entity-side twin of tests/authorization/reconcile_policy_test.go
// (which guards the exported access.ReconcilePolicy against the same #297 wipe).
// The default mocks.MockEnforcer is stateless — GetFilteredPolicy always
// returns [] — so it cannot reproduce the wipe. We wire it to a small
// stateful in-memory policy store that behaves like Casbin's store.

// entityPolicyStore is a stateful in-memory simulation of Casbin's policy
// store: it actually remembers added rows and filters / removes them the way
// Casbin would, so reconcileEntityPolicy's GetFilteredPolicy / RemoveFilteredPolicy
// / AddPolicy interactions are exercised realistically.
type entityPolicyStore struct {
	mu       sync.Mutex
	policies [][]string // each row is [role, path, method]
}

func newEntityPolicyStore() *entityPolicyStore {
	return &entityPolicyStore{policies: make([][]string, 0)}
}

func (s *entityPolicyStore) addPolicy(params ...any) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row := make([]string, len(params))
	for i, p := range params {
		v, _ := p.(string)
		row[i] = v
	}
	// Casbin's AddPolicy is idempotent on exact duplicates.
	for _, existing := range s.policies {
		if entityRowsEqual(existing, row) {
			return false, nil
		}
	}
	s.policies = append(s.policies, row)
	return true, nil
}

func entityRowsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// getFilteredPolicy mimics Casbin: a row matches when every non-empty filter
// value equals the column at fieldIndex+i. Empty strings are wildcards.
func (s *entityPolicyStore) getFilteredPolicy(fieldIndex int, fieldValues ...string) ([][]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([][]string, 0)
	for _, p := range s.policies {
		if entityRowMatches(p, fieldIndex, fieldValues) {
			row := make([]string, len(p))
			copy(row, p)
			out = append(out, row)
		}
	}
	return out, nil
}

func (s *entityPolicyStore) removeFilteredPolicy(fieldIndex int, fieldValues ...string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := make([][]string, 0, len(s.policies))
	removed := false
	for _, p := range s.policies {
		if entityRowMatches(p, fieldIndex, fieldValues) {
			removed = true
			continue
		}
		kept = append(kept, p)
	}
	s.policies = kept
	return removed, nil
}

func entityRowMatches(p []string, fieldIndex int, fieldValues []string) bool {
	for i, v := range fieldValues {
		if v == "" {
			continue
		}
		idx := fieldIndex + i
		if idx >= len(p) || p[idx] != v {
			return false
		}
	}
	return true
}

func (s *entityPolicyStore) snapshot() [][]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([][]string, len(s.policies))
	for i, p := range s.policies {
		row := make([]string, len(p))
		copy(row, p)
		out[i] = row
	}
	return out
}

// newStatefulEntityEnforcer wires a fresh MockEnforcer to an entityPolicyStore
// so reconcileEntityPolicy operates against a realistic in-memory backend.
func newStatefulEntityEnforcer() (*mocks.MockEnforcer, *entityPolicyStore) {
	store := newEntityPolicyStore()
	m := mocks.NewMockEnforcer()
	m.AddPolicyFunc = store.addPolicy
	m.GetFilteredPolicyFunc = store.getFilteredPolicy
	m.RemoveFilteredPolicyFunc = store.removeFilteredPolicy
	return m, store
}

func entityContainsTriple(snapshot [][]string, role, path, method string) bool {
	for _, row := range snapshot {
		if len(row) >= 3 && row[0] == role && row[1] == path && row[2] == method {
			return true
		}
	}
	return false
}

func entityCountMatching(snapshot [][]string, role, path string) int {
	n := 0
	for _, row := range snapshot {
		if len(row) >= 2 && row[0] == role && row[1] == path {
			n++
		}
	}
	return n
}

// TestReconcileEntityPolicy_DifferentMethodOnSamePath_DoesNotWipeSibling is the
// regression guard for the #297 wipe pattern on the ENTITY reconciler.
//
// Scenario: a policy row already exists for (member, /api/v1/things, GET) — for
// example registered by a custom route in a module's permissions.go — and the
// generic entity registration then reconciles the SAME path for POST.
//
// Correct behavior (unified with access.ReconcilePolicy — MR J / C1): adding
// POST must NOT remove the pre-existing GET. Both methods must coexist, because
// each is a legitimate, independently-registered grant on that (role, path).
//
// Current behavior (RED): reconcileEntityPolicy calls
// RemoveFilteredPolicy(0, role, path) — with no method filter — which deletes
// EVERY row for (member, /api/v1/things), wiping the GET grant, then re-adds
// only POST. GET is silently lost.
func TestReconcileEntityPolicy_DifferentMethodOnSamePath_DoesNotWipeSibling(t *testing.T) {
	enforcer, store := newStatefulEntityEnforcer()

	role := "member"
	path := "/api/v1/things"

	// Pre-seed an existing, legitimate grant on the path.
	_, _ = store.addPolicy(role, path, "GET")

	// Reconcile a DIFFERENT method on the SAME (role, path). GET != POST, so the
	// current code skips the early-return and reaches the remove branch.
	reconcileEntityPolicy(enforcer, role, path, "POST")

	snapshot := store.snapshot()

	assert.True(t,
		entityContainsTriple(snapshot, role, path, "GET"),
		"pre-existing GET grant must survive when POST is reconciled on the same "+
			"(role, path); the #297 wipe deletes it. snapshot=%v", snapshot)
	assert.True(t,
		entityContainsTriple(snapshot, role, path, "POST"),
		"newly reconciled POST grant must be present. snapshot=%v", snapshot)
	assert.Equal(t, 2,
		entityCountMatching(snapshot, role, path),
		"exactly 2 rows expected for (%s, %s) — GET and POST must coexist; "+
			"snapshot=%v", role, path, snapshot)
}

// TestReconcileEntityPolicy_ExactTripleAlreadyPresent_IsNoOp verifies the happy
// path is unaffected: reconciling a method that already exists does not thrash
// (no remove, no duplicate row). This guards the fix from over-correcting into
// a "never touch anything" no-op that would break legitimate updates, and
// confirms idempotency.
func TestReconcileEntityPolicy_ExactTripleAlreadyPresent_IsNoOp(t *testing.T) {
	enforcer, store := newStatefulEntityEnforcer()

	role := "member"
	path := "/api/v1/things"

	_, _ = store.addPolicy(role, path, "GET")

	reconcileEntityPolicy(enforcer, role, path, "GET")

	snapshot := store.snapshot()

	assert.Equal(t, 1, entityCountMatching(snapshot, role, path),
		"reconciling an already-present triple must not duplicate it; snapshot=%v", snapshot)
	assert.True(t, entityContainsTriple(snapshot, role, path, "GET"),
		"the (member, path, GET) row must still be present; snapshot=%v", snapshot)
	assert.Equal(t, 0, len(enforcer.RemoveFilteredPolicyCalls),
		"no RemoveFilteredPolicy call expected when the exact triple already exists; "+
			"calls=%v", enforcer.RemoveFilteredPolicyCalls)
}
