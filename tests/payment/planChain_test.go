// tests/payment/planChain_test.go
//
// RED-phase contract for the declarative plan-gating chain (MR-C).
//
// Every production symbol referenced here does NOT exist yet — these tests
// DEFINE it:
//   - entityManagementInterfaces.PlanRequirement (dependency-free flag struct)
//   - paymentMiddleware.PlanChain(db, req, ts) []gin.HandlerFunc
//
// PlanChain assembles the effective-plan middlewares in a FIXED order from a
// declarative PlanRequirement, so the 8 hand-wired call sites can be replaced
// by a single builder. The order pinned here is exactly the legacy chain:
//   InjectOrgContext → InjectEffectivePlan → RequirePlan → CheckRAMAvailability
//
// The tests drive the REAL middlewares returned by PlanChain through
// httptest + gin + a probe handler that records the context keys each
// middleware sets, so the behaviour (not the wiring) is what is pinned.
//
// Context keys asserted here — discovered from the middleware sources, the dev
// must preserve them:
//   - InjectOrgContext      → ctx "org_context_id"   (string, from ?organization_id or JSON body)
//   - InjectEffectivePlan   → ctx "subscription_plan" (*models.SubscriptionPlan), "effective_plan_result", "planSource"
//   - RequirePlan           → 403 when "effective_plan_result" is absent/typed-nil
//   - CheckRAMAvailability  → reads "subscription_plan"; needs a TerminalTrainerService
package payment_tests

import (
	"net/http"
	"net/http/httptest"
	"testing"

	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	paymentMiddleware "soli/formations/src/payment/middleware"
	terminalServices "soli/formations/src/terminalTrainer/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubPlanChainTerminalService is a non-nil TerminalTrainerService whose methods
// are never invoked by these tests: it embeds the interface (a nil value) purely
// to satisfy the type so PlanChain accepts a non-nil ts when CheckHostRAM is set.
// Uniquely named to avoid colliding with any shared mock in this package.
type stubPlanChainTerminalService struct {
	terminalServices.TerminalTrainerService
}

// planChainProbe records what the plan-chain middlewares left in the gin
// context before the handler ran, plus whether the handler was reached at all.
type planChainProbe struct {
	reached       bool
	orgContextSet bool
	orgContextID  string
	planSet       bool
}

// probeHandler returns a terminal handler that snapshots the context keys the
// plan chain is expected to set, then writes 200. If the chain aborts earlier
// (e.g. RequirePlan 403), reached stays false.
func probeHandler(p *planChainProbe) gin.HandlerFunc {
	return func(c *gin.Context) {
		p.reached = true
		if v, ok := c.Get("org_context_id"); ok {
			p.orgContextSet = true
			p.orgContextID, _ = v.(string)
		}
		if _, ok := c.Get("subscription_plan"); ok {
			p.planSet = true
		}
		c.Status(http.StatusOK)
	}
}

// injectUserID mirrors the auth middleware seam the real chain relies on:
// InjectEffectivePlan reads "userId" from the context. Empty userID sets nothing.
func injectUserID(userID string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if userID != "" {
			c.Set("userId", userID)
			c.Set("userRoles", []string{})
		}
		c.Next()
	}
}

// runPlanChain builds a router of [injectUserID(userID)] + chain + probe and
// serves the given request, returning the recorder and the captured probe.
func runPlanChain(userID string, chain []gin.HandlerFunc, req *http.Request) (*httptest.ResponseRecorder, *planChainProbe) {
	gin.SetMode(gin.TestMode)
	probe := &planChainProbe{}
	engine := gin.New()
	handlers := append([]gin.HandlerFunc{injectUserID(userID)}, chain...)
	handlers = append(handlers, probeHandler(probe))
	engine.Handle(req.Method, "/gate", handlers...)

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec, probe
}

// ---------------------------------------------------------------------------
// 1. Empty requirement → empty chain, probe reached, no plan keys set.
// ---------------------------------------------------------------------------

func TestPlanChain_EmptyRequirement_NoMiddlewaresProbeReached(t *testing.T) {
	db := freshTestDB(t)

	chain := paymentMiddleware.PlanChain(db, entityManagementInterfaces.PlanRequirement{}, nil)
	assert.Len(t, chain, 0, "empty PlanRequirement must produce zero middlewares")

	rec, probe := runPlanChain("", chain, httptest.NewRequest(http.MethodGet, "/gate", nil))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, probe.reached, "handler must be reached with an empty chain")
	assert.False(t, probe.orgContextSet, "no org context must be set")
	assert.False(t, probe.planSet, "no subscription_plan must be set")
}

// ---------------------------------------------------------------------------
// 2. OrgContext only → InjectOrgContext runs, org_context_id populated from the
//    organization_id query param; no plan resolution happens.
// ---------------------------------------------------------------------------

func TestPlanChain_OrgContext_InjectsOrgContextID(t *testing.T) {
	db := freshTestDB(t)

	chain := paymentMiddleware.PlanChain(db, entityManagementInterfaces.PlanRequirement{OrgContext: true}, nil)
	assert.Len(t, chain, 1, "OrgContext-only requirement must produce exactly one middleware")

	req := httptest.NewRequest(http.MethodGet, "/gate?organization_id=org-xyz", nil)
	rec, probe := runPlanChain("", chain, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, probe.reached, "handler must be reached")
	assert.True(t, probe.orgContextSet, "org_context_id must be set from the query param")
	assert.Equal(t, "org-xyz", probe.orgContextID)
	assert.False(t, probe.planSet, "OrgContext alone must not resolve a plan")
}

// ---------------------------------------------------------------------------
// 3. RequirePlan with no resolvable plan → 403, handler NOT reached.
// ---------------------------------------------------------------------------

func TestPlanChain_RequirePlan_NoPlan_Returns403(t *testing.T) {
	db := freshTestDB(t)

	chain := paymentMiddleware.PlanChain(db, entityManagementInterfaces.PlanRequirement{RequirePlan: true}, nil)
	assert.Len(t, chain, 2, "RequirePlan must produce InjectEffectivePlan + RequirePlan")

	req := httptest.NewRequest(http.MethodGet, "/gate", nil)
	rec, probe := runPlanChain("user-without-plan", chain, req)

	assert.Equal(t, http.StatusForbidden, rec.Code, "no resolvable plan must be rejected with 403")
	assert.False(t, probe.reached, "handler must NOT run when RequirePlan aborts")
}

// ---------------------------------------------------------------------------
// 4. RequirePlan with a resolvable personal plan → probe reached,
//    subscription_plan present in context.
// ---------------------------------------------------------------------------

func TestPlanChain_RequirePlan_WithPlan_ProbeReachedPlanInContext(t *testing.T) {
	db := freshTestDB(t)

	userID := "user-with-plan"
	plan := createPlan(t, db, "Solo", 10, 0)
	createUserSubscription(t, db, userID, plan)

	chain := paymentMiddleware.PlanChain(db, entityManagementInterfaces.PlanRequirement{RequirePlan: true}, nil)

	req := httptest.NewRequest(http.MethodGet, "/gate", nil)
	rec, probe := runPlanChain(userID, chain, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, probe.reached, "handler must be reached when a plan resolves")
	assert.True(t, probe.planSet, "subscription_plan must be present in context after InjectEffectivePlan")
}

// ---------------------------------------------------------------------------
// 5. CheckHostRAM with a nil TerminalTrainerService must fail-fast (panic) at
//    build time — a startup misconfiguration must never boot silently.
// ---------------------------------------------------------------------------

func TestPlanChain_CheckHostRAM_NilTerminalService_PanicsAtBuild(t *testing.T) {
	db := freshTestDB(t)

	assert.Panics(t, func() {
		paymentMiddleware.PlanChain(db, entityManagementInterfaces.PlanRequirement{CheckHostRAM: true}, nil)
	}, "CheckHostRAM with a nil TerminalTrainerService must panic at PlanChain build")
}

// ---------------------------------------------------------------------------
// 6. Full requirement → the canonical four-middleware chain in fixed order.
//    Order is pinned by the length here plus the behavioural combo in test 7.
// ---------------------------------------------------------------------------

func TestPlanChain_FullRequirement_ReturnsCanonicalChainLengthFour(t *testing.T) {
	db := freshTestDB(t)
	ts := &stubPlanChainTerminalService{}

	req := entityManagementInterfaces.PlanRequirement{
		OrgContext:   true,
		RequirePlan:  true,
		CheckHostRAM: true,
	}
	chain := paymentMiddleware.PlanChain(db, req, ts)

	// InjectOrgContext(1) + InjectEffectivePlan(1) + RequirePlan(1) + CheckRAMAvailability(1).
	assert.Len(t, chain, 4, "full requirement must produce the canonical four-middleware chain")
}

// ---------------------------------------------------------------------------
// 7. Order pin (behavioural): OrgContext + RequirePlan together must run
//    InjectOrgContext BEFORE InjectEffectivePlan — the org context must reach
//    plan resolution so an org-sourced plan satisfies RequirePlan. Proves the
//    two are emitted in the canonical order, not reversed.
// ---------------------------------------------------------------------------

func TestPlanChain_OrgContextThenRequirePlan_OrgPlanSatisfiesRequirement(t *testing.T) {
	db := freshTestDB(t)

	userID := "org-member-user"
	orgPlan := createPlan(t, db, "Team", 20, 0)
	org, _ := createOrgWithSubscription(t, db, "acme", userID, orgPlan)

	req := entityManagementInterfaces.PlanRequirement{OrgContext: true, RequirePlan: true}
	chain := paymentMiddleware.PlanChain(db, req, nil)
	assert.Len(t, chain, 3, "OrgContext + RequirePlan must produce three middlewares")

	httpReq := httptest.NewRequest(http.MethodGet, "/gate?organization_id="+org.ID.String(), nil)
	rec, probe := runPlanChain(userID, chain, httpReq)

	require.Equal(t, http.StatusOK, rec.Code, "org-sourced plan must satisfy RequirePlan (proves OrgContext ran first)")
	assert.True(t, probe.reached, "handler must be reached")
	assert.True(t, probe.orgContextSet, "org_context_id must be set")
	assert.Equal(t, org.ID.String(), probe.orgContextID)
	assert.True(t, probe.planSet, "the org plan must have been resolved into context")
}
