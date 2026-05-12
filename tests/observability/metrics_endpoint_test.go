// tests/observability/metrics_endpoint_test.go
//
// TDD tests for the admin observability metrics endpoint (issue #324).
//
// The production package soli/formations/src/observability does not yet
// exist; running this file must FAIL at compile time until the next slice
// implements:
//
//   - observability.Metrics                                   (package-level var)
//   - observability.observabilityMetrics with exported fields:
//         StripeCreateSuccess, StripeCreateFailure,
//         StripeUpdateSuccess, StripeUpdateFailure,
//         StripeArchiveSuccess, StripeArchiveFailure,
//         StripeSyncPanic,
//         ScenarioSetupPanic, ScenarioSetupFailed,
//         TerminalStopOnCleanupFailure
//     (each an atomic.Uint64 — tests reach in and call .Store / .Add)
//   - observabilityRoutes.NewObservabilityHandler() gin.HandlerFunc
//
// JSON contract (issue #324):
//
//   {
//     "stripe": {
//       "create":  { "success": N, "failure": N },
//       "update":  { "success": N, "failure": N },
//       "archive": { "success": N, "failure": N },
//       "panics":  N
//     },
//     "scenarios": {
//       "setup_panics":              N,
//       "setup_failed_transitions":  N,
//       "terminal_stop_failures":    N
//     },
//     "hooks": {
//       "recent_errors": [{ "hook_name", "entity_name", "hook_type", "error", "timestamp" }, ...]
//     }
//   }
//
// Authorization (two-layer):
//   - Casbin: administrator only
//   - Layer 2: AdminOnly
// Tests use the userRoles-from-context pattern (see tests/admin/users_test.go).
package observability_tests

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"soli/formations/src/entityManagement/hooks"
	"soli/formations/src/observability"
	observabilityRoutes "soli/formations/src/observability/routes"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// resetObservabilityMetrics zeros all observability counters so tests don't leak.
// Used in t.Cleanup() in every test that bumps a counter.
func resetObservabilityMetrics() {
	observability.Metrics.StripeCreateSuccess.Store(0)
	observability.Metrics.StripeCreateFailure.Store(0)
	observability.Metrics.StripeUpdateSuccess.Store(0)
	observability.Metrics.StripeUpdateFailure.Store(0)
	observability.Metrics.StripeArchiveSuccess.Store(0)
	observability.Metrics.StripeArchiveFailure.Store(0)
	observability.Metrics.StripeSyncPanic.Store(0)
	observability.Metrics.ScenarioSetupPanic.Store(0)
	observability.Metrics.ScenarioSetupFailed.Store(0)
	observability.Metrics.TerminalStopOnCleanupFailure.Store(0)
}

// setupObservabilityRouter builds a minimal gin router with the auth middleware
// stub + the handler. Mirrors the pattern from tests/admin/users_test.go.
func setupObservabilityRouter(isAdmin bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if isAdmin {
			c.Set("userId", "test-admin")
			c.Set("userRoles", []string{"administrator"})
		} else {
			c.Set("userId", "test-member")
			c.Set("userRoles", []string{"member"})
		}
		c.Next()
	})
	r.GET("/api/v1/admin/observability-metrics", observabilityRoutes.NewObservabilityHandler())
	return r
}

// mustMap fetches a sub-map by key or fails the test loudly.
func mustMap(t *testing.T, m map[string]any, key string) map[string]any {
	t.Helper()
	sub, ok := m[key].(map[string]any)
	if !ok {
		t.Fatalf("expected %q to be a map, got %T (value: %v)", key, m[key], m[key])
	}
	return sub
}

// mustZero asserts that key in m is a JSON number equal to 0.
func mustZero(t *testing.T, m map[string]any, key string) {
	t.Helper()
	v, ok := m[key].(float64)
	if !ok {
		t.Errorf("expected %q to be a number, got %T (value: %v)", key, m[key], m[key])
		return
	}
	if v != 0 {
		t.Errorf("expected %q to be 0, got %v", key, v)
	}
}

// ---------------------------------------------------------------------------
// localFailingHook — a minimal Hook that always returns an error.
//
// We can't import SimpleFailingHook from tests/entityManagement because Go
// test packages don't share symbols across directories. Re-declare locally.
// ---------------------------------------------------------------------------

type localFailingHook struct {
	name       string
	entityName string
	hookTypes  []hooks.HookType
}

func (h *localFailingHook) GetName() string                { return h.name }
func (h *localFailingHook) GetEntityName() string          { return h.entityName }
func (h *localFailingHook) GetHookTypes() []hooks.HookType { return h.hookTypes }
func (h *localFailingHook) IsEnabled() bool                { return true }
func (h *localFailingHook) GetPriority() int               { return 50 }
func (h *localFailingHook) Execute(ctx *hooks.HookContext) error {
	return errors.New("seeded test failure")
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestObservabilityMetrics_RequiresAdmin asserts the Layer-2 AdminOnly check
// rejects non-admin callers and admits administrators.
func TestObservabilityMetrics_RequiresAdmin(t *testing.T) {
	t.Cleanup(resetObservabilityMetrics)
	for _, tc := range []struct {
		name     string
		isAdmin  bool
		wantCode int
	}{
		{"member is rejected", false, http.StatusForbidden},
		{"administrator is allowed", true, http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := setupObservabilityRouter(tc.isAdmin)
			req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/observability-metrics", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tc.wantCode {
				t.Errorf("expected %d, got %d (body: %s)", tc.wantCode, w.Code, w.Body.String())
			}
		})
	}
}

// TestObservabilityMetrics_ZeroSnapshotShape locks the JSON contract on a
// fresh state: every key must exist, every counter must be 0, and
// hooks.recent_errors must be an empty array (not null).
func TestObservabilityMetrics_ZeroSnapshotShape(t *testing.T) {
	t.Cleanup(resetObservabilityMetrics)
	// Also clear any pre-existing hook errors so this test is order-independent.
	hooks.GlobalHookRegistry.ClearErrors()

	r := setupObservabilityRouter(true)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/observability-metrics", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v (raw: %s)", err, w.Body.String())
	}

	// stripe group
	stripe := mustMap(t, resp, "stripe")
	create := mustMap(t, stripe, "create")
	mustZero(t, create, "success")
	mustZero(t, create, "failure")
	update := mustMap(t, stripe, "update")
	mustZero(t, update, "success")
	mustZero(t, update, "failure")
	archive := mustMap(t, stripe, "archive")
	mustZero(t, archive, "success")
	mustZero(t, archive, "failure")
	mustZero(t, stripe, "panics")

	// scenarios group
	scenarios := mustMap(t, resp, "scenarios")
	mustZero(t, scenarios, "setup_panics")
	mustZero(t, scenarios, "setup_failed_transitions")
	mustZero(t, scenarios, "terminal_stop_failures")

	// hooks group — recent_errors must be a JSON array (possibly empty), NOT null.
	hooksGroup := mustMap(t, resp, "hooks")
	recentRaw, present := hooksGroup["recent_errors"]
	if !present {
		t.Fatalf("hooks.recent_errors key is missing; hooks=%v", hooksGroup)
	}
	if recentRaw == nil {
		t.Fatalf("hooks.recent_errors must be [] (not null) on fresh state")
	}
	recent, ok := recentRaw.([]any)
	if !ok {
		t.Errorf("hooks.recent_errors must be a JSON array, got %T", recentRaw)
	} else if len(recent) != 0 {
		t.Errorf("hooks.recent_errors should be empty on fresh state, got %d items: %v", len(recent), recent)
	}
}

// TestObservabilityMetrics_IncrementsStripeCreateFailureCounter directly
// bumps StripeCreateFailure and verifies:
//   - the targeted counter surfaces at stripe.create.failure
//   - sibling counters (success, panics, update.*) stay at 0 (catches conflation)
func TestObservabilityMetrics_IncrementsStripeCreateFailureCounter(t *testing.T) {
	t.Cleanup(resetObservabilityMetrics)

	observability.Metrics.StripeCreateFailure.Add(1)

	r := setupObservabilityRouter(true)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/observability-metrics", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v (raw: %s)", err, w.Body.String())
	}

	stripe := mustMap(t, resp, "stripe")
	create := mustMap(t, stripe, "create")

	failure, ok := create["failure"].(float64)
	if !ok {
		t.Fatalf("stripe.create.failure must be a number, got %T", create["failure"])
	}
	if failure != 1 {
		t.Errorf("stripe.create.failure: expected 1, got %v", failure)
	}

	// Catch counter conflation — sibling counters must stay 0.
	mustZero(t, create, "success")
	mustZero(t, stripe, "panics")
	update := mustMap(t, stripe, "update")
	mustZero(t, update, "success")
	mustZero(t, update, "failure")
	archive := mustMap(t, stripe, "archive")
	mustZero(t, archive, "success")
	mustZero(t, archive, "failure")

	scenarios := mustMap(t, resp, "scenarios")
	mustZero(t, scenarios, "setup_panics")
	mustZero(t, scenarios, "setup_failed_transitions")
	mustZero(t, scenarios, "terminal_stop_failures")
}

// TestObservabilityMetrics_SurfacesHookErrors seeds a hook error through the
// production registry path (register an always-failing hook, execute it as
// AfterCreate — which triggers recordError on the circular buffer), then
// asserts the endpoint surfaces it under hooks.recent_errors with the
// HookError contract fields.
func TestObservabilityMetrics_SurfacesHookErrors(t *testing.T) {
	const hookName = "observability-test-failing-hook"
	const entityName = "ObservabilityTestEntity"

	// Start from a clean error buffer so we can assert deterministically.
	hooks.GlobalHookRegistry.ClearErrors()

	failing := &localFailingHook{
		name:       hookName,
		entityName: entityName,
		hookTypes:  []hooks.HookType{hooks.AfterCreate},
	}
	if err := hooks.GlobalHookRegistry.RegisterHook(failing); err != nil {
		t.Fatalf("registering failing hook: %v", err)
	}
	t.Cleanup(func() {
		_ = hooks.GlobalHookRegistry.UnregisterHook(hookName)
		hooks.GlobalHookRegistry.ClearErrors()
	})

	// Trigger the hook: AfterCreate execution records the error in the buffer.
	_ = hooks.GlobalHookRegistry.ExecuteHooks(&hooks.HookContext{
		HookType:   hooks.AfterCreate,
		EntityName: entityName,
		EntityID:   "test-entity-id",
		NewEntity:  map[string]any{"id": "test-entity-id"},
	})

	// Sanity-check the buffer was actually populated (catches misuse of the
	// fixture: if this returns 0, the endpoint assertions below tell us
	// nothing useful about the production code).
	if errs := hooks.GlobalHookRegistry.GetRecentErrors(0); len(errs) == 0 {
		t.Fatalf("precondition: hook error buffer should contain >=1 error after a failing AfterCreate, got 0")
	}

	// Now hit the endpoint.
	r := setupObservabilityRouter(true)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/observability-metrics", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v (raw: %s)", err, w.Body.String())
	}

	hooksGroup := mustMap(t, resp, "hooks")
	recentRaw, present := hooksGroup["recent_errors"]
	if !present || recentRaw == nil {
		t.Fatalf("hooks.recent_errors missing or null; hooks=%v", hooksGroup)
	}
	recent, ok := recentRaw.([]any)
	if !ok {
		t.Fatalf("hooks.recent_errors must be a JSON array, got %T", recentRaw)
	}
	if len(recent) == 0 {
		t.Fatalf("expected at least 1 recent hook error, got 0 (body: %s)", w.Body.String())
	}

	// Locate the error we seeded (be tolerant to other errors that may exist).
	var seeded map[string]any
	for _, e := range recent {
		entry, ok := e.(map[string]any)
		if !ok {
			continue
		}
		if entry["hook_name"] == hookName {
			seeded = entry
			break
		}
	}
	if seeded == nil {
		t.Fatalf("could not find seeded hook error (hook_name=%q) in recent_errors=%v", hookName, recent)
	}

	// Assert the HookError JSON contract fields are present.
	for _, key := range []string{"hook_name", "entity_name", "hook_type", "error", "timestamp"} {
		if _, ok := seeded[key]; !ok {
			t.Errorf("missing key %q in seeded recent_errors entry: %+v", key, seeded)
		}
	}

	// Sanity: the value of the contract fields we set are echoed back.
	if got := seeded["hook_name"]; got != hookName {
		t.Errorf("hook_name: expected %q, got %v", hookName, got)
	}
	if got := seeded["entity_name"]; got != entityName {
		t.Errorf("entity_name: expected %q, got %v", entityName, got)
	}
	if got := seeded["hook_type"]; got != string(hooks.AfterCreate) {
		t.Errorf("hook_type: expected %q, got %v", string(hooks.AfterCreate), got)
	}
}
