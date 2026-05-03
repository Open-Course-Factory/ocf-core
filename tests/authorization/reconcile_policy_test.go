package authorization_tests

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/auth/access"
	"soli/formations/src/auth/mocks"
)

// policyStore is a stateful in-memory simulation of Casbin's policy store
// used to drive ReconcilePolicy through realistic GetFilteredPolicy /
// AddPolicy / RemoveFilteredPolicy interactions. The default MockEnforcer
// in src/auth/mocks is stateless — it cannot reproduce the wipe bug
// because GetFilteredPolicy always returns []. We need a store that
// remembers what was added, what was filtered out, and what remains.
//
// Pattern adapted from tests/policy_audit/policy_audit_test.go.
type policyStore struct {
	mu       sync.Mutex
	policies [][]string // each row is [role, path, method]
}

func newPolicyStore() *policyStore {
	return &policyStore{policies: make([][]string, 0)}
}

func (s *policyStore) addPolicy(params ...any) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row := make([]string, len(params))
	for i, p := range params {
		v, _ := p.(string)
		row[i] = v
	}
	// Casbin's AddPolicy is idempotent on exact duplicates.
	for _, existing := range s.policies {
		if rowsEqual(existing, row) {
			return false, nil
		}
	}
	s.policies = append(s.policies, row)
	return true, nil
}

func rowsEqual(a, b []string) bool {
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

// getFilteredPolicy mimics Casbin: rows match if every non-empty filter
// value matches the column at fieldIndex+i. Empty strings are wildcards.
func (s *policyStore) getFilteredPolicy(fieldIndex int, fieldValues ...string) ([][]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([][]string, 0)
	for _, p := range s.policies {
		match := true
		for i, v := range fieldValues {
			if v == "" {
				continue
			}
			idx := fieldIndex + i
			if idx >= len(p) || p[idx] != v {
				match = false
				break
			}
		}
		if match {
			row := make([]string, len(p))
			copy(row, p)
			out = append(out, row)
		}
	}
	return out, nil
}

func (s *policyStore) removeFilteredPolicy(fieldIndex int, fieldValues ...string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := make([][]string, 0, len(s.policies))
	removed := false
	for _, p := range s.policies {
		match := true
		for i, v := range fieldValues {
			if v == "" {
				continue
			}
			idx := fieldIndex + i
			if idx >= len(p) || p[idx] != v {
				match = false
				break
			}
		}
		if match {
			removed = true
			continue
		}
		kept = append(kept, p)
	}
	s.policies = kept
	return removed, nil
}

func (s *policyStore) snapshot() [][]string {
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

// newStatefulEnforcer wires a fresh MockEnforcer to a policyStore so the
// real ReconcilePolicy operates against a realistic in-memory backend.
func newStatefulEnforcer() (*mocks.MockEnforcer, *policyStore) {
	store := newPolicyStore()
	m := mocks.NewMockEnforcer()
	m.AddPolicyFunc = store.addPolicy
	m.GetFilteredPolicyFunc = store.getFilteredPolicy
	m.RemoveFilteredPolicyFunc = store.removeFilteredPolicy
	return m, store
}

// containsTriple reports whether the given triple appears in the snapshot.
func containsTriple(snapshot [][]string, role, path, method string) bool {
	for _, row := range snapshot {
		if len(row) >= 3 && row[0] == role && row[1] == path && row[2] == method {
			return true
		}
	}
	return false
}

// countMatching counts rows matching the given (role, path) pair.
func countMatching(snapshot [][]string, role, path string) int {
	n := 0
	for _, row := range snapshot {
		if len(row) >= 2 && row[0] == role && row[1] == path {
			n++
		}
	}
	return n
}

// ----------------------------------------------------------------------------
// Regression: the wipe bug
// ----------------------------------------------------------------------------

// TestReconcilePolicy_PreservesSiblingMethodsOnSamePath is the primary
// regression test for the wipe bug confirmed in production (issue #297).
//
// When a module's permissions.go registers two HTTP methods on the same
// path during startup (e.g. GET then POST), the second call must NOT wipe
// the first one. Both triples must coexist in the resulting policy state.
//
// Before the fix this test fails: only the LAST method registered survives
// because RemoveFilteredPolicy(0, role, path) deletes every row for that
// (role, path) pair regardless of method.
func TestReconcilePolicy_PreservesSiblingMethodsOnSamePath(t *testing.T) {
	enforcer, store := newStatefulEnforcer()

	access.ReconcilePolicy(enforcer, "member", "/api/v1/test/path", "GET")
	access.ReconcilePolicy(enforcer, "member", "/api/v1/test/path", "POST")

	snapshot := store.snapshot()

	assert.True(t,
		containsTriple(snapshot, "member", "/api/v1/test/path", "GET"),
		"GET triple must survive after POST is registered on the same path; "+
			"the wipe bug deletes it. snapshot=%v", snapshot)
	assert.True(t,
		containsTriple(snapshot, "member", "/api/v1/test/path", "POST"),
		"POST triple must be present. snapshot=%v", snapshot)
	assert.Equal(t, 2,
		countMatching(snapshot, "member", "/api/v1/test/path"),
		"exactly 2 policy rows expected for (member, /api/v1/test/path); "+
			"snapshot=%v", snapshot)
}

// TestReconcilePolicy_PreservesAllFourCRUDMethods verifies the bug is
// also gone for the worst-case scenario: a module registering all four
// CRUD methods on the same path. Before the fix only DELETE would survive.
func TestReconcilePolicy_PreservesAllFourCRUDMethods(t *testing.T) {
	enforcer, store := newStatefulEnforcer()

	path := "/api/v1/things/:id"
	for _, method := range []string{"GET", "POST", "PATCH", "DELETE"} {
		access.ReconcilePolicy(enforcer, "member", path, method)
	}

	snapshot := store.snapshot()

	for _, method := range []string{"GET", "POST", "PATCH", "DELETE"} {
		assert.True(t,
			containsTriple(snapshot, "member", path, method),
			"method %s must be present after all four CRUD methods are registered; snapshot=%v",
			method, snapshot)
	}
	assert.Equal(t, 4, countMatching(snapshot, "member", path),
		"all four CRUD rows must coexist; snapshot=%v", snapshot)
}

// ----------------------------------------------------------------------------
// Idempotency
// ----------------------------------------------------------------------------

// TestReconcilePolicy_IdempotentOnSameTriple verifies that calling
// ReconcilePolicy twice with the exact same (role, path, method) triple
// produces a single row in the store (no duplicate, no wipe-and-readd
// thrash).
func TestReconcilePolicy_IdempotentOnSameTriple(t *testing.T) {
	enforcer, store := newStatefulEnforcer()

	access.ReconcilePolicy(enforcer, "member", "/api/v1/test/path", "GET")
	access.ReconcilePolicy(enforcer, "member", "/api/v1/test/path", "GET")

	snapshot := store.snapshot()

	assert.Equal(t, 1, countMatching(snapshot, "member", "/api/v1/test/path"),
		"calling ReconcilePolicy twice with the same triple must yield a single row; snapshot=%v",
		snapshot)
	assert.True(t,
		containsTriple(snapshot, "member", "/api/v1/test/path", "GET"),
		"the (member, path, GET) row must be present; snapshot=%v", snapshot)
}

// TestReconcilePolicy_NoOpWhenTripleAlreadyExists verifies that when the
// exact (role, path, method) triple already exists in the store, no new
// AddPolicy call is made. This exercises the early-return branch and
// guards against reintroducing a "remove then re-add" thrash.
func TestReconcilePolicy_NoOpWhenTripleAlreadyExists(t *testing.T) {
	enforcer, store := newStatefulEnforcer()

	// Seed one row.
	access.ReconcilePolicy(enforcer, "member", "/api/v1/test/path", "GET")
	require.Equal(t, 1, len(enforcer.AddPolicyCalls),
		"sanity: first reconcile should call AddPolicy once")

	addCallsBefore := len(enforcer.AddPolicyCalls)
	removeCallsBefore := len(enforcer.RemoveFilteredPolicyCalls)

	// Second reconcile of the exact same triple — must be a pure no-op.
	access.ReconcilePolicy(enforcer, "member", "/api/v1/test/path", "GET")

	assert.Equal(t, addCallsBefore, len(enforcer.AddPolicyCalls),
		"AddPolicy must not be called again when the triple already exists")
	assert.Equal(t, removeCallsBefore, len(enforcer.RemoveFilteredPolicyCalls),
		"RemoveFilteredPolicy must not be called when the triple already exists")
	assert.Equal(t, 1, countMatching(store.snapshot(), "member", "/api/v1/test/path"),
		"store must still contain exactly one row")
}

// ----------------------------------------------------------------------------
// Role isolation
// ----------------------------------------------------------------------------

// TestReconcilePolicy_DifferentRolesSamePath verifies that registering the
// same (path, method) under two different roles produces two rows.
// The wipe bug is scoped to (role, path) so this case worked before too —
// this test guards against a regression in the fix.
func TestReconcilePolicy_DifferentRolesSamePath(t *testing.T) {
	enforcer, store := newStatefulEnforcer()

	access.ReconcilePolicy(enforcer, "member", "/api/v1/shared", "GET")
	access.ReconcilePolicy(enforcer, "administrator", "/api/v1/shared", "GET")

	snapshot := store.snapshot()

	assert.True(t, containsTriple(snapshot, "member", "/api/v1/shared", "GET"),
		"member triple must survive; snapshot=%v", snapshot)
	assert.True(t, containsTriple(snapshot, "administrator", "/api/v1/shared", "GET"),
		"administrator triple must be present; snapshot=%v", snapshot)
	assert.Equal(t, 2, len(snapshot),
		"both role rows must coexist; snapshot=%v", snapshot)
}
